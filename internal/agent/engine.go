package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/actonos/actonos/internal/tools"
	"github.com/google/uuid"
)

// Engine orchestrates the ReAct cognitive loop for agents.
type Engine struct {
	agentMgr         *AgentManager
	bus              *bus.EventBus
	llm              *llm.ModelCascadeRouter
	memory           *memory.HybridEngine
	embedding        *memory.EmbeddingService
	profileMgr       *UserProfileManager
	tools            *tools.ToolRegistry
	verifier         *Verifier
	tokenTracker     *memory.TokenTracker
	reflectionEngine *ReflectionEngine
	contextManager   *ContextManager
	runStore         *RunStore
	planner          *Planner
	taskMgr          *TaskManager
	sessionMgr       SessionHistoryProvider
	swarm            *SwarmManager
	dataDir          string
	workspaceDir     string
	inFlight         *inFlightRegistry
	approvalWaiters  *approvalWaiters
}

// NewEngine creates an Engine instance.
func NewEngine(
	agentMgr *AgentManager,
	eventBus *bus.EventBus,
	llmRouter *llm.ModelCascadeRouter,
	mem *memory.HybridEngine,
) *Engine {
	return &Engine{
		agentMgr:        agentMgr,
		bus:             eventBus,
		llm:             llmRouter,
		memory:          mem,
		verifier:        NewVerifier(),
		contextManager:  NewContextManager(128000),
		inFlight:        newInFlightRegistry(),
		approvalWaiters: newApprovalWaiters(),
	}
}

// SetSwarmManager attaches multi-agent dispatch for plan roles.
func (e *Engine) SetSwarmManager(swarm *SwarmManager) {
	e.swarm = swarm
}

// CancelRun interrupts an in-flight turn and marks the durable run cancelled.
func (e *Engine) CancelRun(ctx context.Context, runID string) error {
	if runID == "" {
		return fmt.Errorf("run id is required")
	}
	interrupted := e.inFlight.cancelRun(runID)
	if e.runStore != nil {
		if err := e.runStore.Cancel(ctx, runID, "operator_signal"); err != nil && !interrupted {
			return err
		}
	}
	if !interrupted && e.runStore == nil {
		return fmt.Errorf("run %s is not in flight", runID)
	}
	return nil
}

// CancelAgentWork interrupts every in-flight turn for an agent (used by Stop).
func (e *Engine) CancelAgentWork(agentID string) int {
	return e.inFlight.cancelAgent(agentID)
}

// ReclaimOrphanRuns marks process-crash leftovers so heartbeat does not replay them as live.
func (e *Engine) ReclaimOrphanRuns(ctx context.Context) (int, error) {
	if e.runStore == nil {
		return 0, nil
	}
	return e.runStore.ReclaimOrphans(ctx)
}

// ReclaimStaleRuns marks runs in running status older than maxAge as cancelled.
func (e *Engine) ReclaimStaleRuns(ctx context.Context, maxAge time.Duration) (int, error) {
	if e.runStore == nil {
		return 0, nil
	}
	return e.runStore.ReclaimStaleRuns(ctx, maxAge)
}

// SetRunStore attaches durable run and event persistence.
func (e *Engine) SetRunStore(store *RunStore) {
	e.runStore = store
}

// SetPlanner attaches goal decomposition for autonomous missions.
func (e *Engine) SetPlanner(planner *Planner) {
	e.planner = planner
}

// SetContextManager overrides context budgeting behavior.
func (e *Engine) SetContextManager(manager *ContextManager) {
	e.contextManager = manager
}

// SetProfileManager attaches the user profile & soul manager.
func (e *Engine) SetProfileManager(m *UserProfileManager) {
	e.profileMgr = m
}

// SetReflectionEngine attaches the memory reflection daemon.
func (e *Engine) SetReflectionEngine(r *ReflectionEngine) {
	e.reflectionEngine = r
}

// SetToolRegistry attaches the system tool registry to enable tool execution.
func (e *Engine) SetToolRegistry(r *tools.ToolRegistry) {
	e.tools = r
}

// SetTokenTracker attaches the token usage tracker.
func (e *Engine) SetTokenTracker(t *memory.TokenTracker) {
	e.tokenTracker = t
}

func (e *Engine) SetEmbeddingService(service *memory.EmbeddingService) {
	e.embedding = service
}

// SetTaskManager attaches the autonomous task manager.
func (e *Engine) SetTaskManager(tm *TaskManager) {
	e.taskMgr = tm
}

// SetSessionManager attaches the session history provider.
func (e *Engine) SetSessionManager(sm SessionHistoryProvider) {
	e.sessionMgr = sm
}

// SetDataDir sets the root data directory.
func (e *Engine) SetDataDir(dir string) {
	e.dataDir = dir
}

// SetWorkspaceDir sets the primary workspace directory for file sandbox and environment prompt.
func (e *Engine) SetWorkspaceDir(dir string) {
	e.workspaceDir = dir
}

// HasConfiguredLLM reports whether the engine has a real (non-stub) LLM provider configured.
func (e *Engine) HasConfiguredLLM() bool {
	return e.llm != nil && e.llm.HasRealProvider()
}

// RecordTokenUsage records usage metrics if token tracker is configured.
func (e *Engine) RecordTokenUsage(ctx context.Context, agentID, model, provider, source, convID string, usage llm.Usage) {
	if e.tokenTracker == nil {
		return
	}
	go func() {
		_ = e.tokenTracker.Record(context.Background(), memory.TokenUsageRecord{
			AgentID:          agentID,
			Model:            model,
			Provider:         provider,
			PromptTokens:     usage.PromptTokens,
			CompletionTokens: usage.CompletionTokens,
			TotalTokens:      usage.TotalTokens,
			Source:           source,
			ConversationID:   convID,
		})
	}()
}

// buildCognitivePrompt synthesizes all 7 cognitive layers into a structured XML cognitive context
// using the unified PromptBuilder architecture.
func (e *Engine) buildCognitivePrompt(ctx context.Context, agentID string, agent *AgentManifest, userMessage string) (string, int) {
	ctx = e.withSkillCatalog(ctx, agent)
	return BuildCognitiveSystemPrompt(ctx, agentID, agent, e.dataDir, e.workspaceDir, e.profileMgr, e.memory, e.embedding, userMessage)
}

func (e *Engine) withSkillCatalog(ctx context.Context, agent *AgentManifest) context.Context {
	if len(SkillCatalogFrom(ctx)) > 0 {
		return ctx
	}
	return WithSkillCatalog(ctx, e.skillCatalogForAgent(agent))
}

func (e *Engine) withSkillCatalogForID(ctx context.Context, agentID string) context.Context {
	if len(SkillCatalogFrom(ctx)) > 0 {
		return ctx
	}
	return WithSkillCatalog(ctx, e.SkillCatalogForAgent(ctx, agentID))
}

// SkillCatalogForAgent returns enabled skills the agent is allowed to invoke.
func (e *Engine) SkillCatalogForAgent(ctx context.Context, agentID string) []SkillPromptEntry {
	if e == nil || e.tools == nil {
		return nil
	}
	var authorized []string
	if e.agentMgr != nil && agentID != "" {
		if agent, err := e.agentMgr.Get(ctx, agentID); err == nil && agent != nil {
			authorized = agent.AuthorizedTools
		}
	}
	return e.skillCatalog(authorized)
}

func (e *Engine) skillCatalogForAgent(agent *AgentManifest) []SkillPromptEntry {
	if agent == nil {
		return e.skillCatalog(nil)
	}
	return e.skillCatalog(agent.AuthorizedTools)
}

func (e *Engine) skillCatalog(authorized []string) []SkillPromptEntry {
	if e == nil || e.tools == nil {
		return nil
	}
	raw := e.tools.EnabledSkillCatalog(authorized...)
	if len(raw) == 0 {
		return nil
	}
	out := make([]SkillPromptEntry, 0, len(raw))
	for _, entry := range raw {
		out = append(out, SkillPromptEntry{Name: entry.Name, Description: entry.Description, Path: entry.Path})
	}
	return out
}

func capToolObservation(content string) string {
	if len(content) <= maxToolObservationChars {
		return content
	}
	return content[:maxToolObservationChars] + fmt.Sprintf(
		"\n\n[Observation truncated from %d to %d characters. Fetch a narrower URL or quote a specific section.]",
		len(content), maxToolObservationChars,
	)
}

func (e *Engine) compactAfterContextWindowError(ctx context.Context, runID string, agent *AgentManifest, messages []llm.Message) []llm.Message {
	messages = compactToolObservations(messages, 4000)
	if e.contextManager != nil {
		return e.contextManager.PruneAndSnapshot(ctx, runID, messages, e.contextBudget(agent)/2)
	}
	return messages
}

func (e *Engine) contextBudget(agent *AgentManifest) int {
	outputReserve := defaultStreamMaxTokens
	if agent != nil && agent.ModelConfig.MaxTokens > 0 {
		outputReserve = agent.ModelConfig.MaxTokens
	}
	budget := defaultModelWindowTokens - outputReserve - contextProtocolOverhead
	if budget < 24000 {
		budget = 24000
	}
	return budget
}

func (e *Engine) startRun(ctx context.Context, traceID, agentID, goal, source string) *AgentRun {
	if e.runStore == nil {
		return nil
	}
	run, err := e.runStore.Start(ctx, traceID, agentID, goal, source)
	if err != nil {
		return nil
	}
	return run
}

func (e *Engine) finishRun(ctx context.Context, run *AgentRun, status RunStatus, reason string, iterations int, usage llm.Usage) {
	if run == nil || e.runStore == nil {
		return
	}
	run.Status = status
	run.TerminationReason = reason
	run.Iterations = iterations
	run.PromptTokens = usage.PromptTokens
	run.CompletionTokens = usage.CompletionTokens
	run.TotalTokens = usage.TotalTokens
	_ = e.runStore.Finish(ctx, run)
}

func (e *Engine) saveApprovalCheckpoint(
	ctx context.Context,
	run *AgentRun,
	agentID, goal, source string,
	messages []llm.Message,
	iteration int,
	usage llm.Usage,
	pending llm.ToolCall,
) {
	if run == nil || e.runStore == nil {
		return
	}
	taskID := tools.TaskIDFromContext(ctx)
	if taskID == "" {
		taskID, _ = ctx.Value("task_id").(string)
	}
	// The approval was hashed over the normalized input, so the checkpoint must
	// persist the same form or the resume-time hash comparison will not match.
	pending.Function.Arguments = tools.NormalizeToolInput(pending.Function.Arguments)
	_ = e.runStore.SaveCheckpoint(ctx, RunCheckpoint{
		RunID: run.ID, TraceID: run.TraceID, AgentID: agentID, TaskID: taskID, Goal: goal,
		Source: source, Messages: messages, Iteration: iteration,
		Usage: usage, PendingTool: pending, ConversationID: ConversationIDFromContext(ctx),
	})
}

func (e *Engine) persistConversationReply(_ context.Context, convID, agentID string, resp *llm.Response) {
	if e == nil || e.memory == nil || resp == nil {
		return
	}
	convID = strings.TrimSpace(convID)
	if convID == "" {
		return
	}
	content := strings.TrimSpace(resp.Content)
	if content == "" {
		if len(resp.ToolCalls) == 0 {
			return
		}
		content = "Completed requested operations successfully."
	}
	toolCallsJSON := ""
	if len(resp.ToolCalls) > 0 {
		encoded, err := json.Marshal(resp.ToolCalls)
		if err == nil {
			toolCallsJSON = string(encoded)
		}
	}
	now := time.Now().UTC()
	persistCtx := context.Background()
	_, _ = e.memory.DB().SQLDB().ExecContext(persistCtx, `
		INSERT INTO messages (id, conversation_id, agent_id, role, content, tool_calls_json, created_at)
		VALUES (?, ?, ?, 'assistant', ?, ?, ?)
	`, "msg_"+uuid.NewString(), convID, agentID, content, toolCallsJSON, now)
	_, _ = e.memory.DB().SQLDB().ExecContext(persistCtx, `
		UPDATE conversations SET updated_at = ? WHERE id = ?
	`, now, convID)
}

func addUsage(total, current llm.Usage) llm.Usage {
	total.PromptTokens += current.PromptTokens
	total.CompletionTokens += current.CompletionTokens
	total.TotalTokens += current.TotalTokens
	return total
}

func generateTraceID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err != nil {
		return fmt.Sprintf("%032x", time.Now().UnixNano())
	}
	return hex.EncodeToString(buffer)
}

func truncateRunData(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit] + "...[truncated]"
}

func (e *Engine) checkBudget(ctx context.Context, manifest *AgentManifest, source string) error {
	if manifest == nil {
		return nil
	}
	if e.tokenTracker != nil {
		hourlyCap := manifest.DelegationScope.MaxTokensPerHour
		if hourlyCap <= 0 {
			hourlyCap = DefaultMaxTokensPerHour
		}
		if used, err := e.tokenTracker.GetAgentHourlyTokens(ctx, manifest.AgentID); err == nil && hourlyCap > 0 && used >= int64(hourlyCap) {
			if source == "heartbeat" || source == "cron" {
				return fmt.Errorf("%w: agent %s used %d tokens in the last hour (cap %d); degrading autonomous pulses", ErrHourlyTokenQuota, manifest.AgentID, used, hourlyCap)
			}
		}
	}
	if e.tokenTracker == nil || manifest.DelegationScope.MaxMonthlyBudgetUSD <= 0 {
		return nil
	}
	cost, err := e.tokenTracker.GetAgentMonthlyCost(ctx, manifest.AgentID)
	if err != nil {
		return fmt.Errorf("checking monthly agent budget: %w", err)
	}
	budget := manifest.DelegationScope.MaxMonthlyBudgetUSD
	if cost >= budget && (source == "heartbeat" || source == "cron") {
		return fmt.Errorf(
			"agent %s monthly budget exhausted for autonomous work: spent %.4f USD of %.4f USD (interactive chat still allowed on fallback)",
			manifest.AgentID, cost, budget,
		)
	}
	if cost >= budget*0.8 && manifest.ModelConfig.FallbackModel != "" {
		manifest.ModelConfig.PrimaryModel = manifest.ModelConfig.FallbackModel
	}
	return nil
}

func sourceFromContextOrMessage(ctx context.Context, message, fallback string) string {
	if src := ExecutionSource(ctx); src != "" {
		return src
	}
	return sourceFromMessage(message, fallback)
}

func sourceFromMessage(message, fallback string) string {
	switch {
	case strings.Contains(message, "[AUTONOMOUS MISSION"), strings.Contains(message, "<autonomous_mission_cycle"):
		return "heartbeat"
	case strings.Contains(message, "[AUTONOMOUS HEARTBEAT"), strings.Contains(message, "<autonomous_heartbeat"):
		return "heartbeat"
	case strings.Contains(message, "[AUTONOMOUS PROACTIVE"):
		return "cron"
	case strings.Contains(message, "[Channel:"), strings.Contains(message, "[Channel Metadata]"):
		return "channel"
	default:
		return fallback
	}
}

func (e *Engine) verifyAndExecuteTool(ctx context.Context, agentID, name string, args json.RawMessage) (*tools.ToolResult, error) {
	if e.verifier != nil && name == "native_exec" {
		if err := e.verifier.VerifyToolCommand(args); err != nil {
			return nil, err
		}
	}
	if e.tools == nil {
		return nil, fmt.Errorf("tool registry is not configured")
	}
	return e.tools.Execute(ctx, agentID, name, args)
}

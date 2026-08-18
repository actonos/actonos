package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/google/uuid"
)

var (
	ErrScopeExceeded     = errors.New("sub-agent requested tool or resource beyond parent delegation scope")
	ErrSubTaskTimeout    = errors.New("sub-task execution timed out")
	ErrSubAgentExecution = errors.New("sub-agent execution failed")
)

// SwarmManager orchestrates dynamic multi-agent delegation via Goroutines.
type SwarmManager struct {
	agentMgr    *AgentManager
	bus         *bus.EventBus
	llmRouter   *llm.ModelCascadeRouter
	memory      *memory.HybridEngine
	engine      *Engine
	concurrency int
}

// SetEngine routes delegated work through the same durable execution kernel as parent agents.
func (s *SwarmManager) SetEngine(engine *Engine) {
	s.engine = engine
}

// NewSwarmManager creates a new SwarmManager.
func NewSwarmManager(
	agentMgr *AgentManager,
	eventBus *bus.EventBus,
	llmRouter *llm.ModelCascadeRouter,
	mem *memory.HybridEngine,
	maxConcurrency int,
) *SwarmManager {
	if maxConcurrency <= 0 {
		maxConcurrency = 8
	}
	return &SwarmManager{
		agentMgr:    agentMgr,
		bus:         eventBus,
		llmRouter:   llmRouter,
		memory:      mem,
		concurrency: maxConcurrency,
	}
}

// validateDelegationScope enforces Zero-Trust containment: sub-agent cannot request tools parent lacks.
func (s *SwarmManager) validateDelegationScope(parent *AgentManifest, task *SubTask) error {
	if len(task.AuthorizedTools) == 0 {
		// Inherit parent's authorized tools
		task.AuthorizedTools = parent.AuthorizedTools
		return nil
	}

	parentTools := make(map[string]bool)
	for _, tool := range parent.AuthorizedTools {
		parentTools[tool] = true
	}

	for _, tool := range task.AuthorizedTools {
		// Check for exact match or wildcard match
		if !parentTools[tool] && !parentTools["*"] {
			return fmt.Errorf("%w: tool '%s' not authorized by parent agent '%s'", ErrScopeExceeded, tool, parent.AgentID)
		}
	}

	return nil
}

// SpawnSubAgent creates an isolated goroutine to execute a specialized sub-task.
func (s *SwarmManager) SpawnSubAgent(ctx context.Context, parentID string, task SubTask) (<-chan SubTaskResult, error) {
	if task.ID == "" {
		task.ID = "task_" + uuid.New().String()[:8]
	}
	task.ParentAgentID = parentID

	parent, err := s.agentMgr.Get(ctx, parentID)
	if err != nil {
		return nil, fmt.Errorf("retrieving parent agent: %w", err)
	}

	if err := s.validateDelegationScope(parent, &task); err != nil {
		return nil, err
	}

	if task.Timeout <= 0 {
		task.Timeout = 60 * time.Second
	}

	resultChan := make(chan SubTaskResult, 1)

	// Publish spawn event
	if s.bus != nil {
		s.bus.Publish(bus.NewEvent(bus.EventSubTaskSpawned, parentID, task))
	}

	go func() {
		defer close(resultChan)
		startTime := time.Now()

		taskCtx, cancel := context.WithTimeout(ctx, task.Timeout)
		defer cancel()

		var result SubTaskResult
		result.TaskID = task.ID
		result.AgentID = task.AssignedAgentID
		if result.AgentID == "" {
			result.AgentID = parentID + "_sub"
		}

		// Prompt preparation
		systemPrompt := fmt.Sprintf("You are a specialized sub-agent assisting parent agent %s.\nYour task: %s", parent.Name, task.Title)
		messages := []llm.Message{
			{Role: llm.RoleSystem, Content: systemPrompt},
			{Role: llm.RoleUser, Content: task.Prompt},
		}

		var cascadeOrder []string
		if parent.ModelConfig.PrimaryModel != "" {
			cascadeOrder = append(cascadeOrder, parent.ModelConfig.PrimaryModel)
		}
		if parent.ModelConfig.FallbackModel != "" {
			cascadeOrder = append(cascadeOrder, parent.ModelConfig.FallbackModel)
		}

		var resp *llm.Response
		var err error
		if s.engine != nil {
			executionAgentID := result.AgentID
			if _, lookupErr := s.agentMgr.Get(taskCtx, executionAgentID); lookupErr != nil {
				executionAgentID = parentID
			}
			resp, err = s.engine.ExecuteStep(
				taskCtx,
				executionAgentID,
				fmt.Sprintf("[AUTONOMOUS DELEGATED SUBTASK]\nRole: %s\nTask: %s\n\n%s", task.Title, systemPrompt, task.Prompt),
			)
		} else {
			resp, err = s.llmRouter.CompleteWithCascade(taskCtx, cascadeOrder, messages, llm.CompletionOptions{})
		}
		result.ExecutionTime = time.Since(startTime)

		if err != nil {
			if errors.Is(taskCtx.Err(), context.DeadlineExceeded) {
				result.Status = "timeout"
				result.Error = ErrSubTaskTimeout.Error()
			} else {
				result.Status = "failed"
				result.Error = err.Error()
			}
			if s.bus != nil {
				s.bus.Publish(bus.NewEvent(bus.EventSubTaskFailed, parentID, result))
			}
		} else {
			result.Status = "success"
			result.Output = resp.Content
			result.ToolCallsMade = resp.ToolCalls
			result.TokensUsed = resp.Usage.TotalTokens

			if s.bus != nil {
				s.bus.Publish(bus.NewEvent(bus.EventSubTaskCompleted, parentID, result))
			}
		}

		resultChan <- result
	}()

	return resultChan, nil
}

// DispatchSwarm fans out a slice of sub-tasks with bounded concurrency.
func (s *SwarmManager) DispatchSwarm(ctx context.Context, parentID string, tasks []SubTask) (<-chan SubTaskResult, error) {
	outChan := make(chan SubTaskResult, len(tasks))

	if len(tasks) == 0 {
		close(outChan)
		return outChan, nil
	}

	sem := make(chan struct{}, s.concurrency)
	var wg sync.WaitGroup

	for _, task := range tasks {
		wg.Add(1)
		go func(t SubTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ch, err := s.SpawnSubAgent(ctx, parentID, t)
			if err != nil {
				outChan <- SubTaskResult{
					TaskID:  t.ID,
					AgentID: parentID,
					Status:  "failed",
					Error:   err.Error(),
				}
				return
			}

			res, ok := <-ch
			if ok {
				outChan <- res
			}
		}(task)
	}

	go func() {
		wg.Wait()
		close(outChan)
	}()

	return outChan, nil
}

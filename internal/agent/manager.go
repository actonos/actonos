package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
	"github.com/actonos/actonos/internal/memory"
	"github.com/google/uuid"
)

var (
	ErrAgentNotFound    = errors.New("agent not found")
	ErrAgentAlreadyOpen = errors.New("agent already exists")
	ErrInvalidManifest  = errors.New("invalid agent manifest")
	ErrProtectedAgent   = errors.New("cannot delete or modify protected system agent")
)

var slugRegex = regexp.MustCompile(`[^a-z0-9]+`)

// AgentManager handles lifecycle, validation, and persistent storage of all Agent manifests.
type AgentManager struct {
	mu     sync.RWMutex
	db     *memory.DB
	bus    *bus.EventBus
	agents map[string]*AgentManifest
}

// NewAgentManager creates an AgentManager and loads existing agents from SQLite.
func NewAgentManager(db *memory.DB, eventBus *bus.EventBus) (*AgentManager, error) {
	mgr := &AgentManager{
		db:     db,
		bus:    eventBus,
		agents: make(map[string]*AgentManifest),
	}

	if err := mgr.loadAll(); err != nil {
		return nil, fmt.Errorf("loading agents from database: %w", err)
	}

	// Always ensure the default root system agent exists
	if err := mgr.EnsureDefaultAgent(context.Background()); err != nil {
		return nil, fmt.Errorf("ensuring default system agent: %w", err)
	}

	return mgr, nil
}

// loadAll reads all saved agent manifests from SQLite into memory.
func (m *AgentManager) loadAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := m.db.SQLDB().QueryContext(ctx, "SELECT id, manifest_json, status, created_at, updated_at FROM agents")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, manifestJSON, statusStr string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &manifestJSON, &statusStr, &createdAt, &updatedAt); err != nil {
			return err
		}

		var manifest AgentManifest
		if err := json.Unmarshal([]byte(manifestJSON), &manifest); err != nil {
			continue
		}
		manifest.AgentID = id
		if statusStr == "" {
			statusStr = string(StatusActive)
		}
		manifest.Status = AgentStatus(statusStr)
		if manifest.Status == "" {
			manifest.Status = StatusActive
		}
		manifest.CreatedAt = createdAt
		manifest.UpdatedAt = updatedAt

		m.agents[id] = &manifest
	}

	return rows.Err()
}

// generateAgentID creates an identifier like "agent_dev_architect_a1b2c3d4".
func generateAgentID(name string) string {
	slug := strings.Trim(slugRegex.ReplaceAllString(strings.ToLower(name), "_"), "_")
	if slug == "" {
		slug = "custom"
	}
	if len(slug) > 20 {
		slug = slug[:20]
	}
	shortUUID := uuid.New().String()[:8]
	return fmt.Sprintf("agent_%s_%s", slug, shortUUID)
}

// Create validates and saves a new agent manifest.
func (m *AgentManager) Create(ctx context.Context, manifest AgentManifest) (*AgentManifest, error) {
	if strings.TrimSpace(manifest.Name) == "" {
		return nil, fmt.Errorf("%w: name is required", ErrInvalidManifest)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if manifest.AgentID == "" {
		manifest.AgentID = generateAgentID(manifest.Name)
	}

	if _, exists := m.agents[manifest.AgentID]; exists {
		return nil, fmt.Errorf("%w: %s", ErrAgentAlreadyOpen, manifest.AgentID)
	}

	now := time.Now().UTC()
	manifest.CreatedAt = now
	manifest.UpdatedAt = now
	if manifest.Status == "" {
		manifest.Status = StatusActive
	}
	if len(manifest.ListenChannels) == 0 {
		manifest.ListenChannels = []string{"*"}
	}
	if manifest.DelegationScope.RequireHumanApproval == "" {
		manifest.DelegationScope.RequireHumanApproval = ApprovalMedium
	}
	if len(manifest.DelegationScope.AllowedWorkspacePaths) == 0 {
		manifest.DelegationScope.AllowedWorkspacePaths = []string{"."}
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshalling manifest: %w", err)
	}

	query := `
		INSERT INTO agents (id, manifest_json, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
	`
	_, err = m.db.SQLDB().ExecContext(ctx, query, manifest.AgentID, string(manifestJSON), string(manifest.Status), now, now)
	if err != nil {
		return nil, fmt.Errorf("persisting agent to database: %w", err)
	}

	stored := manifest
	m.agents[manifest.AgentID] = &stored

	if m.bus != nil {
		m.bus.Publish(bus.NewEvent(bus.EventAgentCreated, manifest.AgentID, stored))
	}

	return &stored, nil
}

// Get returns an agent manifest by ID.
func (m *AgentManager) Get(ctx context.Context, agentID string) (*AgentManifest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	copyManifest := *agent
	return &copyManifest, nil
}

// List returns all registered agent manifests.
func (m *AgentManager) List(ctx context.Context) ([]AgentManifest, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]AgentManifest, 0, len(m.agents))
	for _, a := range m.agents {
		out = append(out, *a)
	}
	return out, nil
}

// Update modifies an existing agent manifest.
func (m *AgentManager) Update(ctx context.Context, manifest AgentManifest) (*AgentManifest, error) {
	if manifest.AgentID == "" {
		return nil, fmt.Errorf("%w: agent_id is required", ErrInvalidManifest)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.agents[manifest.AgentID]
	if !exists {
		return nil, fmt.Errorf("%w: %s", ErrAgentNotFound, manifest.AgentID)
	}

	now := time.Now().UTC()
	manifest.CreatedAt = existing.CreatedAt
	manifest.UpdatedAt = now
	if manifest.Status == "" {
		if existing.Status != "" {
			manifest.Status = existing.Status
		} else {
			manifest.Status = StatusActive
		}
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("marshalling manifest: %w", err)
	}

	query := `
		UPDATE agents
		SET manifest_json = ?, status = ?, updated_at = ?
		WHERE id = ?
	`
	_, err = m.db.SQLDB().ExecContext(ctx, query, string(manifestJSON), string(manifest.Status), now, manifest.AgentID)
	if err != nil {
		return nil, fmt.Errorf("updating agent in database: %w", err)
	}

	updated := manifest
	m.agents[manifest.AgentID] = &updated

	if m.bus != nil {
		m.bus.Publish(bus.NewEvent(bus.EventAgentUpdated, manifest.AgentID, updated))
	}

	return &updated, nil
}

// EnsureDefaultAgent creates and persists the built-in system root agent if not present.
func (m *AgentManager) EnsureDefaultAgent(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	defaultSysInstructions := `You are Nova, the primary autonomous intelligence operator and workspace coordinator for ActonOS.

- Role Scope: Coordinate workspace activities, execute authorized tools, and assist across diverse technical, research, and operational domains.
- Execution Protocol: Think systematically before executing actions. Verify observations from tools before concluding.
- Tool Discipline: Only invoke tools directly related to the user request. When tools return observations (e.g. web search, file inspect), synthesize findings immediately into a complete, direct response.
- Communication: Direct, articulate, warm, and highly dependable. Deliver comprehensive substance with zero robotic disclaimers or canned filler.
- Safety: Restrict all file operations strictly to authorized workspace paths. Never perform destructive operations without confirmation.`

	if existing, exists := m.agents[DefaultSystemAgentID]; exists {
		needUpdate := false
		if existing.Status == "" || existing.Status == StatusStopped {
			existing.Status = StatusActive
			needUpdate = true
		}
		// Upgrade legacy or missing tool discipline instructions
		if !strings.Contains(existing.SystemInstructions, "Synthesize Tool Results") ||
			!strings.Contains(existing.SystemInstructions, "Domain Relevance") ||
			strings.Contains(existing.SystemInstructions, "You execute tasks with utmost technical precision") ||
			strings.Contains(existing.SystemInstructions, "như một cộng sự") ||
			strings.Contains(existing.SystemInstructions, "systems architect running directly") ||
			strings.TrimSpace(existing.SystemInstructions) == "" {
			existing.SystemInstructions = defaultSysInstructions
			needUpdate = true
		}
		if existing.ModelConfig.MaxTokens <= 4096 {
			existing.ModelConfig.MaxTokens = 32768
			needUpdate = true
		}
		if needUpdate {
			now := time.Now().UTC()
			existing.UpdatedAt = now
			manifestJSON, _ := json.Marshal(existing)
			_, _ = m.db.SQLDB().ExecContext(ctx, "UPDATE agents SET status = ?, manifest_json = ?, updated_at = ? WHERE id = ?", string(StatusActive), string(manifestJSON), now, DefaultSystemAgentID)
		}
		return nil
	}

	now := time.Now().UTC()
	sysAgent := AgentManifest{
		AgentID:     DefaultSystemAgentID,
		Name:        "Nova",
		Description: "Built-in autonomous primary assistant for ActonOS. Coordinates workspace activities, executes tools, and manages tasks across multiple domains.",
		AvatarIcon:  "sparkles",
		Status:      StatusActive,
		IsSystem:    true,
		ModelConfig: llm.ModelConfig{
			PrimaryModel:  "openai/gpt-5.4-mini",
			FallbackModel: "openai/gpt-5.4-mini",
			Temperature:   0.2,
			MaxTokens:     32768,
		},
		SystemInstructions: defaultSysInstructions,
		AuthorizedTools:    []string{"*"},
		ListenChannels:     []string{"*"},
		DelegationScope: DelegationScope{
			MaxMonthlyBudgetUSD:   100.0,
			AllowedWorkspacePaths: []string{"*"},
			RequireHumanApproval:  ApprovalMedium,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	manifestJSON, err := json.Marshal(sysAgent)
	if err != nil {
		return err
	}

	query := `
		INSERT INTO agents (id, manifest_json, status, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO NOTHING
	`
	_, err = m.db.SQLDB().ExecContext(ctx, query, sysAgent.AgentID, string(manifestJSON), string(sysAgent.Status), now, now)
	if err != nil {
		return fmt.Errorf("persisting default system agent: %w", err)
	}

	stored := sysAgent
	m.agents[DefaultSystemAgentID] = &stored
	return nil
}

// Delete removes an agent manifest from storage. System agents cannot be deleted.
func (m *AgentManager) Delete(ctx context.Context, agentID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	if agentID == DefaultSystemAgentID || agent.IsSystem {
		return ErrProtectedAgent
	}

	query := `DELETE FROM agents WHERE id = ?`
	_, err := m.db.SQLDB().ExecContext(ctx, query, agentID)
	if err != nil {
		return fmt.Errorf("deleting agent from database: %w", err)
	}

	delete(m.agents, agentID)

	if m.bus != nil {
		m.bus.Publish(bus.NewEvent(bus.EventAgentDeleted, agentID, nil))
	}

	return nil
}

// Start sets an agent's status to active.
func (m *AgentManager) Start(ctx context.Context, agentID string) error {
	return m.setStatus(ctx, agentID, StatusActive)
}

// Stop sets an agent's status to stopped.
func (m *AgentManager) Stop(ctx context.Context, agentID string) error {
	return m.setStatus(ctx, agentID, StatusStopped)
}

func (m *AgentManager) setStatus(ctx context.Context, agentID string, status AgentStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	agent, exists := m.agents[agentID]
	if !exists {
		return fmt.Errorf("%w: %s", ErrAgentNotFound, agentID)
	}

	now := time.Now().UTC()
	agent.Status = status
	agent.UpdatedAt = now

	manifestJSON, err := json.Marshal(agent)
	if err != nil {
		return err
	}

	query := `UPDATE agents SET status = ?, manifest_json = ?, updated_at = ? WHERE id = ?`
	_, err = m.db.SQLDB().ExecContext(ctx, query, string(status), string(manifestJSON), now, agentID)
	if err != nil {
		return fmt.Errorf("updating agent status in db: %w", err)
	}

	if m.bus != nil {
		m.bus.Publish(bus.NewEvent(bus.EventAgentStatusChanged, agentID, map[string]any{
			"status": status,
		}))
	}

	return nil
}

package channels

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
)

type mockAgentProvider struct {
	agents []agent.AgentManifest
}

func (m *mockAgentProvider) List(ctx context.Context) ([]agent.AgentManifest, error) {
	return m.agents, nil
}

func (m *mockAgentProvider) Get(ctx context.Context, agentID string) (*agent.AgentManifest, error) {
	for _, a := range m.agents {
		if a.AgentID == agentID {
			return &a, nil
		}
	}
	return nil, errors.New("not found")
}

type mockEngineExecutor struct {
	mu          sync.Mutex
	lastAgentID string
	lastPrompt  string
	result      *llm.Response
}

func (m *mockEngineExecutor) ExecuteStepWithHistory(ctx context.Context, agentID, prompt string, history []llm.Message) (*llm.Response, error) {
	m.mu.Lock()
	m.lastAgentID = agentID
	m.lastPrompt = prompt
	result := m.result
	m.mu.Unlock()
	if result != nil {
		return result, nil
	}
	return &llm.Response{
		Content: "Mock response from " + agentID,
	}, nil
}

func (m *mockEngineExecutor) lastAgent() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastAgentID
}

func TestExtractAgentMention(t *testing.T) {
	tests := []struct {
		input       string
		wantMention string
		wantClean   string
	}{
		{"@support help me please", "support", "help me please"},
		{"/agent devops deploy to prod", "devops", "deploy to prod"},
		{"/ask finance what is the budget?", "finance", "what is the budget?"},
		{"hello world", "", "hello world"},
		{"", "", ""},
	}

	for _, tt := range tests {
		m, c := ExtractAgentMention(tt.input)
		if m != tt.wantMention || c != tt.wantClean {
			t.Errorf("ExtractAgentMention(%q) = (%q, %q), want (%q, %q)", tt.input, m, c, tt.wantMention, tt.wantClean)
		}
	}
}

func TestMessageRouter_ResolveAgent(t *testing.T) {
	agents := []agent.AgentManifest{
		{
			AgentID:        "agent_system_core",
			Name:           "Nova",
			Status:         agent.StatusActive,
			IsSystem:       true,
			ListenChannels: []string{"*"},
		},
		{
			AgentID:        "agent_support",
			Name:           "Support Bot",
			Status:         agent.StatusActive,
			ListenChannels: []string{"telegram"},
		},
		{
			AgentID:        "agent_devops",
			Name:           "DevOps Engineer",
			Status:         agent.StatusActive,
			ListenChannels: []string{"discord"},
		},
	}

	eb := bus.NewEventBus()
	defer eb.Close()
	cm := NewChannelManager(eb, nil)
	defer cm.Stop()

	cm.accounts["tg_support"] = ChannelAccount{
		ID:            "tg_support",
		Channel:       "telegram",
		Enabled:       true,
		BoundAgentIDs: []string{"agent_support"},
	}
	cm.accounts["dc_all"] = ChannelAccount{
		ID:            "dc_all",
		Channel:       "discord",
		Enabled:       true,
		BoundAgentIDs: []string{"*"},
	}

	router := NewMessageRouter(cm, &mockAgentProvider{agents: agents}, nil, &mockEngineExecutor{}, eb)

	// 1. Explicit mention overrides account binding
	msg1 := InboundMessage{
		ChannelID:   "telegram",
		AccountID:   "tg_support",
		TargetAgent: "agent_devops",
		Content:     "run health check",
	}
	// Note: agent_devops only listens to discord, so it shouldn't match for telegram
	if target := router.ResolveAgent(context.Background(), msg1); target != "agent_support" {
		t.Fatalf("expected fallback to bound agent_support for mismatched channel, got %s", target)
	}

	// 2. Mention matching agent with matching channel
	msg2 := InboundMessage{
		ChannelID:   "telegram",
		AccountID:   "tg_support",
		TargetAgent: "agent_support",
		Content:     "help",
	}
	if target := router.ResolveAgent(context.Background(), msg2); target != "agent_support" {
		t.Fatalf("expected agent_support, got %s", target)
	}

	// 3. Bound account routing
	msg3 := InboundMessage{
		ChannelID: "telegram",
		AccountID: "tg_support",
		Content:   "general inquiry",
	}
	if target := router.ResolveAgent(context.Background(), msg3); target != "agent_support" {
		t.Fatalf("expected agent_support from account binding, got %s", target)
	}

	// 4. Wildcard account with explicit channel listener
	msg4 := InboundMessage{
		ChannelID: "discord",
		AccountID: "dc_all",
		Content:   "deploy build",
	}
	if target := router.ResolveAgent(context.Background(), msg4); target != "agent_devops" {
		t.Fatalf("expected agent_devops from explicit channel listen, got %s", target)
	}
}

func TestMessageRouter_Route(t *testing.T) {
	agents := []agent.AgentManifest{
		{
			AgentID:        "agent_system_core",
			Name:           "Nova",
			Status:         agent.StatusActive,
			IsSystem:       true,
			ListenChannels: []string{"*"},
		},
		{
			AgentID:        "agent_support",
			Name:           "Support Bot",
			Status:         agent.StatusActive,
			ListenChannels: []string{"telegram"},
		},
	}

	eb := bus.NewEventBus()
	defer eb.Close()
	cm := NewChannelManager(eb, nil)
	defer cm.Stop()

	cm.accounts["tg_1"] = ChannelAccount{
		ID:            "tg_1",
		Channel:       "telegram",
		Enabled:       true,
		BoundAgentIDs: []string{"agent_support"},
	}

	engine := &mockEngineExecutor{
		result: &llm.Response{
			Content: "Hello, I am Support Bot!",
		},
	}

	sessionMgr := NewChannelSessionManager(nil)
	router := NewMessageRouter(cm, &mockAgentProvider{agents: agents}, sessionMgr, engine, eb)

	msg := InboundMessage{
		ChannelID: "telegram",
		AccountID: "tg_1",
		SenderID:  "user_100",
		Content:   "Hello!",
	}

	err := router.Route(context.Background(), msg)
	if err != nil {
		t.Fatalf("Route failed: %v", err)
	}

	if engine.lastAgent() != "agent_support" {
		t.Fatalf("expected engine to execute with agent_support, got %s", engine.lastAgent())
	}
}

func TestMessageRouter_EventBusSubscription(t *testing.T) {
	agents := []agent.AgentManifest{
		{
			AgentID:        "agent_system_core",
			Name:           "Nova",
			Status:         agent.StatusActive,
			IsSystem:       true,
			ListenChannels: []string{"*"},
		},
	}

	eb := bus.NewEventBus()
	defer eb.Close()
	cm := NewChannelManager(eb, nil)
	defer cm.Stop()

	engine := &mockEngineExecutor{
		result: &llm.Response{
			Content: "System response",
		},
	}

	router := NewMessageRouter(cm, &mockAgentProvider{agents: agents}, nil, engine, eb)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	router.Start(ctx)

	// Publish an inbound channel message event
	eb.Publish(bus.NewEvent(bus.EventChannelMessage, "telegram", InboundMessage{
		ChannelID: "telegram",
		AccountID: "tg_test",
		SenderID:  "user_200",
		Content:   "Test ping",
	}))

	// Allow goroutine to process
	time.Sleep(100 * time.Millisecond)

	if engine.lastAgent() != "agent_system_core" {
		t.Fatalf("expected event router to dispatch to agent_system_core, got %s", engine.lastAgent())
	}

	router.Stop()
}

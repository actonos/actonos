package agent

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

// HeartbeatDaemon monitors proactive trigger rules and executes scheduled background tasks.
type HeartbeatDaemon struct {
	mu       sync.RWMutex
	agentMgr *AgentManager
	engine   *Engine
	eventBus *bus.EventBus
	interval time.Duration
	stopCh   chan struct{}
}

// NewHeartbeatDaemon creates a new HeartbeatDaemon.
func NewHeartbeatDaemon(
	agentMgr *AgentManager,
	engine *Engine,
	eventBus *bus.EventBus,
	interval time.Duration,
) *HeartbeatDaemon {
	if interval <= 0 {
		interval = 1 * time.Minute
	}
	return &HeartbeatDaemon{
		agentMgr: agentMgr,
		engine:   engine,
		eventBus: eventBus,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start launches the heartbeat evaluation loop.
func (h *HeartbeatDaemon) Start(ctx context.Context) {
	go h.loop(ctx)
}

func (h *HeartbeatDaemon) loop(ctx context.Context) {
	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-h.stopCh:
			return
		case <-ticker.C:
			h.checkTriggers(ctx)
		}
	}
}

func (h *HeartbeatDaemon) checkTriggers(ctx context.Context) {
	agents, err := h.agentMgr.List(ctx)
	if err != nil {
		return
	}

	now := time.Now().UTC()
	for _, ag := range agents {
		if ag.Status != StatusActive || len(ag.TriggerRules) == 0 {
			continue
		}

		for _, rule := range ag.TriggerRules {
			if rule.Type == "interval_check" || (rule.Type == "cron_schedule" && rule.Expression != "") {
				slog.Debug("heartbeat triggered rule for agent", "agent_id", ag.AgentID, "rule_type", rule.Type)
				// Emit proactive trigger event
				h.eventBus.Publish(bus.NewEvent(bus.EventSystemHeartbeat, "heartbeat", map[string]any{
					"agent_id":   ag.AgentID,
					"rule_type":  rule.Type,
					"timestamp":  now,
				}))
			}
		}
	}
}

// Stop terminates the heartbeat daemon.
func (h *HeartbeatDaemon) Stop() {
	close(h.stopCh)
}

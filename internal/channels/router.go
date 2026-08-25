package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/agent"
	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/llm"
)

// AgentProvider abstracts agent manifest lookups for the message router.
type AgentProvider interface {
	List(ctx context.Context) ([]agent.AgentManifest, error)
	Get(ctx context.Context, agentID string) (*agent.AgentManifest, error)
}

// EngineExecutor abstracts the ReAct cognitive execution loop.
type EngineExecutor interface {
	ExecuteStepWithHistory(ctx context.Context, agentID, prompt string, history []llm.Message) (*llm.Response, error)
}

// MessageRouter coordinates multi-account messaging dispatch, agent resolution,
// session persistence, cognitive execution, and outbound delivery.
type MessageRouter struct {
	channelMgr *ChannelManager
	agentMgr   AgentProvider
	sessionMgr *ChannelSessionManager
	engine     EngineExecutor
	eventBus   *bus.EventBus
	pairing    *PairingManager
	inbound    *InboundQueue
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
}

// NewMessageRouter initializes a MessageRouter instance.
func NewMessageRouter(
	channelMgr *ChannelManager,
	agentMgr AgentProvider,
	sessionMgr *ChannelSessionManager,
	engine EngineExecutor,
	eventBus *bus.EventBus,
) *MessageRouter {
	ctx, cancel := context.WithCancel(context.Background())
	return &MessageRouter{
		channelMgr: channelMgr,
		agentMgr:   agentMgr,
		sessionMgr: sessionMgr,
		engine:     engine,
		eventBus:   eventBus,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// SetPairingManager enables inbound pairing enforcement.
func (r *MessageRouter) SetPairingManager(pm *PairingManager) {
	r.pairing = pm
}

// SetInboundQueue persists inbound events before EventBus fan-out so a full
// in-memory buffer cannot drop a wake.
func (r *MessageRouter) SetInboundQueue(q *InboundQueue) {
	r.inbound = q
	if r.eventBus != nil && q != nil {
		r.eventBus.SetPersist(q.PersistEvent)
	}
}

// Start launches background subscribers for inbound messages and proactive notifications.
func (r *MessageRouter) Start(ctx context.Context) {
	if r.eventBus == nil {
		return
	}

	if ctx != nil {
		r.ctx, r.cancel = context.WithCancel(ctx)
	}

	channelSub := r.eventBus.Subscribe(bus.EventChannelMessage)
	doneSub := r.eventBus.Subscribe(bus.EventAgentActionDone)
	r.drainInboundQueue()

	r.wg.Add(1)
	go func() {
		defer r.wg.Done()
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.ctx.Done():
				return
			case <-ticker.C:
				r.drainInboundQueue()
			case ev, ok := <-channelSub:
				if !ok {
					return
				}
				if inMsg, ok := ev.Payload.(InboundMessage); ok {
					if r.inbound != nil {
						r.drainInboundQueue()
					} else {
						go func(msg InboundMessage) {
							if err := r.Route(r.ctx, msg); err != nil {
								slog.Error("failed to route inbound channel message",
									"channel", msg.ChannelID,
									"account", msg.AccountID,
									"sender", msg.SenderID,
									"error", err,
								)
							}
						}(inMsg)
					}
				}
			case ev, ok := <-doneSub:
				if !ok {
					return
				}
				if payloadMap, ok := ev.Payload.(map[string]any); ok {
					if pType, ok := payloadMap["type"].(string); ok && pType == "proactive_cron_notification" {
						content, _ := payloadMap["content"].(string)
						jobName, _ := payloadMap["job_name"].(string)
						targetChan, _ := payloadMap["target_channel"].(string)
						targetAcc, _ := payloadMap["target_account_id"].(string)
						targetRec, _ := payloadMap["target_recipient"].(string)

						if targetChan == "" {
							targetChan = "all"
						}
						if targetAcc == "" {
							targetAcc = "all"
						}

						msgText := fmt.Sprintf("⏰ **[%s]**\n\n%s", jobName, content)

						if r.channelMgr != nil {
							_ = r.channelMgr.SendMessage(context.Background(), OutboundMessage{
								ChannelID: targetChan,
								AccountID: targetAcc,
								Recipient: targetRec,
								Content:   msgText,
							})
						}
					}
				}
			}
		}
	}()
}

// Stop terminates background event listeners.
func (r *MessageRouter) Stop() {
	r.cancel()
	r.wg.Wait()
}

// ResolveAgent determines which agent should process an incoming message.
func (r *MessageRouter) ResolveAgent(ctx context.Context, msg InboundMessage) string {
	defaultAgent := agent.DefaultSystemAgentID

	if r.agentMgr == nil {
		return defaultAgent
	}

	agents, err := r.agentMgr.List(ctx)
	if err != nil || len(agents) == 0 {
		return defaultAgent
	}

	// 1. Check if an explicit @mention target agent was parsed
	if msg.TargetAgent != "" && msg.TargetAgent != "default" {
		targetSlug := strings.ToLower(strings.TrimSpace(msg.TargetAgent))
		for _, a := range agents {
			if a.Status != agent.StatusActive {
				continue
			}
			// Match by exact AgentID or normalized Name
			normalizedName := strings.ToLower(strings.ReplaceAll(a.Name, " ", "_"))
			if a.AgentID == msg.TargetAgent || strings.EqualFold(a.AgentID, msg.TargetAgent) || normalizedName == targetSlug || strings.EqualFold(a.Name, msg.TargetAgent) {
				if AgentListensTo(a, msg.ChannelID, msg.AccountID) {
					return a.AgentID
				}
			}
		}
	}

	// 2. Check channel account bindings
	if r.channelMgr != nil && msg.AccountID != "" {
		if acc, ok := r.channelMgr.GetAccountByID(msg.AccountID); ok {
			// Single bound agent
			if len(acc.BoundAgentIDs) == 1 && acc.BoundAgentIDs[0] != "*" && acc.BoundAgentIDs[0] != "all" {
				boundID := acc.BoundAgentIDs[0]
				for _, a := range agents {
					if a.AgentID == boundID && a.Status == agent.StatusActive {
						return a.AgentID
					}
				}
			}

			// Multiple bound agents: pick the first active one that listens to this channel
			if len(acc.BoundAgentIDs) > 1 {
				for _, boundID := range acc.BoundAgentIDs {
					if boundID == "*" || boundID == "all" {
						continue
					}
					for _, a := range agents {
						if a.AgentID == boundID && a.Status == agent.StatusActive && AgentListensTo(a, msg.ChannelID, msg.AccountID) {
							return a.AgentID
						}
					}
				}
			}
		}
	}

	// 3. Fallback to any active non-system agent explicitly listening to this channel (if only 1 exists)
	var explicitMatches []string
	for _, a := range agents {
		if a.IsSystem || a.Status != agent.StatusActive {
			continue
		}
		if len(a.ListenChannels) > 0 && !containsWildcard(a.ListenChannels) && AgentListensTo(a, msg.ChannelID, msg.AccountID) {
			explicitMatches = append(explicitMatches, a.AgentID)
		}
	}
	if len(explicitMatches) == 1 {
		return explicitMatches[0]
	}

	// 4. Default to system core agent
	return defaultAgent
}

// AgentListensTo checks if an agent's ListenChannels config allows incoming messages from a specific channel & account.
func AgentListensTo(manifest agent.AgentManifest, channelID, accountID string) bool {
	if len(manifest.ListenChannels) == 0 {
		return true
	}
	for _, ch := range manifest.ListenChannels {
		ch = strings.TrimSpace(strings.ToLower(ch))
		if ch == "*" || ch == "all" {
			return true
		}
		if strings.EqualFold(ch, channelID) {
			return true
		}
		if accountID != "" && strings.EqualFold(ch, fmt.Sprintf("%s:%s", channelID, accountID)) {
			return true
		}
	}
	return false
}

func (r *MessageRouter) enforcePairing(msg InboundMessage) error {
	if r.pairing == nil {
		return nil
	}
	required := false
	if r.channelMgr != nil && msg.AccountID != "" {
		if acc, ok := r.channelMgr.GetAccountByID(msg.AccountID); ok {
			required = acc.RequiresPairing
		}
	}
	if !required {
		return nil
	}
	if r.pairing.IsAuthorized(msg.ChannelID, msg.SenderID) {
		r.pairing.TouchUser(msg.ChannelID, msg.SenderID)
		return nil
	}
	if pin := ExtractPairingPIN(msg.Content); pin != "" {
		ok, err := r.pairing.ValidateAndPair(msg.ChannelID, pin, msg.SenderID, msg.SenderName)
		if err != nil {
			return fmt.Errorf("pairing failed: %w", err)
		}
		if ok {
			return nil
		}
	}
	return fmt.Errorf("unpaired sender %s on channel %s", msg.SenderID, msg.ChannelID)
}

func containsWildcard(list []string) bool {
	for _, s := range list {
		if s == "*" || s == "all" {
			return true
		}
	}
	return false
}

func (r *MessageRouter) drainInboundQueue() {
	if r.inbound == nil {
		return
	}
	claimed, err := r.inbound.Claim(32)
	if err != nil {
		slog.Warn("inbound queue claim failed", "error", err)
		return
	}
	for _, item := range claimed {
		msg := item.Message
		if err := r.Route(r.ctx, msg); err != nil {
			slog.Error("failed to route inbound channel message",
				"channel", msg.ChannelID,
				"account", msg.AccountID,
				"sender", msg.SenderID,
				"error", err,
			)
		}
	}
}

// Route executes the full end-to-end routing flow for an inbound message.
func (r *MessageRouter) Route(ctx context.Context, msg InboundMessage) error {
	if err := r.enforcePairing(msg); err != nil {
		return err
	}
	targetAgent := r.ResolveAgent(ctx, msg)

	senderID := msg.SenderID
	if msg.Metadata != nil && msg.Metadata["chat_id"] != "" {
		senderID = msg.Metadata["chat_id"]
	}

	// 1. Get or create deterministic intelligent session (agent-aware)
	var convID string
	var err error
	if r.sessionMgr != nil {
		convID, err = r.sessionMgr.GetOrCreateSession(ctx, msg.ChannelID, senderID, msg.SenderName, msg.Content, targetAgent)
		if err != nil {
			slog.Warn("failed to get/create channel session", "error", err)
		}
	}

	// 2. Load short-term Working Memory (recent dialogue history)
	var history []llm.Message
	if r.sessionMgr != nil && convID != "" {
		history = r.sessionMgr.LoadRecentHistory(ctx, convID, 6)
	}

	// 3. Persist incoming user message into SQLite
	if r.sessionMgr != nil && convID != "" {
		_ = r.sessionMgr.SaveMessage(ctx, convID, targetAgent, "user", msg.Content, nil)
	}

	// 4. Construct contextual metadata prompt
	chatMeta := ""
	if msg.Metadata != nil && msg.Metadata["chat_id"] != "" {
		chatMeta = fmt.Sprintf("[Channel: %s | Account: %s | Chat ID: %s | Sender: %s]\n", msg.ChannelID, msg.AccountID, msg.Metadata["chat_id"], msg.SenderName)
	} else if msg.ChannelID != "" {
		chatMeta = fmt.Sprintf("[Channel: %s | Account: %s | Sender ID: %s]\n", msg.ChannelID, msg.AccountID, msg.SenderID)
	}
	promptWithMeta := chatMeta + msg.Content

	// 5. Execute cognitive ReAct loop with multi-layer memory
	slog.Info("processing inbound channel message",
		"channel", msg.ChannelID,
		"account", msg.AccountID,
		"sender", msg.SenderID,
		"target_agent", targetAgent,
	)

	if r.engine == nil {
		return fmt.Errorf("engine not initialized")
	}

	channelCtx := agent.WithConversationContext(agent.WithExecutionSource(ctx, "channel"), convID)
	resp, execErr := r.engine.ExecuteStepWithHistory(channelCtx, targetAgent, promptWithMeta, history)
	if execErr != nil {
		slog.Error("failed to process channel message", "channel", msg.ChannelID, "error", execErr)
		if r.channelMgr != nil {
			_ = r.channelMgr.SendMessage(context.Background(), OutboundMessage{
				ChannelID: msg.ChannelID,
				AccountID: msg.AccountID,
				Recipient: senderID,
				Content:   fmt.Sprintf("⚠️ Unable to process request: %v", execErr),
				Metadata:  msg.Metadata,
			})
		}
		return execErr
	}

	// 6. Persist assistant response into SQLite session history
	if resp != nil && r.sessionMgr != nil && convID != "" {
		_ = r.sessionMgr.SaveMessage(ctx, convID, targetAgent, "assistant", resp.Content, resp.ToolCalls)
	}

	// 7. Deliver outbound response via ChannelManager
	alreadySentViaNotifyTool := false
	if resp != nil {
		for _, tc := range resp.ToolCalls {
			if tc.Function.Name == "native_channel_notify" || tc.Function.Name == "channel_notify" {
				alreadySentViaNotifyTool = true
				break
			}
		}
	}
	if resp != nil && !alreadySentViaNotifyTool && r.channelMgr != nil {
		_ = r.channelMgr.SendMessage(context.Background(), OutboundMessage{
			ChannelID: msg.ChannelID,
			AccountID: msg.AccountID,
			Recipient: senderID,
			Content:   resp.Content,
			Metadata:  msg.Metadata,
		})
	}

	return nil
}

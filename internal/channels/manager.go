package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

// ChannelManager coordinates messaging adapters across dynamic WASM plugins and adapters.
type ChannelManager struct {
	mu         sync.RWMutex
	eventBus   *bus.EventBus
	pairingMgr *PairingManager
	accounts   map[string]ChannelAccount // keyed by account ID
	adapters   map[string]ChannelAdapter // keyed by lowercase adapter/channel name
	statuses   map[string]AccountStatus  // keyed by account ID; runtime health
	ctx        context.Context
	cancel     context.CancelFunc
}

// AccountStatus is the in-memory runtime health of a channel account adapter.
type AccountStatus struct {
	Connected   bool      `json:"connected"`
	LastError   string    `json:"last_error,omitempty"`
	LastErrorAt time.Time `json:"last_error_at,omitempty"`
}

// NewChannelManager initializes the channel manager.
func NewChannelManager(eventBus *bus.EventBus, pairingMgr *PairingManager) *ChannelManager {
	ctx, cancel := context.WithCancel(context.Background())
	m := &ChannelManager{
		eventBus:   eventBus,
		pairingMgr: pairingMgr,
		accounts:   make(map[string]ChannelAccount),
		adapters:   make(map[string]ChannelAdapter),
		statuses:   make(map[string]AccountStatus),
		ctx:        ctx,
		cancel:     cancel,
	}
	m.watchAdapterHealthEvents()
	return m
}

func (m *ChannelManager) watchAdapterHealthEvents() {
	if m.eventBus == nil {
		return
	}
	errSub := m.eventBus.Subscribe(bus.EventChannelAdapterError)
	recoveredSub := m.eventBus.Subscribe(bus.EventChannelAdapterRecovered)
	go func() {
		for {
			select {
			case ev, ok := <-errSub:
				if !ok {
					return
				}
				m.applyAdapterHealthEvent(ev, false)
			case ev, ok := <-recoveredSub:
				if !ok {
					return
				}
				m.applyAdapterHealthEvent(ev, true)
			}
		}
	}()
}

func (m *ChannelManager) applyAdapterHealthEvent(ev bus.Event, recovered bool) {
	payload, ok := ev.Payload.(map[string]any)
	if !ok {
		return
	}
	accountID, _ := payload["account_id"].(string)
	if accountID == "" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if recovered {
		m.statuses[accountID] = AccountStatus{Connected: true}
		return
	}
	errMsg, _ := payload["error"].(string)
	m.statuses[accountID] = AccountStatus{Connected: false, LastError: errMsg, LastErrorAt: time.Now().UTC()}
}

// Start begins listening on all registered channel adapters.
func (m *ChannelManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ctx != nil && ctx.Done() != nil {
		m.ctx = ctx
	}

	for name, adapter := range m.adapters {
		if err := adapter.Start(m.ctx); err != nil {
			slog.Warn("failed to start channel adapter", "name", name, "error", err)
		}
	}
	return nil
}

// Stop cleanly terminates all running channel adapters.
func (m *ChannelManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancel()
	for _, a := range m.adapters {
		_ = a.Stop()
	}
	return nil
}

// RegisterAdapter registers a channel adapter (e.g. from WASM plugin or custom module).
func (m *ChannelManager) RegisterAdapter(adapter ChannelAdapter) error {
	if adapter == nil {
		return fmt.Errorf("cannot register nil adapter")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	name := strings.ToLower(adapter.Name())
	m.adapters[name] = adapter
	if m.ctx != nil && m.ctx.Err() == nil {
		_ = adapter.Start(m.ctx)
	}
	slog.Info("channel adapter registered", "name", name)
	return nil
}

// RegisterDynamicAdapter is an alias for RegisterAdapter for backward compatibility.
func (m *ChannelManager) RegisterDynamicAdapter(adapter ChannelAdapter) error {
	return m.RegisterAdapter(adapter)
}

// UnregisterAdapter removes a channel adapter.
func (m *ChannelManager) UnregisterAdapter(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := strings.ToLower(name)
	if adapter, ok := m.adapters[key]; ok {
		_ = adapter.Stop()
		delete(m.adapters, key)
		slog.Info("channel adapter unregistered", "name", name)
	}
	return nil
}

// UnregisterDynamicAdapter is an alias for UnregisterAdapter.
func (m *ChannelManager) UnregisterDynamicAdapter(name string) error {
	return m.UnregisterAdapter(name)
}

// GetAdapter retrieves a registered adapter by name.
func (m *ChannelManager) GetAdapter(name string) (ChannelAdapter, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	a, ok := m.adapters[strings.ToLower(name)]
	return a, ok
}

// SyncAccounts stores and synchronizes account configurations.
func (m *ChannelManager) SyncAccounts(ctx context.Context, accounts []ChannelAccount) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	newMap := make(map[string]ChannelAccount)
	for _, acc := range accounts {
		if acc.ID == "" {
			acc.ID = fmt.Sprintf("%s_%d", acc.Channel, len(newMap)+1)
		}
		newMap[acc.ID] = acc
	}
	m.accounts = newMap
	return nil
}

// GetAccountStatuses returns in-memory runtime health.
func (m *ChannelManager) GetAccountStatuses() map[string]AccountStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]AccountStatus, len(m.statuses))
	for id, st := range m.statuses {
		out[id] = st
	}
	return out
}

// GetAccounts returns all registered accounts.
func (m *ChannelManager) GetAccounts() []ChannelAccount {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list := make([]ChannelAccount, 0, len(m.accounts))
	for _, acc := range m.accounts {
		list = append(list, acc)
	}
	return list
}

// GetAccountByID returns an account by ID.
func (m *ChannelManager) GetAccountByID(id string) (ChannelAccount, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	acc, ok := m.accounts[id]
	return acc, ok
}

// FindBoundAgent resolves which agent should handle an inbound message on a given account.
func (m *ChannelManager) FindBoundAgent(channelID, accountID string) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if acc, ok := m.accounts[accountID]; ok {
		if len(acc.BoundAgentIDs) == 1 && acc.BoundAgentIDs[0] != "*" && acc.BoundAgentIDs[0] != "all" {
			return acc.BoundAgentIDs[0]
		}
	}
	return ""
}

// Send implements tools.ChannelMessageSender.
func (m *ChannelManager) Send(ctx context.Context, channelID, accountID, recipient, content string) error {
	return m.SendMessage(ctx, OutboundMessage{
		ChannelID: channelID,
		AccountID: accountID,
		Recipient: recipient,
		Content:   content,
	})
}

// SendMessage dispatches an outbound message to matching channel adapters.
func (m *ChannelManager) SendMessage(ctx context.Context, msg OutboundMessage) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	targetChannel := strings.ToLower(strings.TrimSpace(msg.ChannelID))
	if targetChannel == "" {
		targetChannel = "all"
	}

	var errs []string
	dispatched := 0

	for name, adapter := range m.adapters {
		if targetChannel == "all" || targetChannel == name {
			if err := adapter.SendMessage(ctx, msg); err != nil {
				errs = append(errs, fmt.Sprintf("adapter %s: %v", name, err))
			} else {
				dispatched++
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("dispatch errors (%d succeeded): %s", dispatched, strings.Join(errs, "; "))
	}
	return nil
}

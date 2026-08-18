package channels

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/actonos/actonos/internal/bus"
)

// ChannelManager coordinates multi-account messaging adapters across Telegram, WhatsApp, Discord, etc.
type ChannelManager struct {
	mu          sync.RWMutex
	eventBus    *bus.EventBus
	pairingMgr  *PairingManager
	accounts    map[string]ChannelAccount // keyed by account ID
	tgAdapters  map[string]*TelegramAdapter
	waAdapters  map[string]*WhatsAppAdapter
	dcAdapters  map[string]*DiscordAdapter
	ctx         context.Context
	cancel      context.CancelFunc
}

// NewChannelManager initializes the multi-account channel manager.
func NewChannelManager(eventBus *bus.EventBus, pairingMgr *PairingManager) *ChannelManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &ChannelManager{
		eventBus:   eventBus,
		pairingMgr: pairingMgr,
		accounts:   make(map[string]ChannelAccount),
		tgAdapters: make(map[string]*TelegramAdapter),
		waAdapters: make(map[string]*WhatsAppAdapter),
		dcAdapters: make(map[string]*DiscordAdapter),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Start begins listening on all registered channel accounts.
func (m *ChannelManager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for id, acc := range m.accounts {
		if !acc.Enabled {
			continue
		}
		m.startAccountAdapterLocked(ctx, acc)
		slog.Info("channel account started", "channel", acc.Channel, "account_id", id, "name", acc.Name)
	}
	return nil
}

// Stop cleanly terminates all running channel adapters.
func (m *ChannelManager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancel()
	for _, a := range m.tgAdapters {
		_ = a.Stop()
	}
	for _, a := range m.waAdapters {
		_ = a.Stop()
	}
	for _, a := range m.dcAdapters {
		_ = a.Stop()
	}
	return nil
}

// SyncAccounts dynamically synchronizes and starts/stops account adapters.
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

	// Stop removed or disabled adapters
	for id, acc := range m.accounts {
		newAcc, exists := newMap[id]
		if !exists || !newAcc.Enabled || newAcc.Token != acc.Token {
			if tg, ok := m.tgAdapters[id]; ok {
				_ = tg.Stop()
				delete(m.tgAdapters, id)
			}
			if wa, ok := m.waAdapters[id]; ok {
				_ = wa.Stop()
				delete(m.waAdapters, id)
			}
			if dc, ok := m.dcAdapters[id]; ok {
				_ = dc.Stop()
				delete(m.dcAdapters, id)
			}
		}
	}

	m.accounts = newMap

	// Start or update active adapters
	for _, acc := range newMap {
		if !acc.Enabled {
			continue
		}
		switch acc.Channel {
		case "telegram":
			if _, running := m.tgAdapters[acc.ID]; !running && acc.Token != "" {
				m.startTelegramAccountLocked(ctx, acc)
			}
		case "whatsapp":
			if _, running := m.waAdapters[acc.ID]; !running && acc.Token != "" {
				m.startWhatsAppAccountLocked(ctx, acc)
			}
		case "discord":
			if _, running := m.dcAdapters[acc.ID]; !running && acc.Token != "" {
				m.startDiscordAccountLocked(ctx, acc)
			}
		}
	}

	return nil
}

func (m *ChannelManager) startAccountAdapterLocked(ctx context.Context, acc ChannelAccount) {
	switch acc.Channel {
	case "telegram":
		m.startTelegramAccountLocked(ctx, acc)
	case "whatsapp":
		m.startWhatsAppAccountLocked(ctx, acc)
	case "discord":
		m.startDiscordAccountLocked(ctx, acc)
	}
}

func (m *ChannelManager) startTelegramAccountLocked(_ context.Context, acc ChannelAccount) {
	if acc.Token == "" {
		return
	}
	adapterCtx := m.ctx
	if adapterCtx == nil || adapterCtx.Err() != nil {
		m.ctx, m.cancel = context.WithCancel(context.Background())
		adapterCtx = m.ctx
	}
	adapter := NewTelegramAdapter(acc.Token, m.eventBus, m.pairingMgr)
	adapter.SetAccountID(acc.ID)
	if err := adapter.Start(adapterCtx); err == nil {
		m.tgAdapters[acc.ID] = adapter
	} else {
		slog.Warn("failed to start telegram account adapter", "account_id", acc.ID, "error", err)
	}
}

func (m *ChannelManager) startWhatsAppAccountLocked(_ context.Context, acc ChannelAccount) {
	if acc.Token == "" {
		return
	}
	adapterCtx := m.ctx
	if adapterCtx == nil || adapterCtx.Err() != nil {
		m.ctx, m.cancel = context.WithCancel(context.Background())
		adapterCtx = m.ctx
	}
	secret := acc.WebhookSecret
	if secret == "" {
		secret = "acton_verify_token"
	}
	adapter := NewWhatsAppAdapter(acc.Token, acc.PhoneID, secret, m.eventBus, m.pairingMgr)
	if err := adapter.Start(adapterCtx); err == nil {
		m.waAdapters[acc.ID] = adapter
	} else {
		slog.Warn("failed to start whatsapp account adapter", "account_id", acc.ID, "error", err)
	}
}

func (m *ChannelManager) startDiscordAccountLocked(_ context.Context, acc ChannelAccount) {
	if acc.Token == "" {
		return
	}
	adapterCtx := m.ctx
	if adapterCtx == nil || adapterCtx.Err() != nil {
		m.ctx, m.cancel = context.WithCancel(context.Background())
		adapterCtx = m.ctx
	}
	adapter := NewDiscordAdapter(acc.Token, m.eventBus)
	if err := adapter.Start(adapterCtx); err == nil {
		m.dcAdapters[acc.ID] = adapter
	} else {
		slog.Warn("failed to start discord account adapter", "account_id", acc.ID, "error", err)
	}
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

// SendMessage dispatches an outbound message according to target channel, account, and recipient.
func (m *ChannelManager) SendMessage(ctx context.Context, msg OutboundMessage) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	targetChannel := strings.ToLower(strings.TrimSpace(msg.ChannelID))
	if targetChannel == "" {
		targetChannel = "all"
	}
	targetAccount := strings.TrimSpace(msg.AccountID)

	var errs []string

	// Telegram Dispatching
	if targetChannel == "all" || targetChannel == "telegram" {
		for id, adapter := range m.tgAdapters {
			if targetAccount != "" && targetAccount != "all" && targetAccount != id {
				continue
			}
			acc := m.accounts[id]
			recipients := m.resolveRecipients(acc, adapter.GetLastChatID(), adapter.GetKnownChatIDs(), msg.Recipient)
			for _, r := range recipients {
				if err := adapter.SendMessage(ctx, OutboundMessage{
					ChannelID: "telegram",
					AccountID: id,
					Recipient: r,
					Content:   msg.Content,
				}); err != nil {
					errs = append(errs, fmt.Sprintf("telegram account %s: %v", id, err))
				}
			}
		}
	}

	// WhatsApp Dispatching
	if targetChannel == "all" || targetChannel == "whatsapp" {
		for id, adapter := range m.waAdapters {
			if targetAccount != "" && targetAccount != "all" && targetAccount != id {
				continue
			}
			acc := m.accounts[id]
			recipients := m.resolveRecipients(acc, "", nil, msg.Recipient)
			for _, r := range recipients {
				if err := adapter.SendMessage(ctx, OutboundMessage{
					ChannelID: "whatsapp",
					AccountID: id,
					Recipient: r,
					Content:   msg.Content,
				}); err != nil {
					errs = append(errs, fmt.Sprintf("whatsapp account %s: %v", id, err))
				}
			}
		}
	}

	// Discord Dispatching
	if targetChannel == "all" || targetChannel == "discord" {
		for id, adapter := range m.dcAdapters {
			if targetAccount != "" && targetAccount != "all" && targetAccount != id {
				continue
			}
			acc := m.accounts[id]
			recipients := m.resolveRecipients(acc, "", nil, msg.Recipient)
			for _, r := range recipients {
				if err := adapter.SendMessage(ctx, OutboundMessage{
					ChannelID: "discord",
					AccountID: id,
					Recipient: r,
					Content:   msg.Content,
				}); err != nil {
					errs = append(errs, fmt.Sprintf("discord account %s: %v", id, err))
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("dispatch errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (m *ChannelManager) resolveRecipients(acc ChannelAccount, lastID string, knownIDs []string, explicitRecipient string) []string {
	if explicitRecipient != "" && explicitRecipient != "all" {
		return []string{explicitRecipient}
	}

	var results []string
	if acc.DefaultChatID != "" {
		results = append(results, acc.DefaultChatID)
	}
	if lastID != "" && !contains(results, lastID) {
		results = append(results, lastID)
	}
	for _, k := range knownIDs {
		if !contains(results, k) {
			results = append(results, k)
		}
	}

	// If explicit recipient was "all" and we have pairing manager, also add authorized users
	if (explicitRecipient == "all" || len(results) == 0) && m.pairingMgr != nil {
		paired := m.pairingMgr.ListAuthorized(acc.Channel)
		for _, p := range paired {
			if !contains(results, p.SenderID) {
				results = append(results, p.SenderID)
			}
		}
	}

	return results
}

func contains(arr []string, item string) bool {
	for _, s := range arr {
		if s == item {
			return true
		}
	}
	return false
}

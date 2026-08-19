package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

// TelegramAdapter handles two-way messaging with the Telegram Bot API via Long-Polling or Outbound.
type TelegramAdapter struct {
	mu             sync.RWMutex
	token          string
	accountID      string
	apiBaseURL     string
	bus            *bus.EventBus
	pairingMgr     *PairingManager
	client         *http.Client
	running        bool
	stopChan       chan struct{}
	lastUpdateID   int64
	lastChatID     string
	chatIDs        map[string]bool
	unauthNotified map[string]time.Time
	pollFailing    bool // tracks whether the last getUpdates poll failed, to publish health events only on state transitions
}

// NewTelegramAdapter creates a new TelegramAdapter.
func NewTelegramAdapter(token string, bus *bus.EventBus, pairingMgr *PairingManager) *TelegramAdapter {
	return &TelegramAdapter{
		token:          token,
		bus:            bus,
		pairingMgr:     pairingMgr,
		client:         &http.Client{Timeout: 35 * time.Second},
		stopChan:       make(chan struct{}),
		chatIDs:        make(map[string]bool),
		unauthNotified: make(map[string]time.Time),
	}
}

func (t *TelegramAdapter) Name() string { return "telegram" }

// reportPollHealth publishes a channel adapter health event only on a state
// transition (first failure after being healthy, or first success after a
// run of failures), so a persistently broken poll loop doesn't spam a new
// event every ~500ms cycle while still surfacing the failure promptly.
func (t *TelegramAdapter) reportPollHealth(pollErr error) {
	t.mu.Lock()
	wasFailing := t.pollFailing
	t.pollFailing = pollErr != nil
	accountID := t.accountID
	t.mu.Unlock()

	if t.bus == nil || accountID == "" {
		return
	}
	if pollErr != nil && !wasFailing {
		t.bus.Publish(bus.NewEvent(bus.EventChannelAdapterError, accountID, map[string]any{
			"channel":    "telegram",
			"account_id": accountID,
			"error":      pollErr.Error(),
		}))
	} else if pollErr == nil && wasFailing {
		t.bus.Publish(bus.NewEvent(bus.EventChannelAdapterRecovered, accountID, map[string]any{
			"channel":    "telegram",
			"account_id": accountID,
		}))
	}
}

// SetAPIBaseURL configures a custom Telegram API gateway or reverse proxy endpoint.
func (t *TelegramAdapter) SetAPIBaseURL(url string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.apiBaseURL = strings.TrimSpace(url)
}

func (t *TelegramAdapter) getAPIBase() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.apiBaseURL != "" {
		return strings.TrimRight(t.apiBaseURL, "/")
	}
	if env := os.Getenv("TELEGRAM_API_BASE"); env != "" {
		return strings.TrimRight(env, "/")
	}
	return "https://api.telegram.org"
}

// SetAccountID sets the account identifier for this adapter.
func (t *TelegramAdapter) SetAccountID(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.accountID = id
}

// GetAccountID returns the account identifier for this adapter.
func (t *TelegramAdapter) GetAccountID() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.accountID
}

// UpdateToken updates the bot token dynamically.
func (t *TelegramAdapter) UpdateToken(token string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.token = strings.TrimSpace(token)
}

// RestartWithToken dynamically swaps token and re-initiates the background polling loop.
func (t *TelegramAdapter) RestartWithToken(token string) error {
	_ = t.Stop()
	t.UpdateToken(token)
	if strings.TrimSpace(token) == "" {
		return nil
	}
	return t.Start(context.Background())
}

// GetLastChatID returns the most recently active Telegram chat ID.
func (t *TelegramAdapter) GetLastChatID() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.lastChatID
}

// GetKnownChatIDs returns all chat IDs that have messaged the bot.
func (t *TelegramAdapter) GetKnownChatIDs() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	res := make([]string, 0, len(t.chatIDs))
	for id := range t.chatIDs {
		res = append(res, id)
	}
	return res
}

// Start begins the background long-polling loop.
func (t *TelegramAdapter) Start(ctx context.Context) error {
	t.mu.Lock()
	if t.token == "" {
		t.mu.Unlock()
		slog.Info("telegram channel adapter: no token provided, idle mode")
		return nil
	}
	if t.running {
		t.mu.Unlock()
		return nil
	}
	t.running = true
	t.stopChan = make(chan struct{})
	t.mu.Unlock()

	slog.Info("telegram channel adapter started (long-polling active)")
	go t.pollLoop(ctx)
	return nil
}

// Stop gracefully terminates the polling loop.
func (t *TelegramAdapter) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		close(t.stopChan)
		t.running = false
		slog.Info("telegram channel adapter stopped")
	}
	return nil
}

func (t *TelegramAdapter) pollLoop(ctx context.Context) {
	defer func() {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.stopChan:
			return
		default:
			t.fetchUpdates(ctx)
			time.Sleep(500 * time.Millisecond)
		}
	}
}

type tgUpdate struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		MessageID int64 `json:"message_id"`
		From      *struct {
			ID        int64  `json:"id"`
			FirstName string `json:"first_name"`
			Username  string `json:"username"`
		} `json:"from"`
		Chat struct {
			ID   int64  `json:"id"`
			Type string `json:"type"`
		} `json:"chat"`
		Text string `json:"text"`
	} `json:"message"`
}

type tgResponse struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

func (t *TelegramAdapter) fetchUpdates(ctx context.Context) {
	t.mu.RLock()
	token := t.token
	offset := t.lastUpdateID + 1
	t.mu.RUnlock()

	if token == "" {
		return
	}

	url := fmt.Sprintf("%s/bot%s/getUpdates?offset=%d&timeout=20", t.getAPIBase(), token, offset)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return
	}

	resp, err := t.client.Do(req)
	if err != nil {
		if ctx.Err() == nil {
			slog.Warn("telegram getUpdates network error", "account_id", t.GetAccountID(), "error", err)
			t.reportPollHealth(fmt.Errorf("network error: %w", err))
		}
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		slog.Warn("telegram getUpdates non-200 response", "account_id", t.GetAccountID(), "status", resp.StatusCode, "body", string(body))
		t.reportPollHealth(fmt.Errorf("telegram API returned status %d", resp.StatusCode))
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var tgResp tgResponse
	if err := json.Unmarshal(body, &tgResp); err != nil || !tgResp.OK {
		slog.Warn("telegram getUpdates unmarshal failed or !OK", "account_id", t.GetAccountID(), "error", err, "body", string(body))
		t.reportPollHealth(errors.New("telegram getUpdates returned an unexpected/failed response"))
		return
	}
	t.reportPollHealth(nil)

	for _, upd := range tgResp.Result {
		t.mu.Lock()
		if upd.UpdateID > t.lastUpdateID {
			t.lastUpdateID = upd.UpdateID
		}
		t.mu.Unlock()

		if upd.Message == nil || upd.Message.From == nil || upd.Message.Text == "" {
			continue
		}

		slog.Info("telegram message received", "from_id", upd.Message.From.ID, "chat_id", upd.Message.Chat.ID, "user", upd.Message.From.Username, "text", upd.Message.Text)
		t.handleInboundMessage(ctx, upd)
	}
}

func (t *TelegramAdapter) handleInboundMessage(ctx context.Context, upd tgUpdate) {
	senderID := strconv.FormatInt(upd.Message.From.ID, 10)
	chatID := strconv.FormatInt(upd.Message.Chat.ID, 10)
	senderName := upd.Message.From.Username
	if senderName == "" {
		senderName = upd.Message.From.FirstName
	}
	text := strings.TrimSpace(upd.Message.Text)

	// Save active chat ID for proactive push / reminders
	t.mu.Lock()
	t.lastChatID = chatID
	if t.chatIDs == nil {
		t.chatIDs = make(map[string]bool)
	}
	t.chatIDs[chatID] = true
	t.mu.Unlock()

	// Check Authorization
	isAuth := false
	if t.pairingMgr != nil {
		isAuth = t.pairingMgr.IsAuthorized("telegram", senderID)
	} else {
		isAuth = true // If no pairing manager, allow by default
	}

	if !isAuth {
		// Attempt pairing check
		pin := ExtractPairingPIN(text)
		if pin != "" && t.pairingMgr != nil {
			paired, err := t.pairingMgr.ValidateAndPair("telegram", pin, senderID, senderName)
			if err == nil && paired {
				slog.Info("telegram user paired successfully", "sender_id", senderID, "name", senderName)
				_ = t.SendMessage(ctx, OutboundMessage{
					ChannelID: "telegram",
					Recipient: chatID,
					Content:   fmt.Sprintf("🎉 Authentication successful!\n\nWelcome %s, your Telegram account is now paired with ActonOS Kernel. You can send prompts and commands anytime.", senderName),
				})
				return
			}
		}

		slog.Warn("unauthorized telegram message received; prompting for pairing", "sender_id", senderID, "chat_id", chatID)

		t.mu.Lock()
		if t.unauthNotified == nil {
			t.unauthNotified = make(map[string]time.Time)
		}
		lastSent, exists := t.unauthNotified[senderID]
		now := time.Now()
		if exists && now.Sub(lastSent) < 15*time.Second {
			t.mu.Unlock()
			return
		}
		t.unauthNotified[senderID] = now
		t.mu.Unlock()

		_ = t.SendMessage(ctx, OutboundMessage{
			ChannelID: "telegram",
			AccountID: t.GetAccountID(),
			Recipient: chatID,
			Content:   fmt.Sprintf("🔒 Unauthorized Access to ActonOS.\n\nPlease generate a 6-digit Pairing PIN on your ActonOS Web UI (Integrations -> Channel Pairing) and send it here to authenticate.\n(Your Sender ID: %s)", senderID),
		})
		return
	}

	// Update active time
	if t.pairingMgr != nil {
		t.pairingMgr.TouchUser("telegram", senderID)
	}

	// Publish to Bus
	if t.bus != nil {
		t.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, "telegram", InboundMessage{
			ChannelID:   "telegram",
			AccountID:   t.GetAccountID(),
			SenderID:    senderID,
			SenderName:  senderName,
			TargetAgent: "default",
			Content:     text,
			Metadata: map[string]string{
				"chat_id":    chatID,
				"message_id": strconv.FormatInt(upd.Message.MessageID, 10),
			},
		}))
	}
}

// SendMessage sends an outbound message to a Telegram chat with automatic Markdown, plain text fallback, and chunking for long messages.
func (t *TelegramAdapter) SendMessage(ctx context.Context, msg OutboundMessage) error {
	t.mu.RLock()
	token := t.token
	t.mu.RUnlock()

	if token == "" {
		return fmt.Errorf("telegram token not configured")
	}

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	// Telegram max message limit is 4096 chars. Chunk if longer than 3900.
	const maxChunkLen = 3900
	if len(content) <= maxChunkLen {
		return t.sendSingleMessage(ctx, token, msg.Recipient, content)
	}

	chunks := splitTextIntoChunks(content, maxChunkLen)
	for _, chunk := range chunks {
		if err := t.sendSingleMessage(ctx, token, msg.Recipient, chunk); err != nil {
			return err
		}
		time.Sleep(150 * time.Millisecond) // small delay to respect rate limits
	}
	return nil
}

func (t *TelegramAdapter) sendSingleMessage(ctx context.Context, token, recipient, text string) error {
	url := fmt.Sprintf("%s/bot%s/sendMessage", t.getAPIBase(), token)

	// 1. Try sending with Markdown formatting
	bodyMD := map[string]any{
		"chat_id":    recipient,
		"text":       text,
		"parse_mode": "Markdown",
	}
	dataMD, _ := json.Marshal(bodyMD)

	reqMD, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(dataMD))
	if err == nil {
		reqMD.Header.Set("Content-Type", "application/json")
		if resp, err := t.client.Do(reqMD); err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
	}

	// 2. Fallback: Send plain text without parse_mode
	bodyPlain := map[string]any{
		"chat_id": recipient,
		"text":    text,
	}
	dataPlain, err := json.Marshal(bodyPlain)
	if err != nil {
		return err
	}

	reqPlain, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(dataPlain))
	if err != nil {
		return err
	}
	reqPlain.Header.Set("Content-Type", "application/json")

	respPlain, err := t.client.Do(reqPlain)
	if err != nil {
		return fmt.Errorf("sending plain telegram message: %w", err)
	}
	defer respPlain.Body.Close()

	if respPlain.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(respPlain.Body)
		return fmt.Errorf("telegram api error (%d): %s", respPlain.StatusCode, string(respBody))
	}

	return nil
}

func splitTextIntoChunks(text string, maxLen int) []string {
	if len(text) <= maxLen {
		return []string{text}
	}

	var chunks []string
	remaining := text

	for len(remaining) > 0 {
		if len(remaining) <= maxLen {
			chunks = append(chunks, remaining)
			break
		}

		// Try to find a clean paragraph or newline split
		splitIdx := strings.LastIndex(remaining[:maxLen], "\n\n")
		if splitIdx == -1 {
			splitIdx = strings.LastIndex(remaining[:maxLen], "\n")
		}
		if splitIdx == -1 || splitIdx < maxLen/2 {
			splitIdx = strings.LastIndex(remaining[:maxLen], " ")
		}
		if splitIdx == -1 {
			splitIdx = maxLen
		}

		chunk := strings.TrimSpace(remaining[:splitIdx])
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		remaining = strings.TrimSpace(remaining[splitIdx:])
	}

	return chunks
}

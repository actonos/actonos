package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/coder/websocket"
)

// DiscordAdapter handles two-way messaging with Discord via Gateway WebSocket and REST API / Webhooks.
type DiscordAdapter struct {
	mu             sync.RWMutex
	token          string
	accountID      string
	bus            *bus.EventBus
	pairingMgr     *PairingManager
	client         *http.Client
	running        bool
	stopChan       chan struct{}
	isWebhook      bool
	lastChannelID  string
	unauthNotified map[string]time.Time
	seq            atomic.Int64
	gatewayFailing bool // tracks whether the gateway is currently failing to (re)connect, to publish health events only on state transitions
}

// NewDiscordAdapter creates a new DiscordAdapter supporting both Bot Tokens and Webhooks.
func NewDiscordAdapter(token string, bus *bus.EventBus, pairingMgr ...*PairingManager) *DiscordAdapter {
	trimmed := strings.TrimSpace(token)
	isWebhook := strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
	var pm *PairingManager
	if len(pairingMgr) > 0 {
		pm = pairingMgr[0]
	}
	return &DiscordAdapter{
		token:          trimmed,
		bus:            bus,
		pairingMgr:     pm,
		client:         &http.Client{Timeout: 30 * time.Second},
		isWebhook:      isWebhook,
		unauthNotified: make(map[string]time.Time),
	}
}

func (d *DiscordAdapter) Name() string { return "discord" }

// SetAccountID sets the account identifier.
func (d *DiscordAdapter) SetAccountID(id string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.accountID = id
}

// GetAccountID returns the account identifier.
func (d *DiscordAdapter) GetAccountID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.accountID
}

// GetLastChannelID returns the most recently active Discord channel ID.
func (d *DiscordAdapter) GetLastChannelID() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.lastChannelID
}

// reportGatewayHealth publishes a channel adapter health event only on a
// state transition (first dial failure after being connected, or first
// successful (re)connect after a run of failures), so a persistently broken
// gateway doesn't spam a new event on every backoff retry.
func (d *DiscordAdapter) reportGatewayHealth(dialErr error) {
	d.mu.Lock()
	wasFailing := d.gatewayFailing
	d.gatewayFailing = dialErr != nil
	accountID := d.accountID
	d.mu.Unlock()

	if d.bus == nil || accountID == "" {
		return
	}
	if dialErr != nil && !wasFailing {
		d.bus.Publish(bus.NewEvent(bus.EventChannelAdapterError, accountID, map[string]any{
			"channel":    "discord",
			"account_id": accountID,
			"error":      dialErr.Error(),
		}))
	} else if dialErr == nil && wasFailing {
		d.bus.Publish(bus.NewEvent(bus.EventChannelAdapterRecovered, accountID, map[string]any{
			"channel":    "discord",
			"account_id": accountID,
		}))
	}
}

// Start begins the Discord connection (Gateway WebSocket for bots, or ready state for webhooks).
func (d *DiscordAdapter) Start(ctx context.Context) error {
	d.mu.Lock()
	if d.token == "" {
		d.mu.Unlock()
		slog.Info("discord channel adapter: no token or webhook provided, idle mode")
		return nil
	}
	if d.running {
		d.mu.Unlock()
		return nil
	}
	d.running = true
	d.stopChan = make(chan struct{})
	d.mu.Unlock()

	if d.isWebhook {
		slog.Info("discord channel adapter started (webhook mode)")
		return nil
	}

	slog.Info("discord channel adapter starting (gateway bot mode)...")
	go d.gatewayLoop(ctx)
	return nil
}

// Stop terminates the gateway connection.
func (d *DiscordAdapter) Stop() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.running {
		close(d.stopChan)
		d.running = false
		slog.Info("discord channel adapter stopped")
	}
	return nil
}

type discordGatewayPayload struct {
	Op int             `json:"op"`
	D  json.RawMessage `json:"d"`
	S  *int64          `json:"s,omitempty"`
	T  string          `json:"t,omitempty"`
}

func (d *DiscordAdapter) gatewayLoop(ctx context.Context) {
	defer func() {
		d.mu.Lock()
		d.running = false
		d.mu.Unlock()
	}()

	gatewayURL := "wss://gateway.discord.gg/?v=10&encoding=json"
	backoff := 2 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		default:
		}

		connCtx, connCancel := context.WithCancel(ctx)
		wsConn, _, err := websocket.Dial(connCtx, gatewayURL, nil)
		if err != nil {
			slog.Warn("discord gateway connect failed; retrying", "account_id", d.GetAccountID(), "error", err, "retry_in", backoff.String())
			d.reportGatewayHealth(err)
			connCancel()
			select {
			case <-ctx.Done():
				return
			case <-d.stopChan:
				return
			case <-time.After(backoff):
				if backoff < 60*time.Second {
					backoff *= 2
				}
				continue
			}
		}

		backoff = 2 * time.Second
		d.reportGatewayHealth(nil)
		d.runGatewaySession(connCtx, wsConn)
		connCancel()
		_ = wsConn.Close(websocket.StatusNormalClosure, "reconnecting")

		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func (d *DiscordAdapter) runGatewaySession(ctx context.Context, wsConn *websocket.Conn) {
	heartbeatDone := make(chan struct{})
	defer close(heartbeatDone)

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		default:
		}

		_, data, err := wsConn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				slog.Debug("discord gateway read connection closed", "account_id", d.GetAccountID(), "error", err)
			}
			return
		}

		var payload discordGatewayPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			continue
		}

		if payload.S != nil {
			d.seq.Store(*payload.S)
		}

		switch payload.Op {
		case 10: // Hello
			var hello struct {
				HeartbeatInterval int64 `json:"heartbeat_interval"`
			}
			if err := json.Unmarshal(payload.D, &hello); err == nil && hello.HeartbeatInterval > 0 {
				go d.heartbeatLoop(ctx, wsConn, hello.HeartbeatInterval, heartbeatDone)
			}
			// Send Identify (Opcode 2)
			d.sendIdentify(ctx, wsConn)

		case 1: // Heartbeat requested by server
			d.sendHeartbeat(ctx, wsConn)

		case 11: // Heartbeat ACK
			// Heartbeat acknowledged

		case 0: // Dispatch Event
			d.handleDispatch(ctx, payload.T, payload.D)
		}
	}
}

func (d *DiscordAdapter) heartbeatLoop(ctx context.Context, wsConn *websocket.Conn, intervalMs int64, done <-chan struct{}) {
	ticker := time.NewTicker(time.Duration(intervalMs) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-d.stopChan:
			return
		case <-done:
			return
		case <-ticker.C:
			d.sendHeartbeat(ctx, wsConn)
		}
	}
}

func (d *DiscordAdapter) sendHeartbeat(ctx context.Context, wsConn *websocket.Conn) {
	hb := map[string]any{
		"op": 1,
		"d":  d.seq.Load(),
	}
	data, _ := json.Marshal(hb)
	_ = wsConn.Write(ctx, websocket.MessageText, data)
}

func (d *DiscordAdapter) sendIdentify(ctx context.Context, wsConn *websocket.Conn) {
	d.mu.RLock()
	rawTok := d.token
	d.mu.RUnlock()

	cleanTok := strings.TrimPrefix(rawTok, "Bot ")
	cleanTok = strings.TrimSpace(cleanTok)

	identify := map[string]any{
		"op": 2,
		"d": map[string]any{
			"token":   cleanTok,
			"intents": 37377, // GUILDS (1) | GUILD_MESSAGES (512) | DIRECT_MESSAGES (4096) | MESSAGE_CONTENT (32768)
			"properties": map[string]any{
				"os":      "linux",
				"browser": "ActonOS",
				"device":  "ActonOS",
			},
		},
	}
	data, _ := json.Marshal(identify)
	_ = wsConn.Write(ctx, websocket.MessageText, data)
}

func (d *DiscordAdapter) handleDispatch(ctx context.Context, eventType string, eventData json.RawMessage) {
	switch eventType {
	case "READY":
		var ready struct {
			User struct {
				ID       string `json:"id"`
				Username string `json:"username"`
			} `json:"user"`
		}
		if err := json.Unmarshal(eventData, &ready); err == nil {
			slog.Info("discord bot connected & online", "account_id", d.GetAccountID(), "bot_username", ready.User.Username, "bot_id", ready.User.ID)
		}

	case "MESSAGE_CREATE":
		var msg struct {
			ID        string `json:"id"`
			ChannelID string `json:"channel_id"`
			GuildID   string `json:"guild_id"`
			Content   string `json:"content"`
			Author    struct {
				ID       string `json:"id"`
				Username string `json:"username"`
				Bot      bool   `json:"bot"`
			} `json:"author"`
		}
		if err := json.Unmarshal(eventData, &msg); err != nil || msg.Author.Bot || msg.Content == "" {
			return
		}

		senderID := msg.Author.ID
		senderName := msg.Author.Username
		text := strings.TrimSpace(msg.Content)
		channelID := msg.ChannelID

		d.mu.Lock()
		d.lastChannelID = channelID
		d.mu.Unlock()

		// Pairing & Zero-Trust Check
		isAuth := false
		if d.pairingMgr != nil {
			isAuth = d.pairingMgr.IsAuthorized("discord", senderID)
		} else {
			isAuth = true
		}

		if !isAuth {
			pin := ExtractPairingPIN(text)
			if pin != "" && d.pairingMgr != nil {
				paired, err := d.pairingMgr.ValidateAndPair("discord", pin, senderID, senderName)
				if err == nil && paired {
					slog.Info("discord user paired successfully", "sender_id", senderID, "name", senderName)
					_ = d.SendMessage(ctx, OutboundMessage{
						ChannelID: "discord",
						Recipient: channelID,
						Content:   fmt.Sprintf("🎉 Authentication successful!\n\nWelcome %s, your Discord account is now paired with ActonOS Kernel. You can send prompts and commands anytime.", senderName),
					})
					return
				}
			}

			slog.Warn("unauthorized discord message received; prompting for pairing", "sender_id", senderID, "channel_id", channelID, "text", text)

			d.mu.Lock()
			if d.unauthNotified == nil {
				d.unauthNotified = make(map[string]time.Time)
			}
			lastSent, exists := d.unauthNotified[senderID]
			now := time.Now()
			if exists && now.Sub(lastSent) < 15*time.Second {
				d.mu.Unlock()
				return
			}
			d.unauthNotified[senderID] = now
			d.mu.Unlock()

			_ = d.SendMessage(ctx, OutboundMessage{
				ChannelID: "discord",
				AccountID: d.GetAccountID(),
				Recipient: channelID,
				Content:   fmt.Sprintf("🔒 Unauthorized Access to ActonOS.\n\nPlease generate a 6-digit Pairing PIN on your ActonOS Web UI (Integrations -> Channel Pairing) and send it here to authenticate.\n(Your Sender ID: %s)", senderID),
			})
			return
		}

		if d.pairingMgr != nil {
			d.pairingMgr.TouchUser("discord", senderID)
		}

		slog.Info("discord inbound message received", "author", senderName, "channel_id", channelID, "text", text)

		if d.bus != nil {
			d.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, "discord", InboundMessage{
				ChannelID:   "discord",
				AccountID:   d.GetAccountID(),
				SenderID:    senderID,
				SenderName:  senderName,
				TargetAgent: "default",
				Content:     text,
				Metadata: map[string]string{
					"chat_id":    channelID,
					"channel_id": channelID,
					"guild_id":   msg.GuildID,
					"message_id": msg.ID,
				},
			}))
		}
	}
}

// SendMessage sends an outbound message to a Discord channel (or Webhook).
func (d *DiscordAdapter) SendMessage(ctx context.Context, msg OutboundMessage) error {
	d.mu.RLock()
	tok := d.token
	isWebhook := d.isWebhook
	lastChan := d.lastChannelID
	d.mu.RUnlock()

	content := strings.TrimSpace(msg.Content)
	if content == "" {
		return nil
	}

	// 1. Webhook Mode
	if isWebhook {
		url := tok
		if msg.Recipient != "" && msg.Recipient != "default" && strings.HasPrefix(msg.Recipient, "http") {
			url = msg.Recipient
		}
		return d.postWebhook(ctx, url, content)
	}

	targetChannel := msg.Recipient
	if targetChannel == "" || targetChannel == "default" || targetChannel == "all" {
		targetChannel = lastChan
	}
	if targetChannel == "" && d.pairingMgr != nil {
		paired := d.pairingMgr.ListAuthorized("discord")
		if len(paired) > 0 {
			targetChannel = paired[0].SenderID
		}
	}
	if targetChannel == "" {
		return fmt.Errorf("no discord recipient channel specified")
	}

	// Discord max message length is 2000 chars. Chunk if longer.
	const maxDiscordLen = 1900
	if len(content) <= maxDiscordLen {
		return d.sendBotMessage(ctx, tok, targetChannel, content)
	}

	chunks := splitTextIntoChunks(content, maxDiscordLen)
	for _, chunk := range chunks {
		if err := d.sendBotMessage(ctx, tok, targetChannel, chunk); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil
}

func (d *DiscordAdapter) postWebhook(ctx context.Context, webhookURL, content string) error {
	body := map[string]string{"content": content}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, webhookURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending discord webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discord webhook returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

func (d *DiscordAdapter) getOrCreateDMChannel(ctx context.Context, token, recipientID string) (string, error) {
	cleanTok := strings.TrimPrefix(token, "Bot ")
	cleanTok = strings.TrimSpace(cleanTok)

	url := "https://discord.com/api/v10/users/@me/channels"
	body := map[string]string{"recipient_id": recipientID}
	data, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bot "+cleanTok)
	req.Header.Set("Content-Type", "application/json")

	resp, err := d.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("opening discord DM channel: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("discord open DM returned %d: %s", resp.StatusCode, string(respBody))
	}

	var dm struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(respBody, &dm); err != nil || dm.ID == "" {
		return "", fmt.Errorf("invalid dm response from discord: %s", string(respBody))
	}
	return dm.ID, nil
}

func (d *DiscordAdapter) sendBotMessage(ctx context.Context, token, channelOrUserID, content string) error {
	cleanTok := strings.TrimPrefix(token, "Bot ")
	cleanTok = strings.TrimSpace(cleanTok)

	sendToChannel := func(chanID string) (int, []byte, error) {
		url := fmt.Sprintf("https://discord.com/api/v10/channels/%s/messages", chanID)
		body := map[string]string{"content": content}
		data, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
		if err != nil {
			return 0, nil, err
		}
		req.Header.Set("Authorization", "Bot "+cleanTok)
		req.Header.Set("Content-Type", "application/json")

		resp, err := d.client.Do(req)
		if err != nil {
			return 0, nil, fmt.Errorf("sending discord bot message: %w", err)
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, respBody, nil
	}

	statusCode, respBody, err := sendToChannel(channelOrUserID)
	if err != nil {
		return err
	}

	// If 404 Unknown Channel (code 10003), channelOrUserID is likely a User ID — create/fetch the DM channel and retry
	if statusCode == http.StatusNotFound && (strings.Contains(string(respBody), "10003") || strings.Contains(string(respBody), "Unknown Channel")) {
		dmChanID, dmErr := d.getOrCreateDMChannel(ctx, token, channelOrUserID)
		if dmErr == nil && dmChanID != "" {
			d.mu.Lock()
			d.lastChannelID = dmChanID
			d.mu.Unlock()

			statusCode, respBody, err = sendToChannel(dmChanID)
			if err != nil {
				return err
			}
		}
	}

	if statusCode < 200 || statusCode >= 300 {
		return fmt.Errorf("discord api error (%d): %s", statusCode, string(respBody))
	}
	return nil
}


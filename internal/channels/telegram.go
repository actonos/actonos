package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

// TelegramAdapter handles two-way messaging with the Telegram Bot API.
type TelegramAdapter struct {
	mu       sync.RWMutex
	token    string
	bus      *bus.EventBus
	client   *http.Client
	running  bool
	stopChan chan struct{}
}

// NewTelegramAdapter creates a new TelegramAdapter.
func NewTelegramAdapter(token string, bus *bus.EventBus) *TelegramAdapter {
	return &TelegramAdapter{
		token:    token,
		bus:      bus,
		client:   &http.Client{Timeout: 30 * time.Second},
		stopChan: make(chan struct{}),
	}
}

func (t *TelegramAdapter) Name() string { return "telegram" }

func (t *TelegramAdapter) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.token == "" {
		slog.Info("telegram channel adapter: no token provided, idle mode")
		return nil
	}

	t.running = true
	slog.Info("telegram channel adapter started")
	return nil
}

func (t *TelegramAdapter) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.running {
		close(t.stopChan)
		t.running = false
	}
	return nil
}

func (t *TelegramAdapter) SendMessage(ctx context.Context, msg OutboundMessage) error {
	if t.token == "" {
		return fmt.Errorf("telegram token not configured")
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	body := map[string]any{
		"chat_id": msg.Recipient,
		"text":    msg.Content,
	}

	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := t.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending telegram message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("telegram api returned status %d", resp.StatusCode)
	}

	return nil
}

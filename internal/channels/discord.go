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

// DiscordAdapter handles sending messages to Discord via Webhooks.
type DiscordAdapter struct {
	mu         sync.RWMutex
	webhookURL string
	bus        *bus.EventBus
	client     *http.Client
}

// NewDiscordAdapter creates a new DiscordAdapter.
func NewDiscordAdapter(webhookURL string, bus *bus.EventBus) *DiscordAdapter {
	return &DiscordAdapter{
		webhookURL: webhookURL,
		bus:        bus,
		client:     &http.Client{Timeout: 30 * time.Second},
	}
}

func (d *DiscordAdapter) Name() string { return "discord" }

func (d *DiscordAdapter) Start(ctx context.Context) error {
	slog.Info("discord channel adapter initialized")
	return nil
}

func (d *DiscordAdapter) Stop() error {
	return nil
}

func (d *DiscordAdapter) SendMessage(ctx context.Context, msg OutboundMessage) error {
	url := d.webhookURL
	if msg.Recipient != "" && msg.Recipient != "default" {
		url = msg.Recipient
	}

	if url == "" {
		return fmt.Errorf("discord webhook url not configured")
	}

	body := map[string]string{
		"content": msg.Content,
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

	resp, err := d.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending discord message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("discord webhook returned status %d", resp.StatusCode)
	}

	return nil
}

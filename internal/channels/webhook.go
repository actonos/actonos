package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/security"
)

// WebhookAdapter dispatches outbound agent events and receives inbound webhooks.
type WebhookAdapter struct {
	targetURL string
	bus       *bus.EventBus
	client    *http.Client
}

// NewWebhookAdapter creates a new WebhookAdapter.
func NewWebhookAdapter(targetURL string, bus *bus.EventBus) *WebhookAdapter {
	return &WebhookAdapter{
		targetURL: targetURL,
		bus:       bus,
		client:    &http.Client{Timeout: 30 * time.Second},
	}
}

func (w *WebhookAdapter) Name() string { return "webhook" }

func (w *WebhookAdapter) Start(ctx context.Context) error {
	return nil
}

func (w *WebhookAdapter) Stop() error {
	return nil
}

func (w *WebhookAdapter) SendMessage(ctx context.Context, msg OutboundMessage) error {
	target := w.targetURL
	if recipientURL := absoluteHTTPURL(msg.Recipient); recipientURL != "" {
		if err := security.ValidateOutboundURL(ctx, recipientURL); err != nil {
			return fmt.Errorf("webhook recipient url rejected: %w", err)
		}
		target = recipientURL
	}

	if target == "" {
		return fmt.Errorf("target webhook url not configured")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("dispatching webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook endpoint returned status %d", resp.StatusCode)
	}

	return nil
}

func absoluteHTTPURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "default") {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return ""
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return ""
	}
	return raw
}

package channels

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

// WhatsAppAdapter connects ActonOS with WhatsApp Cloud API or bridge webhooks.
type WhatsAppAdapter struct {
	mu             sync.RWMutex
	accessToken    string
	phoneNumberID  string
	verifyToken    string
	bus            *bus.EventBus
	pairingMgr     *PairingManager
	client         *http.Client
	unauthNotified map[string]time.Time
}

// NewWhatsAppAdapter creates a new WhatsAppAdapter.
func NewWhatsAppAdapter(accessToken, phoneNumberID, verifyToken string, bus *bus.EventBus, pairingMgr *PairingManager) *WhatsAppAdapter {
	return &WhatsAppAdapter{
		accessToken:    accessToken,
		phoneNumberID:  phoneNumberID,
		verifyToken:    verifyToken,
		bus:            bus,
		pairingMgr:     pairingMgr,
		client:         &http.Client{Timeout: 30 * time.Second},
		unauthNotified: make(map[string]time.Time),
	}
}

func (w *WhatsAppAdapter) Name() string { return "whatsapp" }

func (w *WhatsAppAdapter) Start(ctx context.Context) error {
	return nil
}

func (w *WhatsAppAdapter) Stop() error {
	return nil
}

// VerifyWebhook validates the Meta webhook challenge request.
func (w *WhatsAppAdapter) VerifyWebhook(mode, token, challenge string) (string, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	if mode == "subscribe" && token == w.verifyToken {
		return challenge, true
	}
	return "", false
}

// HandleInboundPayload parses incoming WhatsApp message notifications from Webhooks.
func (w *WhatsAppAdapter) HandleInboundPayload(ctx context.Context, payload []byte) error {
	var body struct {
		Entry []struct {
			Changes []struct {
				Value struct {
					Messages []struct {
						From string `json:"from"`
						ID   string `json:"id"`
						Text struct {
							Body string `json:"body"`
						} `json:"text"`
						Type string `json:"type"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}

	if err := json.Unmarshal(payload, &body); err != nil {
		return fmt.Errorf("parsing whatsapp webhook: %w", err)
	}

	for _, entry := range body.Entry {
		for _, change := range entry.Changes {
			for _, msg := range change.Value.Messages {
				if msg.Type != "text" || msg.Text.Body == "" {
					continue
				}

				senderPhone := msg.From
				text := msg.Text.Body

				// Pairing / Authorization Check
				isAuth := false
				if w.pairingMgr != nil {
					isAuth = w.pairingMgr.IsAuthorized("whatsapp", senderPhone)
				} else {
					isAuth = true
				}

				if !isAuth {
					if len(text) == 6 && w.pairingMgr != nil {
						paired, err := w.pairingMgr.ValidateAndPair("whatsapp", text, senderPhone, senderPhone)
						if err == nil && paired {
							_ = w.SendMessage(ctx, OutboundMessage{
								ChannelID: "whatsapp",
								Recipient: senderPhone,
								Content:   "🎉 Authentication successful! Your WhatsApp number is now paired with ActonOS Kernel.",
							})
							continue
						}
					}

					w.mu.Lock()
					if w.unauthNotified == nil {
						w.unauthNotified = make(map[string]time.Time)
					}
					lastSent, exists := w.unauthNotified[senderPhone]
					now := time.Now()
					if exists && now.Sub(lastSent) < 15*time.Second {
						w.mu.Unlock()
						continue
					}
					w.unauthNotified[senderPhone] = now
					w.mu.Unlock()

					_ = w.SendMessage(ctx, OutboundMessage{
						ChannelID: "whatsapp",
						Recipient: senderPhone,
						Content:   fmt.Sprintf("🔒 Unauthorized. Please send your 6-digit ActonOS Pairing PIN to authenticate. (Phone: %s)", senderPhone),
					})
					continue
				}

				if w.pairingMgr != nil {
					w.pairingMgr.TouchUser("whatsapp", senderPhone)
				}

				if w.bus != nil {
					w.bus.Publish(bus.NewEvent(bus.EventAgentActionStarted, "whatsapp", InboundMessage{
						ChannelID:   "whatsapp",
						SenderID:    senderPhone,
						SenderName:  senderPhone,
						TargetAgent: "default",
						Content:     text,
						Metadata: map[string]string{
							"message_id": msg.ID,
						},
					}))
				}
			}
		}
	}

	return nil
}

// SendMessage dispatches a WhatsApp text message via Cloud API.
func (w *WhatsAppAdapter) SendMessage(ctx context.Context, msg OutboundMessage) error {
	w.mu.RLock()
	token := w.accessToken
	phoneID := w.phoneNumberID
	w.mu.RUnlock()

	if token == "" || phoneID == "" {
		return fmt.Errorf("whatsapp access token or phone number id not configured")
	}

	url := fmt.Sprintf("https://graph.facebook.com/v20.0/%s/messages", phoneID)
	reqBody := map[string]any{
		"messaging_product": "whatsapp",
		"to":                msg.Recipient,
		"type":              "text",
		"text": map[string]string{
			"body": msg.Content,
		},
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("dispatching whatsapp message: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("whatsapp api returned status %d: %s", resp.StatusCode, string(respBytes))
	}

	return nil
}

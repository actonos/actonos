---
name: actonos-channels-dev
description: "Skill for developing and integrating messaging channel adapters in internal/channels/. Covers Telegram, WhatsApp, Discord, Session state, pairing codes, and webhooks."
---

# ActonOS Channels Development Skill

Use this skill when developing or extending messaging platform adapters in the `internal/channels/` package.

---

## 1. Package Overview

```
internal/channels/
├── adapter.go          # Core ChannelAdapter interface, InboundMessage, OutboundMessage
├── telegram.go         # Telegram Bot API integration (long-polling & webhook support)
├── whatsapp.go         # WhatsApp Cloud API integration & webhook verification
├── discord.go          # Discord Gateway bot & webhook integration
├── pairing.go          # PairingManager: 6-digit dynamic code verification & authorization
├── session.go          # Session state store per sender/channel (maps to conversations)
├── webhook.go          # Generic webhook router & verification handler
└── *_test.go           # Channel test suites
```

---

## 2. Core Abstractions (`adapter.go`)

Every channel integration implements the `ChannelAdapter` interface:

```go
type ChannelAdapter interface {
    // Name returns the unique channel type identifier (e.g. "telegram", "whatsapp", "discord")
    Name() string

    // Start initializes listeners, webhooks, or polling loops
    Start(ctx context.Context) error

    // Stop cleanly shuts down channel connections
    Stop() error

    // SendMessage delivers a response back to the user on that platform
    SendMessage(ctx context.Context, msg OutboundMessage) error
}
```

### Message Contracts
```go
type InboundMessage struct {
    ChannelID   string            `json:"channel_id"`
    SenderID    string            `json:"sender_id"`
    SenderName  string            `json:"sender_name"`
    TargetAgent string            `json:"target_agent"` // Extracted @agent_mention or default
    Content     string            `json:"content"`
    Metadata    map[string]string `json:"metadata,omitempty"`
}

type OutboundMessage struct {
    ChannelID string `json:"channel_id"`
    Recipient string `json:"recipient"`
    Content   string `json:"content"`
}
```

---

## 3. Pairing & Security Flow (`pairing.go`)

To prevent unauthorized users from triggering agents via public messaging platforms:
1. When an unknown `SenderID` messages the bot, the channel issues a 6-digit temporary pairing code.
2. The user enters the pairing code in the ActonOS Web UI (`ChannelsPage.tsx` $\rightarrow$ `POST /api/integrations/pairing/verify`).
3. `PairingManager` marks the `SenderID` as authorized and creates a bound session.

---

## 4. Multi-Account Configuration (`ChannelAccount`)

Channels support multiple configured accounts (e.g., separate Telegram bots or WhatsApp phone numbers).
- Stored and queried via `GET /api/integrations/channels` and `POST /api/integrations/channels`.
- Agents filter incoming messages using their `ListenChannels` array (`["*"]` or `["telegram"]`).

---

## 5. Development Checklist for New Channels

1. Implement `ChannelAdapter` in a new file (e.g., `internal/channels/slack.go`).
2. Integrate with `PairingManager` for security authorization.
3. Wire the adapter in `internal/server/router.go` and `internal/channels/`.
4. Update `web/src/pages/Channels/ChannelsPage.tsx` and `web/src/locales/{en,vi}/channels.json`.
5. Add unit tests verifying `SendMessage` and inbound message normalization.

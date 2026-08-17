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

## 4. Multi-Account Configuration & Proactive Routing

Channels support multiple concurrently configured accounts (e.g. multiple distinct Telegram bots or WhatsApp phone numbers).
- **Accounts Definition**: `ChannelAccount` (ID, Name, Channel, Token, PhoneID, Enabled, `BoundAgentIDs`).
- **Dynamic Sync**: Auto-persisted via `POST /api/integrations/channels` and synced into active pollers.
- **Proactive Push**: Proactive notifications from Cron or Heartbeat specify `target_channel`, `target_account_id`, and `target_recipient`.
- **Automatic Long-Message Chunking**:
  - Telegram limits single messages to 4096 characters.
  - The adapter automatically chunks long articles (>3900 chars) along paragraph/newline boundaries (`\n\n` $\rightarrow$ `\n` $\rightarrow$ space) to prevent truncation or Telegram API errors.
- **Anti-Double-Dispatch**:
  - The ReAct loop and proactive schedulers cooperate so messages are delivered exactly once without duplicate spam.

---

## 5. Development Checklist for New Channels

1. Implement `ChannelAdapter` in a new file (e.g., `internal/channels/slack.go`).
2. Integrate with `PairingManager` for security authorization.
3. Wire the adapter into `ChannelManager` and `internal/channels/`.
4. Update `web/src/pages/Channels/ChannelsPage.tsx` and `web/src/locales/{en,vi}/channels.json`.
5. Implement message chunking for platform length limits (e.g. 4096 for Telegram, 2000 for Discord).
6. Add unit tests verifying `SendMessage` and inbound message normalization.

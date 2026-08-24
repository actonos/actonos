---
name: actonos-channels-dev
description: "Skill for developing messaging channel adapters via WASM Plugins and managing routing, session state, pairing codes, and accounts in internal/channels/."
---

# ActonOS Channels Development Skill

Use this skill when developing or extending messaging platform integrations in ActonOS. External chat adapters (Telegram, Discord, Slack, WhatsApp, Zalo) are developed as **WASM Plugins** using the ActonOS Plugin SDK, while `internal/channels/` manages host routing, session isolation, pairing, and multi-account dispatching.

---

## 1. Architecture Overview

```
internal/channels/
├── adapter.go          # Core ChannelAdapter interface, InboundMessage, OutboundMessage contracts
├── manager.go          # ChannelManager: multi-account lifecycle, account sync, dynamic poller registration
├── router.go           # MessageRouter: inbound dispatch, @agent mention parsing, routing modes
├── pairing.go          # PairingManager: 6-digit security pairing codes & authorization ledger
├── session.go          # Session state store per sender/channel/agent (conv_{channel}_{sender}_{agentID})
├── webhook.go          # Generic webhook router & verification handler
└── *_test.go           # Channel test suites

internal/plugin/
└── bridge_channel.go   # WasmChannelBridge: bridges WASM plugin instances to ChannelAdapter
```

---

## 2. Core Abstractions (`adapter.go`)

Every channel integration (bridged from WASM) satisfies the `ChannelAdapter` interface:

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
    TargetAgent string            `json:"target_agent"` // Extracted @agent_mention or bound agent
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

## 3. Developing a Channel Plugin (Plugin SDK)

Channel adapters run inside the **Wazero WebAssembly sandbox**. To create a new channel:

1. **Scaffold with `acton-plugin`**:
   ```bash
   acton-plugin new channel-slack --type=channel
   ```

2. **Implement `sdk.ChannelAdapter` in Go / TinyGo**:
   ```go
   type SlackChannel struct {
       sdk.BaseChannel
   }

   func (s *SlackChannel) SendMessage(ctx sdk.Context, msg sdk.OutboundMessage) error {
       token, _ := ctx.Vault().GetSecret("slack_bot_token")
       // Use ctx.HTTP().Post or WebSocket to deliver message
       return nil
   }

   func (s *SlackChannel) PollMessages(ctx sdk.Context) ([]sdk.InboundMessage, error) {
       // Poll updates or consume stream from WebSocket
       return inbounds, nil
   }
   ```

3. **Declare Manifest & Permissions (`manifest.json`)**:
   ```json
   {
     "id": "channel-slack",
     "name": "Slack Bot Gateway",
     "version": "1.0.0",
     "capabilities": ["channel"],
     "permissions": {
       "net_outbound": ["slack.com", "*.slack.com"],
       "secrets": ["slack_bot_token"]
     }
   }
   ```

4. **Package Bundle**:
   ```bash
   acton-plugin build
   acton-plugin pack -out dist/channel-slack.actonpkg
   ```

---

## 4. Inbound Message Routing & Agent Mention Parsing (`router.go`)

Incoming messages pass through `MessageRouter.handleInboundMessage()`:
- **Mention Parsing (`ExtractAgentMention`)**: Extracts `@agent_name` or `/agent <name>` from prompt text, cleaning the input before passing to LLM.
- **Routing Modes**:
  - `exclusive`: Only the assigned agent can respond.
  - `mention`: Routes to explicitly mentioned `@agent` in group chats; falls back to default agent in private DMs.
  - `fallback`: Routes to Nova (`agent_system_core`) if no matching agent is bound.
- **Session Isolation**: Generates deterministic session IDs `conv_{channel}_{sender}_{agentID}` so different agents maintain separate memory diaries with the same user.

---

## 5. Pairing & Security Flow (`pairing.go`)

To prevent unauthorized strangers from triggering agents on public channels:
1. When an unknown `SenderID` messages the bot, the system generates a 6-digit temporary pairing code.
2. The user enters the pairing code via REST API `POST /api/integrations/pairing/verify`.
3. `PairingManager` records the sender as authorized.

---

## 6. Multi-Account Management (`manager.go`)

Channels support multiple concurrent bot accounts (e.g., Support Bot, DevOps Bot):
- Persisted via `POST /api/integrations/channels` and synchronized into active pollers via `ChannelManager.SyncAccounts()`.
- Proactive alerts (Cron, Heartbeat) specify `target_channel`, `target_account_id`, and `target_recipient`.
- Status visibility: `GetAccountStatuses()` exposes connection health and error diagnostics.

---

## 7. Agent Execution Boundary

- Channel adapters deliver messages to the event bus and never execute tools directly.
- All cognitive tool calls go through `ToolRegistry.Execute` to enforce authorization, approval requirements, and sandbox constraints.

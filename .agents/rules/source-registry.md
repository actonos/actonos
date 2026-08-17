# ActonOS Canonical Source Registry

> **Source of truth** for all files, routes, components, and namespaces in the ActonOS codebase.
> AI agents MUST cross-reference this registry before creating, modifying, or referencing files.

---

## Backend API Routes (from `internal/server/router.go`)

### Public Routes (no auth)

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/health` | `handleHealth` | `router.go` |
| `GET` | `/api/auth/status` | `handleGetAuthStatus` | `api_auth.go` |
| `POST` | `/api/auth/setup` | `handleSetupAuth` | `api_auth.go` |
| `POST` | `/api/auth/login` | `handleLogin` | `api_auth.go` |
| `POST` | `/api/auth/logout` | `handleLogout` | `api_auth.go` |
| `GET` | `/api/auth/callback` | `handleOAuthCallback` | `api_integrations.go` |
| `GET` | `/api/webhooks/whatsapp` | `handleWhatsAppVerifyWebhook` | `api_integrations.go` |
| `POST` | `/api/webhooks/whatsapp` | `handleWhatsAppInboundWebhook` | `api_integrations.go` |

### Protected Routes (require auth via `RequireAuthMiddleware`)

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `PUT` | `/api/auth/password` | `handleChangePassword` | `api_auth.go` |

#### Dashboard

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/dashboard/summary` | `handleDashboardSummary` | `api_dashboard.go` |

#### Agent Management

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/agents` | `handleListAgents` | `api_agent.go` |
| `POST` | `/api/agents` | `handleCreateAgent` | `api_agent.go` |
| `GET` | `/api/agents/{agentID}` | `handleGetAgent` | `api_agent.go` |
| `PUT` | `/api/agents/{agentID}` | `handleUpdateAgent` | `api_agent.go` |
| `DELETE` | `/api/agents/{agentID}` | `handleDeleteAgent` | `api_agent.go` |
| `POST` | `/api/agents/{agentID}/start` | `handleStartAgent` | `api_agent.go` |
| `POST` | `/api/agents/{agentID}/stop` | `handleStopAgent` | `api_agent.go` |
| `POST` | `/api/agents/{agentID}/chat` | `handleChat` | `api_agent.go` |

#### Soul & Memory (global + per-agent)

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/agents/soul` | `handleGetSoul` | `api_agent.go` |
| `PUT` | `/api/agents/soul` | `handleSaveSoul` | `api_agent.go` |
| `GET` | `/api/agents/memory-md` | `handleGetMemoryMD` | `api_agent.go` |
| `GET` | `/api/agents/{agentID}/soul` | `handleGetSoul` | `api_agent.go` |
| `PUT` | `/api/agents/{agentID}/soul` | `handleSaveSoul` | `api_agent.go` |
| `GET` | `/api/agents/{agentID}/memory-md` | `handleGetMemoryMD` | `api_agent.go` |

#### Cron Jobs (under `/api/agents/cron` and alias `/api/cron`)

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/agents/cron` | `handleListCronJobs` | `api_agent.go` |
| `POST` | `/api/agents/cron` | `handleSaveCronJob` | `api_agent.go` |
| `POST` | `/api/agents/cron/{id}/run` | `handleRunCronJob` | `api_agent.go` |
| `DELETE` | `/api/agents/cron/{id}` | `handleDeleteCronJob` | `api_agent.go` |

#### Conversations

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/conversations` | `handleListConversations` | `api_conversations.go` |
| `POST` | `/api/conversations` | `handleCreateConversation` | `api_conversations.go` |
| `GET` | `/api/conversations/{id}` | `handleGetConversation` | `api_conversations.go` |
| `PUT` | `/api/conversations/{id}` | `handleUpdateConversation` | `api_conversations.go` |
| `DELETE` | `/api/conversations/{id}` | `handleDeleteConversation` | `api_conversations.go` |

#### Tools & Hub

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/tools` | `handleListTools` | `api_tools.go` |
| `POST` | `/api/tools/mcp` | `handleConnectMCP` | `api_tools.go` |
| `DELETE` | `/api/tools/mcp/{serverID}` | `handleDisconnectMCP` | `api_tools.go` |
| `POST` | `/api/tools/execute` | `handleExecuteTool` | `api_tools.go` |
| `POST` | `/api/tools/skill` | `handleCreateSkill` | `api_tools.go` |
| `POST` | `/api/tools/wasm` | `handleUploadWASM` | `api_tools.go` |
| `GET` | `/api/tools/hub/catalog` | `handleListHubCatalog` | `api_tools.go` |
| `POST` | `/api/tools/hub/install` | `handleInstallHubSkill` | `api_tools.go` |
| `POST` | `/api/tools/hub/uninstall` | `handleUninstallHubSkill` | `api_tools.go` |

#### Setup & Onboarding

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/setup/status` | `handleGetSetupStatus` | `api_setup.go` |
| `POST` | `/api/setup/wizard` | `handleSetupWizard` | `api_setup.go` |

#### Integrations, Channels & Pairing

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/integrations` | `handleListIntegrations` | `api_integrations.go` |
| `GET` | `/api/integrations/oauth/callback` | `handleOAuthCallback` | `api_integrations.go` |
| `POST` | `/api/integrations/{provider}/auth-url` | `handleGetAuthURL` | `api_integrations.go` |
| `POST` | `/api/integrations/{provider}/token` | `handleSaveDirectToken` | `api_integrations.go` |
| `POST` | `/api/integrations/{provider}/config` | `handleSaveProviderConfig` | `api_integrations.go` |
| `POST` | `/api/integrations/{provider}/test` | `handleTestIntegration` | `api_integrations.go` |
| `POST` | `/api/integrations/{provider}/disconnect` | `handleDisconnectIntegration` | `api_integrations.go` |
| `POST` | `/api/integrations/{provider}/toggle` | `handleToggleIntegration` | `api_integrations.go` |
| `GET` | `/api/integrations/channels` | `handleGetChannels` | `api_integrations.go` |
| `POST` | `/api/integrations/channels` | `handleSaveChannels` | `api_integrations.go` |
| `POST` | `/api/integrations/pairing/code` | `handleGeneratePairingCode` | `api_integrations.go` |
| `POST` | `/api/integrations/pairing/verify` | `handleVerifyPairingCode` | `api_integrations.go` |
| `GET` | `/api/integrations/authorizations` | `handleListAuthorizations` | `api_integrations.go` |
| `DELETE` | `/api/integrations/authorizations` | `handleRevokeAuthorization` | `api_integrations.go` |

#### Workspace

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/workspace/files` | `handleListWorkspaceFiles` | `api_workspace.go` |
| `GET` | `/api/workspace/file` | `handleGetWorkspaceFile` | `api_workspace.go` |
| `POST` | `/api/workspace/file` | `handleSaveWorkspaceFile` | `api_workspace.go` |
| `DELETE` | `/api/workspace/file` | `handleDeleteWorkspaceFile` | `api_workspace.go` |
| `POST` | `/api/workspace/mkdir` | `handleMkdirWorkspace` | `api_workspace.go` |
| `POST` | `/api/workspace/upload` | `handleUploadWorkspaceFile` | `api_workspace.go` |

#### System, HAL, Keys & Identity

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/system/metrics` | `handleGetMetrics` | `api_system.go` |
| `GET` | `/api/system/identity` | `handleGetIdentity` | `api_system.go` |
| `PUT` | `/api/system/identity` | `handleSaveIdentity` | `api_system.go` |
| `GET` | `/api/system/profile` | `handleGetIdentity` | `api_system.go` |
| `PUT` | `/api/system/profile` | `handleSaveIdentity` | `api_system.go` |
| `GET` | `/api/system/keys` | `handleGetAPIKeys` | `api_system.go` |
| `POST` | `/api/system/keys` | `handleSaveAPIKeys` | `api_system.go` |
| `POST` | `/api/system/keys/test` | `handleTestAPIKey` | `api_system.go` |
| `GET` | `/api/system/audit` | `handleGetAuditLogs` | `api_system.go` |
| `GET` | `/api/system/storage` | `handleGetStorageInfo` | `api_system.go` |
| `GET` | `/api/system/backup` | `handleGetBackup` | `api_system.go` |
| `POST` | `/api/system/ota/check` | `handleCheckOTA` | `api_system.go` |
| `GET` | `/api/system/tailscale` | `handleGetTailscale` | `api_system.go` |
| `GET` | `/api/system/wifi/scan` | `handleWifiScan` | `api_system.go` |
| `POST` | `/api/system/wifi/connect` | `handleWifiConnect` | `api_system.go` |
| `POST` | `/api/system/restart` | `handleRestart` | `api_system.go` |

---

## Backend Packages (`internal/`)

| Package | Purpose | Key Files |
|:---|:---|:---|
| `agent` | Agent engine, manifests, cron, swarm, profile | `engine.go`, `manager.go`, `types.go`, `swarm.go`, `planner.go`, `verifier.go`, `reflection.go`, `profile.go`, `heartbeat.go`, `context.go`, `cron_scheduler.go` |
| `auth` | System auth, OAuth 2.1, token refresh, delegation | `system_auth.go`, `oauth2.go`, `token_refresher.go`, `delegation.go`, `state.go`, `dcr.go` |
| `bus` | Event bus (Go channels) | `eventbus.go` |
| `channels` | Multi-platform messaging adapters | `adapter.go`, `telegram.go`, `whatsapp.go`, `discord.go`, `pairing.go`, `session.go`, `webhook.go` |
| `llm` | LLM provider abstraction, model cascading | `provider.go`, `router.go`, `openai_compat.go` |
| `memory` | Hybrid RAG, vector search, FTS5, vault | `hybrid.go`, `vector.go`, `fts.go`, `decay.go`, `vault.go`, `db.go` |
| `sandbox` | Command execution isolation | `sandbox.go`, `bwrap_linux.go`, `jail_docker.go` |
| `server` | HTTP router, API handlers, static assets | `router.go`, `api_*.go`, `static.go`, `layered_fs.go` |
| `system` | HAL, hardware metrics, Tailscale, Wi-Fi | `hal.go`, `hal_linux.go`, `hal_docker.go`, `tailscale.go`, `metrics.go` |
| `tools` | Tool registry, MCP, WASM, skill watcher, hub | `registry.go`, `mcp_client.go`, `wasm_runner.go`, `native_tools.go`, `skill_watcher.go`, `hub.go`, `browser_tool.go` |

---

## Frontend Pages (`web/src/pages/`)

| Directory | NavTab ID | Component | Purpose |
|:---|:---|:---|:---|
| `Dashboard/` | `dashboard` | `DashboardPage` | System overview, agent summaries |
| `Agents/` | `agents` | `AgentsPage` | Agent list (responsive table) |
| `Agents/` | `agent-studio` | `AgentStudioPage` | Agent detail editor (config, soul, memory) |
| `Chat/` | `chat` | `ChatPage` | Conversational interface |
| `Automations/` | `automations` | `AutomationsPage` | Cron jobs, scheduled tasks |
| `Channels/` | `channels` | `ChannelsPage` | Telegram, WhatsApp, Discord config |
| `Connectors/` | `connectors` | `ConnectorsPage` | SaaS integrations (OAuth) |
| `ToolHub/` | `tools` | `ToolHubPage` | MCP servers, WASM plugins |
| `Skills/` | `skills` | `SkillsPage` | Skill marketplace |
| `Workspace/` | `workspace` | `WorkspacePage` | File manager |
| `Settings/` | `settings` | `SettingsPage` | System settings, keys, backup |
| `Auth/` | — | `SetupWizardPage` | First-run onboarding |
| `Auth/` | — | `LoginPage` | Password login |

---

## UI Components (`web/src/components/`)

### Atomic Primitives (`ui/`)

| File | Component | Purpose |
|:---|:---|:---|
| `Button.tsx` | `Button` | Primary/secondary/ghost pill buttons |
| `Input.tsx` | `Input` | Capsule text inputs with labels |
| `Card.tsx` | `Card` | Soft Meadow 24px surface cards |
| `Badge.tsx` | `Badge` | Status pill badges |
| `Modal.tsx` | `Modal` | Accessible dialog container |
| `ConfirmModal.tsx` | `ConfirmModal` | Confirmation dialog with actions |
| `PromptModal.tsx` | `PromptModal` | Text input modal dialog |
| `Toast.tsx` | `ToastProvider`, `useToast` | Toast notification system |
| `BlobBackdrop.tsx` | `BlobBackdrop` | Decorative organic SVG blobs |
| `LanguageSwitcher.tsx` | `LanguageSwitcher` | Globe icon + language trigger |
| `LanguageSelectModal.tsx` | `LanguageSelectModal` | Language selection overlay |

### Layout (`layout/`)

| File | Component | Purpose |
|:---|:---|:---|
| `Sidebar.tsx` | `Sidebar` | Collapsible left navigation sidebar |
| `Header.tsx` | `Header` | Sticky top header bar |
| `Navbar.tsx` | `Navbar` | Top navigation bar (alternate) |
| `PageContainer.tsx` | `PageContainer` | Max-width centered container |

### Feature Components (`features/`)

| File | Component | Purpose |
|:---|:---|:---|
| `features/agents/AgentCard.tsx` | `AgentCard` | Agent persona display card |
| `features/agents/AgentFormModal.tsx` | `AgentFormModal` | Create/edit agent dialog |
| `features/agents/CronJobModal.tsx` | `CronJobModal` | Cron job create/edit dialog |
| `features/agents/SoulEditorModal.tsx` | `SoulEditorModal` | SOUL.md editor modal |
| `features/onboarding/` | (setup forms) | Onboarding wizard forms |
| `features/tools/` | (tool cards) | Tool display components |

### Chat Components (`chat/`)

| File | Component | Purpose |
|:---|:---|:---|
| `chat/MarkdownContent.tsx` | `MarkdownContent` | Markdown renderer for chat messages |

---

## Locale Namespaces (`web/src/locales/`)

Both `en/` and `vi/` contain the following 14 namespace files:

| Namespace | File | UI Coverage |
|:---|:---|:---|
| `common` | `common.json` | Buttons, validation, generic labels |
| `nav` | `nav.json` | Navigation items, sidebar labels |
| `setup` | `setup.json` | Setup wizard, onboarding |
| `chat` | `chat.json` | Chat input, streaming states |
| `agents` | `agents.json` | Agent table, create/edit modal |
| `tools` | `tools.json` | MCP servers, tool management |
| `skills` | `skills.json` | Skills marketplace |
| `settings` | `settings.json` | System settings, API keys |
| `workspace` | `workspace.json` | File manager |
| `integrations` | `integrations.json` | OAuth connectors |
| `channels` | `channels.json` | Messaging channels config |
| `connectors` | `connectors.json` | SaaS connector details |
| `dashboard` | `dashboard.json` | Dashboard overview |
| `automations` | `automations.json` | Cron jobs, scheduled tasks |

---

## Go Types ↔ TypeScript Types Mapping

| Go Type (`internal/agent/types.go`) | TS Type (`web/src/lib/types.ts`) | Notes |
|:---|:---|:---|
| `AgentManifest` | `AgentManifest` | Must stay in sync. Go uses `llm.ModelConfig`, TS has inline `ModelConfig` |
| `DelegationScope` | `DelegationScope` | Same field names |
| `TriggerRule` | `TriggerRule` | Same field names |
| `AgentStatus` | `AgentStatus` | Values: `active`, `stopped`, `error` |
| `ApprovalLevel` | `ApprovalLevel` | Values: `Low`, `Medium`, `High` |
| `SubTask` / `SubTaskResult` | — | Not exposed to frontend (internal only) |
| `AgentStreamEvent` | — | Consumed via SSE, not a REST type |
| `AuditLogEntry` | — | Future: needs TS type for audit log UI |
| — | `ConversationItem` | Defined in both `types.ts` and `api.ts` (duplicate) |
| — | `ChatMessageRecord` | TS only |
| — | `ToolInfo` | TS only |
| — | `SystemMetrics` | TS only |
| — | `TailscaleStatus` | TS only |
| — | `ConnectorInfo` | TS only |
| — | `LLMProviderInfo` | TS only |

---

## Key Configuration Files

| File | Purpose |
|:---|:---|
| `web/src/lib/i18n.ts` | i18next initialization, namespace registration |
| `web/src/lib/api.ts` | REST API client functions |
| `web/src/lib/types.ts` | Shared TypeScript type contracts |
| `web/src/lib/models.ts` | LLM model catalog |
| `web/src/index.css` | Tailwind v4 theme tokens, font declarations, scrollbar styles |
| `web/src/App.tsx` | Root component, routing, auth flow |

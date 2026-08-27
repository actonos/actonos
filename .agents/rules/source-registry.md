# ActonOS Canonical Source Registry

> **Source of truth** for all files, routes, components, and namespaces in the ActonOS codebase.
> AI agents MUST cross-reference this registry before creating, modifying, or referencing files.

---

## Backend API Routes (from `internal/server/router.go`)

### Public Routes (no auth)

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/health` | `handleHealth` | `router.go` |
| `GET` | `/api/models` | `handleGetModelsCatalog` | `api_system.go` |
| `GET` | `/api/notifications/push/vapid-key` | `handleGetVAPIDPublicKey` | `api_notifications.go` |
| `GET` | `/api/auth/status` | `handleGetAuthStatus` | `api_auth.go` |
| `POST` | `/api/auth/setup` | `handleSetupAuth` | `api_auth.go` |
| `POST` | `/api/auth/login` | `handleLogin` | `api_auth.go` |
| `POST` | `/api/auth/logout` | `handleLogout` | `api_auth.go` |

### Protected Routes (require auth via `RequireAuthMiddleware`)

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `PUT` | `/api/auth/password` | `handleChangePassword` | `api_auth.go` |

#### Dashboard

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/dashboard/summary` | `handleDashboardSummary` | `api_dashboard.go` |
| `GET` | `/api/realtime` | `handleRealtimeStream` | `api_realtime.go` |

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
| `GET` | `/api/tools/mcp` | `handleListMCPServers` | `api_tools.go` |
| `DELETE` | `/api/tools/mcp/{serverID}` | `handleDisconnectMCP` | `api_tools.go` |
| `PUT` | `/api/tools/mcp/{serverID}` | `handleToggleMCPServer` | `api_tools.go` |
| `POST` | `/api/tools/execute` | `handleExecuteTool` | `api_tools.go` |
| `POST` | `/api/tools/skill` | `handleCreateSkill` | `api_tools.go` |
| `PUT` | `/api/tools/skills/{name}/toggle` | `handleToggleSkill` | `api_tools.go` |
| `POST` | `/api/tools/wasm` | `handleUploadWASM` | `api_tools.go` |
| `GET` | `/api/tools/hub/catalog` | `handleListHubCatalog` | `api_tools.go` |
| `POST` | `/api/tools/hub/install` | `handleInstallHubSkill` | `api_tools.go` |
#### Plugins (WasmLoader)

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/plugins` | `handleListPlugins` | `api_plugins.go` |
| `GET` | `/api/plugins/available` | `handleListAvailablePlugins` | `api_plugins.go` |
| `POST` | `/api/plugins/install` | `handleInstallAvailablePlugin` | `api_plugins.go` |
| `POST` | `/api/plugins/upload` | `handleUploadPlugin` | `api_plugins.go` |
| `POST` | `/api/plugins/{id}/enable` | `handleEnablePlugin` | `api_plugins.go` |
| `POST` | `/api/plugins/{id}/disable` | `handleDisablePlugin` | `api_plugins.go` |
| `DELETE` | `/api/plugins/{id}` | `handleDeletePlugin` | `api_plugins.go` |
| `GET` | `/api/plugins/{id}/logs` | `handleGetPluginLogs` | `api_plugins.go` |
| `POST` | `/api/plugins/{id}/config` | `handleUpdatePluginConfig` | `api_plugins.go` |
| `PUT` | `/api/plugins/{id}/config` | `handleUpdatePluginConfig` | `api_plugins.go` |

#### Hardware-Bound Vault Secrets

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/vault/secrets` | `handleListVaultSecrets` | `api_vault.go` |
| `POST` | `/api/vault/secrets` | `handleSetVaultSecret` | `api_vault.go` |
| `GET` | `/api/vault/secrets/{name}` | `handleGetVaultSecret` | `api_vault.go` |
| `PUT` | `/api/vault/secrets/{name}` | `handleSetVaultSecret` | `api_vault.go` |
| `DELETE` | `/api/vault/secrets/{name}` | `handleDeleteVaultSecret` | `api_vault.go` |

#### Human Approval & Agent Runs

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/approvals` | `handleListApprovals` | `api_approvals.go` |
| `POST` | `/api/approvals/{id}/approve` | `handleApproveAction` | `api_approvals.go` |
| `POST` | `/api/approvals/{id}/reject` | `handleRejectAction` | `api_approvals.go` |
| `GET` | `/api/runs` | `handleListAgentRuns` | `api_runs.go` |
| `GET` | `/api/runs/{id}/events` | `handleListRunEvents` | `api_runs.go` |

#### Setup & Onboarding

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/setup/status` | `handleGetSetupStatus` | `api_setup.go` |
| `POST` | `/api/setup/wizard` | `handleSetupWizard` | `api_setup.go` |

#### Channel Accounts & Device Pairing

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/integrations/channels` | `handleGetChannels` | `api_integrations.go` |
| `GET` | `/api/integrations/channels/accounts` | `handleListAllChannelAccounts` | `api_integrations.go` |
| `POST` | `/api/integrations/channels` | `handleSaveChannels` | `api_integrations.go` |
| `POST` | `/api/integrations/pairing/code` | `handleGeneratePairingCode` | `api_integrations.go` |
| `POST` | `/api/integrations/pairing/verify` | `handleVerifyPairingCode` | `api_integrations.go` |
| `GET` | `/api/integrations/authorizations` | `handleListAuthorizations` | `api_integrations.go` |
| `DELETE` | `/api/integrations/authorizations` | `handleRevokeAuthorization` | `api_integrations.go` |

#### Workspace

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/workspace/files` | `handleDBListWorkspaceFiles` | `api_workspace_db.go` |
| `GET` | `/api/workspace/file` | `handleDBGetWorkspaceFile` | `api_workspace_db.go` |
| `POST` | `/api/workspace/file` | `handleDBSaveWorkspaceFile` | `api_workspace_db.go` |
| `DELETE` | `/api/workspace/file` | `handleDBDeleteWorkspaceFile` | `api_workspace_db.go` |
| `POST` | `/api/workspace/mkdir` | `handleDBMkdirWorkspace` | `api_workspace_db.go` |
| `POST` | `/api/workspace/upload` | `handleDBUploadWorkspaceFile` | `api_workspace_db.go` |
| `GET` | `/api/workspace/raw` | `handleDBRawWorkspaceFile` | `api_workspace_db.go` |

#### Autonomous Missions & Tasks Backlog

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/tasks` | `handleListTasks` | `api_tasks.go` |
| `POST` | `/api/tasks` | `handleCreateTask` | `api_tasks.go` |
| `GET` | `/api/tasks/{id}` | `handleGetTask` | `api_tasks.go` |
| `PUT` | `/api/tasks/{id}` | `handleUpdateTask` | `api_tasks.go` |
| `DELETE` | `/api/tasks/{id}` | `handleDeleteTask` | `api_tasks.go` |

#### Heartbeat Coordinator & Pulse

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/heartbeat/config` | `handleGetHeartbeatConfig` | `api_tasks.go` |
| `PUT` | `/api/heartbeat/config` | `handleSaveHeartbeatConfig` | `api_tasks.go` |
| `POST` | `/api/heartbeat/trigger` | `handleTriggerHeartbeatPulse` | `api_tasks.go` |
| `GET` | `/api/heartbeat/runs` | `handleListHeartbeatRuns` | `api_tasks.go` |

#### Notification Center

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/notifications` | `handleListNotifications` | `api_notifications.go` |
| `GET` | `/api/notifications/unread-count` | `handleGetUnreadNotificationsCount` | `api_notifications.go` |
| `POST` | `/api/notifications/mark-read` | `handleMarkNotificationRead` | `api_notifications.go` |
| `DELETE` | `/api/notifications` | `handleDeleteNotifications` | `api_notifications.go` |
| `GET` | `/api/notifications/push/vapid-key` | `handleGetVAPIDPublicKey` | `api_notifications.go` |
| `POST` | `/api/notifications/push/subscribe` | `handleSubscribePush` | `api_notifications.go` |
| `POST` | `/api/notifications/push/unsubscribe` | `handleUnsubscribePush` | `api_notifications.go` |
| `POST` | `/api/notifications/push/test` | `handleTestPushNotification` | `api_notifications.go` |

#### System, HAL, Keys, Tokens & Identity

| Method | Path | Handler | File |
|:---|:---|:---|:---|
| `GET` | `/api/system/metrics` | `handleGetMetrics` | `api_system.go` |
| `GET` | `/api/system/embedding` | `handleGetEmbeddingStatus` | `api_embedding.go` |
| `GET` | `/api/system/token-usage` | `handleGetTokenUsage` | `api_system.go` |
| `GET` | `/api/system/token-usage/history` | `handleGetTokenHistory` | `api_system.go` |
| `GET` | `/api/system/heartbeat/history` | `handleGetHeartbeatHistory` | `api_system.go` |
| `GET` | `/api/system/identity` | `handleGetIdentity` | `api_system.go` |
| `PUT` | `/api/system/identity` | `handleSaveIdentity` | `api_system.go` |
| `GET` | `/api/system/profile` | `handleGetIdentity` | `api_system.go` |
| `PUT` | `/api/system/profile` | `handleSaveIdentity` | `api_system.go` |
| `GET` | `/api/system/keys` | `handleGetAPIKeys` | `api_system.go` |
| `POST` | `/api/system/keys` | `handleSaveAPIKeys` | `api_system.go` |
| `DELETE` | `/api/system/keys/{provider}` | `handleDeleteAPIKey` | `api_system.go` |
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
| `agent` | Agent engine, durable runs, manifests, tasks, cron, swarm, profile | `engine.go`, `runs.go`, `manager.go`, `types.go`, `tasks.go`, `swarm.go`, `planner.go`, `verifier.go`, `reflection.go`, `profile.go`, `heartbeat.go`, `context.go`, `cron_scheduler.go` |
| `auth` | System auth, OAuth 2.1, token refresh, delegation | `system_auth.go`, `oauth2.go`, `token_refresher.go`, `delegation.go`, `state.go`, `dcr.go` |
| `bus` | Event bus (Go channels) | `eventbus.go` |
| `channels` | Unified messaging channel abstractions, session routing & pairing | `adapter.go`, `manager.go`, `router.go`, `pairing.go`, `session.go`, `webhook.go` |
| `llm` | LLM provider abstraction, cascading, and true SSE streaming | `provider.go`, `router.go`, `openai.go`, `anthropic.go`, `gemini.go`, `deepseek.go` |
| `memory` | Hybrid RAG, durable local embedding queue, workspace watcher, Chromem vector search, FTS5, token ledger, vault | `embedding.go`, `embedding_watcher.go`, `hybrid.go`, `vector.go`, `fts.go`, `decay.go`, `tokens.go`, `vault.go`, `db.go` |
| `plugin` | WasmLoader unified polyglot plugin runtime, Wazero linear memory sandbox, host syscalls, security gate, bridges | `types.go`, `loader.go`, `host_api.go`, `security_gate.go`, `bridge_tool.go`, `bridge_channel.go`, `bridge_connector.go`, `manager.go` |
| `sandbox` | Fail-closed command isolation and Linux cgroup enforcement | `executor.go`, `strong_linux.go`, `strong_other.go`, `bwrap_linux.go`, `jail_docker.go`, `subshell.go` |
| `security` | Canonical workspace path containment and outbound SSRF protection | `path.go`, `network.go` |
| `server` | HTTP router, configured data roots, durable approval/run APIs, administrative action dispatch, true SSE, static assets | `router.go`, `api_approvals.go`, `api_runs.go`, `api_plugins.go`, `admin_actions.go`, `api_*.go`, `static.go`, `layered_fs.go` |
| `system` | HAL, hardware metrics, Tailscale, Wi-Fi, tamper-evident audit | `hal.go`, `hal_linux.go`, `hal_docker.go`, `tailscale.go`, `metrics.go`, `audit.go` |
| `tools` | Authorized execution boundary, approvals, MCP, WASM, skill watcher, hub | `registry.go`, `approval.go`, `command_policy.go`, `mcp_client.go`, `native_tools.go`, `skill_watcher.go`, `hub.go`, `browser_tool.go` |

---

## Frontend Pages (`web/src/pages/`)

| Directory | NavTab ID | Component | Purpose |
|:---|:---|:---|:---|
| `Dashboard/` | `dashboard` | `DashboardPage` | System overview, agent summaries, token launcher |
| `Agents/` | `agents` | `AgentsPage` | Agent list (responsive table) |
| `Agents/` | `agent-studio` | `AgentStudioPage` | Agent detail editor (config, soul, memory, heartbeat, tools, channels, governance) |
| `Missions/` | `missions` | `MissionsPage` | Autonomous task matrix, pulse audit ledger, approval queue, durable run governance |
| `Operations/` | `operations` | `OperationsPage` | Live hardware/Docker telemetry, execution feed, canvas, terminal, task controls, approvals, and model cost |
| `Chat/` | `chat` | `ChatPage` | Conversational interface |
| `Automations/` | `automations` | `AutomationsPage` | Cron jobs, scheduled tasks |
| `Plugins/` | `plugins` | `PluginsPage` | Sandboxed WASM Plugin Hub (Tools, Chat Channels, SaaS Connectors) |
| `ToolHub/` | `tools` | `ToolHubPage` | Native system tools & MCP servers |
| `Skills/` | `skills` | `SkillsPage` | Skill-as-a-Folder packages & Community Skill Hub |
| `Workspace/` | `workspace` | `WorkspacePage` | File manager |
| `Terminal/` | `terminal` | `TerminalPage` | Direct interactive web terminal connected to host OS shell |
| `Notifications/` | `notifications` | `NotificationsPage` | Full notification history, filters, pagination, clear actions |
| `AuditLogs/` | `audit-logs` | `AuditLogsPage` | Tamper-evident audit log ledger & trace inspector |
| `Settings/` | `settings` | `SettingsPage` | System settings, keys, backup, token ledger |
| `Auth/` | — | `SetupWizardPage` | First-run onboarding |
| `Auth/` | — | `LoginPage` | Password login |

---

## UI Components (`web/src/components/`)

### Modals (`modals/` & `ui/` & Feature Modals)

| File | Component | Purpose |
|:---|:---|:---|
| `features/notifications/NotificationBell.tsx` | `NotificationBell` | Header notification trigger button & recent popup dropdown |
| `features/agents/AgentHeartbeatSection.tsx` | `AgentHeartbeatSection` | Per-agent autonomous heartbeat, standing directives, interval, active hours |
| `features/agents/AgentStudioNav.tsx` | `AgentStudioNav` | Sticky section navigation bar for Agent Studio |
| `modals/TokenLedgerModal.tsx` | `TokenLedgerModal` | Comprehensive token usage analytics & ledger table |
| `pages/Missions/components/TaskModal.tsx` | `TaskModal` | Mission backlog task create/edit modal |
| `pages/AuditLogs/components/AuditLogDetailModal.tsx` | `AuditLogDetailModal` | Full audit log trace, cryptographic verification & JSON inspector |
| `pages/Plugins/PluginDetailModal.tsx` | `PluginDetailModal` | Plugin manifest, capabilities, config form, and secrets editor |
| `pages/Plugins/PluginLogsModal.tsx` | `PluginLogsModal` | Live sandbox execution log stream for WASM plugin |
| `pages/Plugins/PluginUploadModal.tsx` | `PluginUploadModal` | Upload and installation modal for `.actonpkg` package bundles |
| `ui/Modal.tsx` | `Modal` | Accessible dialog container |
| `ui/ConfirmModal.tsx` | `ConfirmModal` | Confirmation dialog with actions |
| `features/chat/ChatApprovalCard.tsx` | `ChatApprovalCard` | In-bubble approve/reject for chat tool pauses |
| `features/governance/ApprovalInterruption.tsx` | `ApprovalInterruption` | Full-screen approval overlay for mission, heartbeat, and REST 202 |

---

## Locale Namespaces (`web/src/locales/`)

Both `en/` and `vi/` contain the following 16 active namespace files:

| Namespace | File | UI Coverage |
|:---|:---|:---|
| `common` | `common.json` | Buttons, validation, generic labels |
| `nav` | `nav.json` | Navigation items, sidebar labels |
| `missions` | `missions.json` | Mission control, task matrix, heartbeat directives |
| `setup` | `setup.json` | Setup wizard, onboarding |
| `chat` | `chat.json` | Chat input, streaming states |
| `agents` | `agents.json` | Agent table, create/edit modal |
| `tools` | `tools.json` | MCP servers, tool management |
| `skills` | `skills.json` | Skills marketplace |
| `plugins` | `plugins.json` | WASM plugins hub, upload, detail modal, logs, config & secrets |
| `settings` | `settings.json` | System settings, API keys, token ledger tab |
| `notifications` | `notifications.json` | Notification center, browser push, history page |
| `audit` | `audit.json` | Audit logs ledger, filters, cryptographic hash verification, detail modal |
| `workspace` | `workspace.json` | File manager |
| `dashboard` | `dashboard.json` | Dashboard overview |
| `automations` | `automations.json` | Cron jobs, scheduled tasks |
| `operations` | `operations.json` | Live operations, runtime telemetry, approvals and cost |

---

## Go Types ↔ TypeScript Types Mapping

| Go Type (`internal/agent/types.go`, `plugin/types.go`, `tasks.go`, `memory/tokens.go`, `system/notifications.go`) | TS Type (`web/src/lib/types.ts`) | Notes |
|:---|:---|:---|
| `AutonomousTask` | `AutonomousTask` | Backlog missions, priority, status, progress, execution_log |
| `HeartbeatConfig` | `HeartbeatConfigData` | System core standing directives, interval, zero-noise |
| `AgentHeartbeatConfig` | `AgentHeartbeatConfig` | Per-agent directives, interval, target channel/account, active hours |
| `HeartbeatRun` | `HeartbeatRun` | Cognitive pulse execution audit record |
| `ChannelAccount` | `ChannelAccount` | Multi-account credentials, bound agents, routing_mode (`exclusive`, `mention`, `fallback`) |
| `Notification` | `NotificationItem` | Realtime notification alert with pagination & type |
| `TokenUsageSummary` | `TokenUsageSummary` | Full token usage stats & trend models |
| `TokenUsageRecord` | `TokenUsageRecord` | Token ledger transaction entry |
| `CronJob` | `CronJob` | Proactive cron definition |
| `AgentManifest` | `AgentManifest` | Must stay in sync. Go uses `llm.ModelConfig`, TS has inline `ModelConfig` |
| `PluginInfo` | `PluginInfo` | Full WASM plugin descriptor with manifest, status, and memory stats |
| `PluginManifest` | `PluginManifest` | Plugin manifest descriptor, capabilities, permissions, config schemas |
| `PluginPermissions` | `PluginPermissions` | Granular capability permissions (egress, vault, storage, bus) |
| `PluginStatus` | `PluginStatus` | `running`, `stopped`, `disabled`, `error` |
| `PluginToolDef` | `PluginToolDef` | Tool definition within plugin |
| `PluginChannelDef` | `PluginChannelDef` | Channel adapter definition within plugin |
| `PluginConnectorDef` | `PluginConnectorDef` | SaaS connector definition within plugin |
| `DelegationScope` | `DelegationScope` | Same field names |
| `TriggerRule` | `TriggerRule` | Same field names |
| `AgentStatus` | `AgentStatus` | Values: `active`, `stopped`, `error` |
| `ApprovalLevel` | `ApprovalLevel` | Values: `Low`, `Medium`, `High` |
| `SubTask` / `SubTaskResult` | — | Not exposed to frontend (internal only) |
| `AgentStreamEvent` | — | Consumed via SSE, not a REST type |
| `AuditLogEntry` | — | Future: needs TS type for audit log UI |
| `ApprovalRequest` (`internal/tools/approval.go`) | `ApprovalRequest` | Durable exact-action human approval |
| `AgentRun` / `RunEvent` (`internal/agent/runs.go`) | `AgentRun` / `RunEvent` | Durable execution and tracing contracts |
| — | `ConversationItem` | Defined in both `types.ts` and `api.ts` (duplicate) |
| — | `ChatMessageRecord` | TS only |
| — | `ToolInfo` | TS only |
| — | `SystemMetrics` | TS only |
| — | `TailscaleStatus` | TS only |
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

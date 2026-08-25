import type {
  AgentManifest,
  CatalogResponse,
  ChannelAccount,
  HealthReport,
  LLMProviderInfo,
  ToolInfo,
  SystemMetrics,
  TailscaleStatus,
  MCPServerStatus,
  MutationResult,
  PluginInfo,
  RegistryPlugin,
  VaultSecretMeta,
} from './types';
import { API_BASE, fetchJSON, getAuthHeaders, HTTP_STATUS_ACCEPTED } from './api/client';
import { operationsApi, type OTAStatus } from './api/operations';
export { API_BASE, createRealtimeSocket, fetchJSON, getAuthHeaders } from './api/client';
export type { OTAStatus, WifiNetwork } from './api/operations';

export interface ConversationItem {
  id: string;
  agent_id: string;
  title: string;
  created_at: string;
  updated_at: string;
}

export interface MessageRecordItem {
  id: string;
  conversation_id: string;
  role: 'user' | 'assistant' | 'system' | 'tool';
  content: string;
  tool_calls_json?: string;
  created_at: string;
}

export interface ProviderKeysData {
  providers?: LLMProviderInfo[];
  anthropic_configured: boolean;
  anthropic_masked: string;
  gemini_configured: boolean;
  gemini_masked: string;
  openai_configured: boolean;
  openai_masked: string;
  deepseek_configured: boolean;
  deepseek_masked: string;
  ollama_url: string;
}

export interface UserIdentityProfile {
  user_name: string;
  user_role?: string;
  language: string;
  timezone?: string;
  communication_style?: string;
  bio?: string;
  custom_instructions?: string;
  soul?: string;
  updated_at?: string;
}

export type HealthData = HealthReport;

export type IntegrationIdentity = Record<string, unknown> | null;

export interface AuditLogItem {
  timestamp: string;
  trace_id: string;
  agent_id: string;
  tool_name: string;
  risk_level: string;
  execution_time_ms: number;
  status: string;
  error?: string;
  previous_hash?: string;
  entry_hash?: string;
}

export interface StorageInfoData {
  storage_bytes: number;
  vectors_bytes: number;
  workspace_bytes: number;
  logs_bytes: number;
  total_bytes: number;
}

export interface HubSkillItem {
  id: string;
  slug?: string;
  name: string;
  description: string;
  category: string;
  author: string;
  version?: string;
  icon?: string;
  tags?: string[];
  stars?: number;
  updatedAt?: number | string;
  contentLanguage?: string;
  path?: string;
  files?: string[];
  isMultiFile?: boolean;
  skillUrl?: string;
  sourceGithubUrl?: string;
  installed: boolean;
  skill_md?: string;
}

export interface CronJobItem {
  id: string;
  agent_id: string;
  name: string;
  cron_expr: string;
  prompt: string;
  target_channel?: string;
  target_account_id?: string;
  target_recipient?: string;
  channel?: string;
  account_id?: string;
  recipient?: string;
  enabled: boolean;
  last_run?: string;
  next_run?: string;
}

export interface WorkspaceFileItem {
  name: string;
  path: string;
  is_dir: boolean;
  size: number;
  mod_time: string;
}

export interface DashboardSummaryData {
  metrics?: SystemMetrics;
  tailscale?: TailscaleStatus;
  agents_count: number;
  agents_active: number;
  tools_count: number;
  tools_native: number;
  tools_mcp: number;
  tools_skills: number;
  tools_wasm: number;
  cron_count: number;
  storage: {
    storage_bytes: number;
    vectors_bytes: number;
    workspace_bytes: number;
    logs_bytes: number;
    total_bytes: number;
  };
  recent_audit: AuditLogItem[];
  timestamp: string;
}

export interface ChannelAuthorizationItem {
  channel_id: string;
  sender_id: string;
  sender_name: string;
  paired_at: string;
  last_active_at: string;
  status: string;
}

export const api = {
  ...operationsApi,
  // Authentication & Initial Setup
  getAuthStatus: () =>
    fetchJSON<{ initialized: boolean; authenticated: boolean; user_name?: string }>('/auth/status'),
  setupInitialAdmin: (data: {
    password: string;
    user_name?: string;
    user_role?: string;
    language?: string;
    timezone?: string;
    communication_style?: string;
    custom_instructions?: string;
  }) =>
    fetchJSON<{ status: string }>('/auth/setup', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  login: (password: string) =>
    fetchJSON<{ status: string }>('/auth/login', {
      method: 'POST',
      body: JSON.stringify({ password }),
    }),
  logout: () =>
    fetchJSON<{ status: string }>('/auth/logout', {
      method: 'POST',
    }),
  changeAdminPassword: (currentPassword: string, newPassword: string) =>
    fetchJSON<{ status: string }>('/auth/password', {
      method: 'PUT',
      body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
    }),

  // Health & Dashboard
  getHealth: () => fetchJSON<HealthData>('/health'),
  getReady: () => fetchJSON<HealthData>('/ready'),
  getDashboardSummary: () => fetchJSON<DashboardSummaryData>('/dashboard/summary'),

  // Agents, Soul & Cron
  listAgents: () => fetchJSON<{ agents: AgentManifest[]; count: number }>('/agents'),
  getAgent: (id: string) => fetchJSON<AgentManifest>(`/agents/${id}`),
  createAgent: (manifest: Partial<AgentManifest>) =>
    fetchJSON<AgentManifest>('/agents', {
      method: 'POST',
      body: JSON.stringify(manifest),
    }),
  updateAgent: (id: string, manifest: Partial<AgentManifest>) =>
    fetchJSON<AgentManifest>(`/agents/${id}`, {
      method: 'PUT',
      body: JSON.stringify(manifest),
    }),
  deleteAgent: (id: string) =>
    fetchJSON<{ status: string }>(`/agents/${id}`, { method: 'DELETE' }),
  startAgent: (id: string) =>
    fetchJSON<{ status: string }>(`/agents/${id}/start`, { method: 'POST' }),
  stopAgent: (id: string) =>
    fetchJSON<{ status: string }>(`/agents/${id}/stop`, { method: 'POST' }),
  getSoul: (agentID?: string) => {
    const url = agentID ? `/agents/${agentID}/soul` : '/agents/soul';
    return fetchJSON<{ content: string; soul: string; path?: string; agent_id?: string }>(url).then((r) => ({
      ...r,
      soul: r.soul || r.content,
      content: r.content || r.soul,
    }));
  },
  saveSoul: (content: string, agentID?: string) => {
    const url = agentID ? `/agents/${agentID}/soul` : '/agents/soul';
    return fetchJSON<{ status: string; path?: string; agent_id?: string }>(url, {
      method: 'PUT',
      body: JSON.stringify({ content, soul: content, agent_id: agentID }),
    });
  },
  getMemoryMD: (agentID?: string) => {
    const url = agentID ? `/agents/${agentID}/memory-md` : '/agents/memory-md';
    return fetchJSON<{ memory_md: string; agent_id?: string }>(url);
  },
  clearMemoryMD: (agentID?: string) => {
    const url = agentID ? `/agents/${agentID}/memory-md` : '/agents/memory-md';
    return fetchJSON<{ status: string; agent_id?: string }>(url, {
      method: 'DELETE',
    });
  },
  listCronJobs: () => fetchJSON<{ jobs: CronJobItem[]; count: number }>('/cron'),
  saveCronJob: (job: Partial<CronJobItem> & { target_channel?: string; target_account_id?: string; target_recipient?: string }) =>
    fetchJSON<{ status: string; job?: CronJobItem; job_id?: string }>('/cron', {
      method: 'POST',
      body: JSON.stringify({
        ...job,
        channel: job.channel || job.target_channel,
        account_id: job.account_id || job.target_account_id || 'all',
        target_account_id: job.target_account_id || job.account_id || 'all',
        recipient: job.recipient || job.target_recipient,
      }),
    }),
  deleteCronJob: (id: string) =>
    fetchJSON<{ status: string }>(`/cron/${id}`, { method: 'DELETE' }),
  triggerCronJob: (id: string) =>
    fetchJSON<{ status: string; message: string }>(`/cron/${id}/run`, { method: 'POST' }),

  // Chat Execution
  chat: (agentID: string, message: string, conversationID?: string) =>
    fetchJSON<{
      response: string;
      conversation_id: string;
      tool_calls?: unknown[];
      usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number };
    }>(`/agents/${agentID}/chat`, {
      method: 'POST',
      body: JSON.stringify({ message, conversation_id: conversationID }),
    }),
  streamChat: (agentID: string, data: { message: string; conversation_id?: string | null }, signal?: AbortSignal) =>
    fetch(`${API_BASE}/agents/${agentID}/chat`, {
      method: 'POST',
      headers: getAuthHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ ...data, stream: true }),
      signal,
    }),

  // Conversations
  listConversations: (agentID?: string) =>
    fetchJSON<{ conversations: ConversationItem[]; count: number }>(
      `/conversations${agentID ? `?agent_id=${agentID}` : ''}`
    ),
  createConversation: (agentID: string, title?: string) =>
    fetchJSON<ConversationItem>('/conversations', {
      method: 'POST',
      body: JSON.stringify({ agent_id: agentID, title }),
    }),
  getConversation: (id: string) =>
    fetchJSON<{ conversation: ConversationItem; messages: MessageRecordItem[] }>(`/conversations/${id}`),
  deleteConversation: (id: string) =>
    fetchJSON<{ status: string }>(`/conversations/${id}`, { method: 'DELETE' }),
  updateConversationTitle: (id: string, title: string) =>
    fetchJSON<{ status: string; title?: string }>(`/conversations/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ title }),
    }),
  togglePinConversation: (id: string, is_pinned: boolean) =>
    fetchJSON<{ status: string; is_pinned: boolean }>(`/conversations/${id}/pin`, {
      method: 'PUT',
      body: JSON.stringify({ is_pinned }),
    }),

  // Tools Registry & MCP Host
  listTools: (category?: string) =>
    fetchJSON<{ tools: ToolInfo[]; count: number }>(`/tools${category ? `?category=${category}` : ''}`),
  connectMCP: (cfg: { id: string; transport?: string; command?: string; args?: string[]; env?: Record<string, string>; url?: string }) =>
    fetchJSON<MutationResult<{ status: string; server_id: string; tools_discovered: number }>>('/tools/mcp', {
      method: 'POST',
      body: JSON.stringify({ transport: cfg.transport || 'stdio', ...cfg }),
    }),
  disconnectMCP: (id: string) =>
    fetchJSON<MutationResult<{ status: string }>>(`/tools/mcp/${id}`, { method: 'DELETE' }),
  listMCPServers: () =>
    fetchJSON<{ servers: MCPServerStatus[] }>('/tools/mcp'),
  toggleMCPServer: (id: string, enabled: boolean) =>
    fetchJSON<MutationResult<{ status: string; enabled: boolean }>>(`/tools/mcp/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    }),
  executeTool: (name: string, input: Record<string, unknown>) =>
    fetchJSON<unknown>('/tools/execute?test=true', {
      method: 'POST',
      body: JSON.stringify({ name, tool: name, input, arguments: input }),
    }),
  createSkill: (skill: { name: string; description: string; content: string }) =>
    fetchJSON<MutationResult<{ status: string; name: string; path: string }>>('/tools/skill', {
      method: 'POST',
      body: JSON.stringify(skill),
    }),
  uploadWASM: (file: File) => {
    const fd = new FormData();
    fd.append('file', file);
    return fetch(`${API_BASE}/plugins/upload`, {
      method: 'POST',
      headers: getAuthHeaders(),
      body: fd,
    }).then(async (response) => {
      if (!response.ok) throw new Error(`HTTP Error ${response.status}`);
      const envelope = await response.json();
      const data = envelope.data ?? envelope;
      if (response.status === HTTP_STATUS_ACCEPTED && data?.approval) {
        window.dispatchEvent(new CustomEvent('actonos:approval-required', { detail: data.approval }));
      }
      return data as MutationResult<{ status: string; filename?: string }>;
    });
  },

  // WASM Plugin Subsystem
  listPlugins: () => fetchJSON<{ plugins: PluginInfo[]; count: number }>('/plugins'),
  listAvailablePlugins: () => fetchJSON<{ catalog: RegistryPlugin[]; count: number }>('/plugins/available'),
  installAvailablePlugin: (pluginId: string, downloadUrl?: string) =>
    fetchJSON<MutationResult<{ status: string; plugin: PluginInfo }>>('/plugins/install', {
      method: 'POST',
      body: JSON.stringify({ plugin_id: pluginId, download_url: downloadUrl }),
    }),
  uploadPlugin: (file: File, id?: string, manifest?: string) => {
    const fd = new FormData();
    fd.append('file', file);
    if (id) fd.append('id', id);
    if (manifest) fd.append('manifest', manifest);
    return fetch(`${API_BASE}/plugins/upload`, {
      method: 'POST',
      headers: getAuthHeaders(),
      body: fd,
    }).then(async (response) => {
      if (!response.ok) throw new Error(`HTTP Error ${response.status}`);
      const envelope = await response.json();
      return (envelope.data ?? envelope) as MutationResult<{ status: string; plugin: PluginInfo }>;
    });
  },
  enablePlugin: (id: string) => fetchJSON<MutationResult<{ status: string; plugin: PluginInfo }>>(`/plugins/${encodeURIComponent(id)}/enable`, { method: 'POST' }),
  disablePlugin: (id: string) => fetchJSON<MutationResult<{ status: string; plugin: PluginInfo }>>(`/plugins/${encodeURIComponent(id)}/disable`, { method: 'POST' }),
  deletePlugin: (id: string) => fetchJSON<MutationResult<{ status: string; id: string }>>(`/plugins/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  getPluginLogs: (id: string) => fetchJSON<{ id: string; status: string; logs: string[] }>(`/plugins/${encodeURIComponent(id)}/logs`),
  updatePluginConfig: (id: string, config: Record<string, unknown>, secrets?: Record<string, string>) =>
    fetchJSON<MutationResult<{ status: string; plugin: PluginInfo }>>(`/plugins/${encodeURIComponent(id)}/config`, {
      method: 'POST',
      body: JSON.stringify({ config, secrets }),
    }),

  // Hardware Vault Secrets Management
  listVaultSecrets: () => fetchJSON<{ secrets: VaultSecretMeta[]; count: number }>('/vault/secrets'),
  getVaultSecret: (name: string) => fetchJSON<{ name: string; configured: boolean; masked: string; length: number; is_provider: boolean }>(`/vault/secrets/${encodeURIComponent(name)}`),
  setVaultSecret: (name: string, value: string) =>
    fetchJSON<MutationResult<{ status: string; name: string; updated_at: string }>>('/vault/secrets', {
      method: 'POST',
      body: JSON.stringify({ name, value }),
    }),
  deleteVaultSecret: (name: string) =>
    fetchJSON<MutationResult<{ status: string; name: string }>>(`/vault/secrets/${encodeURIComponent(name)}`, {
      method: 'DELETE',
    }),

  // Skills Public Community Hub
  listHubCatalog: () => fetchJSON<{ catalog: HubSkillItem[]; count: number }>('/tools/hub/catalog'),
  installHubSkill: (skillId: string) =>
    fetchJSON<MutationResult<{ status: string; skill: string }>>('/tools/hub/install', {
      method: 'POST',
      body: JSON.stringify({ skill_id: skillId }),
    }),
  uninstallHubSkill: (skillId: string) =>
    fetchJSON<MutationResult<{ status: string; skill: string }>>('/tools/hub/uninstall', {
      method: 'POST',
      body: JSON.stringify({ skill_id: skillId }),
    }),
  toggleSkill: (name: string, enabled: boolean) =>
    fetchJSON<{ name: string; enabled: boolean; status: string }>(`/tools/skills/${encodeURIComponent(name)}/toggle`, {
      method: 'PUT',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ enabled }),
    }),

  // System & LLM Provider Keys
  getAPIKeys: () => fetchJSON<ProviderKeysData>('/system/keys'),
  saveAPIKeys: (keys: {
    anthropic_key?: string;
    gemini_key?: string;
    openai_key?: string;
    deepseek_key?: string;
    ollama_url?: string;
    provider?: string;
    api_key?: string;
    base_url?: string;
    default_model?: string;
    enabled?: boolean;
  }) =>
    fetchJSON<{ status: string }>('/system/keys', {
      method: 'POST',
      body: JSON.stringify(keys),
    }),
  saveProviderKey: (params: {
    provider: string;
    api_key?: string;
    base_url?: string;
    default_model?: string;
    enabled?: boolean;
  }) =>
    fetchJSON<{ status: string }>('/system/keys', {
      method: 'POST',
      body: JSON.stringify(params),
    }),
  deleteProviderKey: (provider: string) =>
    fetchJSON<{ status: string; provider: string }>(`/system/keys/${provider}`, {
      method: 'DELETE',
    }),
  testAPIKey: (provider: string, key: string, url?: string, model?: string) =>
    fetchJSON<{ status: string; response: string; model: string; latency_ms: number }>('/system/keys/test', {
      method: 'POST',
      body: JSON.stringify({ provider, key, url, model }),
    }),
  getAuditLogs: () => fetchJSON<{ entries: AuditLogItem[]; count: number }>('/system/audit'),
  verifyAuditChain: () => fetchJSON<{ status: string; message: string }>('/system/audit/verify'),
  getStorageInfo: () => fetchJSON<StorageInfoData>('/system/storage'),
  getModelsCatalog: () => fetchJSON<CatalogResponse>('/models'),
  getIdentity: () => fetchJSON<UserIdentityProfile>('/system/identity'),
  saveIdentity: (profile: Partial<UserIdentityProfile>) =>
    fetchJSON<{ status: string; profile: UserIdentityProfile; updated_at: string }>('/system/identity', {
      method: 'PUT',
      body: JSON.stringify(profile),
    }),
  checkOTA: () =>
    fetchJSON<OTAStatus>(
      '/system/ota/check',
      { method: 'POST' }
    ),

  // Chat Channels
  getChannels: () =>
    fetchJSON<{
      telegram: ChannelAccount[];
      discord: ChannelAccount[];
      whatsapp: ChannelAccount[];
      slack: ChannelAccount[];
      webhook_secret: string;
      webhook_url: string;
    }>('/integrations/channels'),
  listAllChannelAccounts: () =>
    fetchJSON<{ accounts: ChannelAccount[]; count: number }>('/integrations/channels/accounts'),
  saveChannels: (cfg: {
    telegram_token?: string;
    discord_token?: string;
    whatsapp_token?: string;
    whatsapp_phone_id?: string;
    webhook_secret?: string;
    telegram_accounts?: ChannelAccount[];
    discord_accounts?: ChannelAccount[];
    whatsapp_accounts?: ChannelAccount[];
    slack_accounts?: ChannelAccount[];
  }) =>
    fetchJSON<{ status: string }>('/integrations/channels', {
      method: 'POST',
      body: JSON.stringify(cfg),
    }),
  generatePairingCode: (channel_id: string = 'telegram') =>
    fetchJSON<{ code: string; channel_id: string; expires_in: number }>('/integrations/pairing/code', {
      method: 'POST',
      body: JSON.stringify({ channel_id }),
    }),
  verifyPairingCode: (data: { channel_id: string; code: string; sender_id: string; sender_name: string }) =>
    fetchJSON<{ status: string; sender: string }>('/integrations/pairing/verify', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  listAuthorizations: (channel_id?: string) =>
    fetchJSON<{ users: ChannelAuthorizationItem[]; count: number }>(
      `/integrations/authorizations${channel_id ? `?channel_id=${channel_id}` : ''}`
    ),
  revokeAuthorization: (data: { channel_id: string; sender_id: string }) =>
    fetchJSON<{ status: string }>('/integrations/authorizations', {
      method: 'DELETE',
      body: JSON.stringify(data),
    }),

  // Autonomous Tasks & Mission Control
  listTasks: (params?: { status?: string; priority?: string }) => {
    const query = new URLSearchParams();
    if (params?.status) query.set('status', params.status);
    if (params?.priority) query.set('priority', params.priority);
    const qStr = query.toString() ? `?${query.toString()}` : '';
    return fetchJSON<{ tasks: import('./types').AutonomousTask[]; count: number }>(`/tasks${qStr}`);
  },
  createTask: (data: Partial<import('./types').AutonomousTask>) =>
    fetchJSON<import('./types').AutonomousTask>('/tasks', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  getTask: (id: string) => fetchJSON<import('./types').AutonomousTask>(`/tasks/${id}`),
  updateTask: (id: string, data: Partial<import('./types').AutonomousTask>) =>
    fetchJSON<import('./types').AutonomousTask>(`/tasks/${id}`, {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  deleteTask: (id: string) =>
    fetchJSON<{ status: string; id: string }>(`/tasks/${id}`, {
      method: 'DELETE',
    }),

  // Heartbeat Configuration & Manual Pulse Trigger
  getHeartbeatConfig: () =>
    fetchJSON<import('./types').HeartbeatConfigData>('/heartbeat/config'),
  saveHeartbeatConfig: (data: import('./types').HeartbeatConfigData) =>
    fetchJSON<import('./types').HeartbeatConfigData>('/heartbeat/config', {
      method: 'PUT',
      body: JSON.stringify(data),
    }),
  triggerHeartbeatPulse: () =>
    fetchJSON<{ status: string; message?: string }>('/heartbeat/trigger', {
      method: 'POST',
    }),
  listHeartbeatRuns: () =>
    fetchJSON<import('./types').HeartbeatRun[]>('/heartbeat/runs'),

  // Notification Center
  listNotifications: (params?: { page?: number; limit?: number; type?: string; unread_only?: boolean }) => {
    const query = new URLSearchParams();
    if (params?.page) query.set('page', String(params.page));
    if (params?.limit) query.set('limit', String(params.limit));
    if (params?.type && params.type !== 'all') query.set('type', params.type);
    if (params?.unread_only) query.set('unread_only', 'true');
    const qStr = query.toString() ? `?${query.toString()}` : '';
    return fetchJSON<import('./types').NotificationListResponse>(`/notifications${qStr}`);
  },
  getUnreadNotificationsCount: () =>
    fetchJSON<{ unread_count: number }>('/notifications/unread-count'),
  markNotificationRead: (id?: string, all?: boolean) =>
    fetchJSON<{ status: string; unread_count: number }>('/notifications/mark-read', {
      method: 'POST',
      body: JSON.stringify({ id, all }),
    }),
  deleteNotification: (id: string) =>
    fetchJSON<{ status: string }>(`/notifications?id=${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
  clearAllNotifications: () =>
    fetchJSON<{ status: string }>('/notifications?all=true', {
      method: 'DELETE',
    }),
  getVAPIDPublicKey: () =>
    fetchJSON<import('./types').VAPIDKeyResponse>('/notifications/push/vapid-key'),
  subscribePush: (payload: import('./types').PushSubscriptionPayload) =>
    fetchJSON<{ status: string; message: string }>('/notifications/push/subscribe', {
      method: 'POST',
      body: JSON.stringify(payload),
    }),
  unsubscribePush: (endpoint: string) =>
    fetchJSON<{ status: string }>('/notifications/push/unsubscribe', {
      method: 'POST',
      body: JSON.stringify({ endpoint }),
    }),
  testPushNotification: (params?: { title?: string; message?: string; link?: string }) =>
    fetchJSON<{ status: string; notification: import('./types').NotificationItem }>('/notifications/push/test', {
      method: 'POST',
      body: JSON.stringify(params || {}),
    }),
  getTerminalInfo: () =>
    fetchJSON<import('./types').TerminalInfoResponse>('/terminal/info'),

  // Workspace File Manager & Semantic Studio
  listWorkspaceFiles: (parentId: string = '') =>
    fetchJSON<import('./types').WorkspaceFileListResponse>(`/workspace/files?parent_id=${encodeURIComponent(parentId)}`),
  getWorkspaceFile: (fileId: string) =>
    fetchJSON<import('./types').WorkspaceFileDetailResponse>(`/workspace/file?id=${encodeURIComponent(fileId)}`),
  saveWorkspaceFile: (input: { fileId?: string; parentId?: string; name?: string; content: string; expectedVersion?: number }) =>
    fetchJSON<MutationResult<import('./types').WorkspaceMutationResponse>>('/workspace/file', {
      method: 'POST',
      body: JSON.stringify({
        file_id: input.fileId,
        parent_id: input.parentId,
        name: input.name,
        content: input.content,
        expected_version: input.expectedVersion,
      }),
    }),
  deleteWorkspaceFile: (fileId: string, expectedVersion?: number, recursive = false) =>
    fetchJSON<MutationResult<import('./types').WorkspaceMutationResponse>>(`/workspace/file?id=${encodeURIComponent(fileId)}&expected_version=${expectedVersion || ''}&recursive=${recursive}`, {
      method: 'DELETE',
    }),
  mkdirWorkspace: (parentId: string, name: string) =>
    fetchJSON<MutationResult<import('./types').WorkspaceMutationResponse>>('/workspace/mkdir', {
      method: 'POST',
      body: JSON.stringify({ parent_id: parentId, name }),
    }),
  renameWorkspaceFile: (fileId: string, parentId: string, name: string, expectedVersion: number) =>
    fetchJSON<MutationResult<import('./types').WorkspaceMutationResponse>>('/workspace/rename', {
      method: 'POST',
      body: JSON.stringify({ file_id: fileId, parent_id: parentId, name, expected_version: expectedVersion }),
    }),
  duplicateWorkspaceFile: (fileId: string, parentId: string, name?: string) =>
    fetchJSON<MutationResult<import('./types').WorkspaceMutationResponse>>('/workspace/duplicate', {
      method: 'POST',
      body: JSON.stringify({ file_id: fileId, parent_id: parentId, name }),
    }),
  getWorkspaceStats: () =>
    fetchJSON<import('./types').WorkspaceStatsResponse>('/workspace/stats'),
  reindexWorkspaceFile: (fileId: string) =>
    fetchJSON<{ status: string; id: string; file_id: string }>('/workspace/reindex', {
      method: 'POST',
      body: JSON.stringify({ file_id: fileId }),
    }),
  getWorkspaceChunks: (fileId: string) =>
    fetchJSON<import('./types').WorkspaceChunksResponse>(`/workspace/chunks?id=${encodeURIComponent(fileId)}`),
  getWorkspaceZipUrl: (fileId?: string, fileIds?: string[]) => {
    if (fileIds && fileIds.length > 0) {
      return `/api/workspace/zip?ids=${encodeURIComponent(fileIds.join(','))}`;
    }
    return `/api/workspace/zip?id=${encodeURIComponent(fileId || '')}`;
  },
  getWorkspaceRawUrl: (fileId: string, download?: boolean) =>
    `/api/workspace/raw?id=${encodeURIComponent(fileId)}${download ? '&download=1' : ''}`,
  uninstallPlugin: (id: string) =>
    fetchJSON<MutationResult<{ status: string; id: string }>>(`/plugins/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    }),
};

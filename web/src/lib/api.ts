import type {
  AgentManifest,
  ChannelAccount,
  ConnectorInfo,
  LLMProviderInfo,
  ToolInfo,
  SystemMetrics,
  TailscaleStatus,
} from './types';

export const API_BASE = '/api';

export function getAuthHeaders(extra?: Record<string, string>): Record<string, string> {
  const token = localStorage.getItem('actonos_token');
  const headers: Record<string, string> = {
    ...extra,
  };
  if (token) {
    headers['Authorization'] = `Bearer ${token}`;
  }
  return headers;
}

async function fetchJSON<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const headers = getAuthHeaders({
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string>),
  });

  const res = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers,
  });

  if (!res.ok) {
    let errorMsg = `HTTP Error ${res.status}`;
    try {
      const errBody = await res.json();
      if (errBody.error?.message) {
        errorMsg = errBody.error.message;
      }
    } catch {
      // Ignore json parse error on non-json response
    }
    throw new Error(errorMsg);
  }

  const json = await res.json();
  return json.data !== undefined ? json.data : json;
}

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

export interface AuditLogItem {
  timestamp: string;
  trace_id: string;
  agent_id: string;
  tool_name: string;
  risk_level: string;
  execution_time_ms: number;
  status: string;
  error?: string;
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
  name: string;
  description: string;
  category: string;
  author: string;
  version: string;
  icon: string;
  tags: string[];
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
    fetchJSON<{ status: string; token: string }>('/auth/setup', {
      method: 'POST',
      body: JSON.stringify(data),
    }),
  login: (password: string) =>
    fetchJSON<{ status: string; token: string }>('/auth/login', {
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
  getHealth: () => fetchJSON<any>('/health'),
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
      tool_calls?: any[];
      usage?: { prompt_tokens: number; completion_tokens: number; total_tokens: number };
    }>(`/agents/${agentID}/chat`, {
      method: 'POST',
      body: JSON.stringify({ message, conversation_id: conversationID }),
    }),
  streamChat: (agentID: string, data: { message: string; conversation_id?: string | null }) =>
    fetch(`${API_BASE}/agents/${agentID}/chat`, {
      method: 'POST',
      headers: getAuthHeaders({ 'Content-Type': 'application/json' }),
      body: JSON.stringify({ ...data, stream: true }),
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
    fetchJSON<{ status: string; title: string }>(`/conversations/${id}`, {
      method: 'PUT',
      body: JSON.stringify({ title }),
    }),

  // Tools Registry & MCP Host
  listTools: (category?: string) =>
    fetchJSON<{ tools: ToolInfo[]; count: number }>(`/tools${category ? `?category=${category}` : ''}`),
  connectMCP: (cfg: { id: string; command: string; args?: string[]; env?: Record<string, string> }) =>
    fetchJSON<{ status: string; server_id: string; tools_discovered: number }>('/tools/mcp', {
      method: 'POST',
      body: JSON.stringify({ transport: 'stdio', ...cfg }),
    }),
  disconnectMCP: (id: string) =>
    fetchJSON<{ status: string }>(`/tools/mcp/${id}`, { method: 'DELETE' }),
  executeTool: (name: string, input: any) =>
    fetchJSON<any>('/tools/execute', {
      method: 'POST',
      body: JSON.stringify({ name, tool: name, input, arguments: input }),
    }),
  createSkill: (skill: { name: string; description: string; content: string }) =>
    fetchJSON<{ status: string; name: string; path: string }>('/tools/skill', {
      method: 'POST',
      body: JSON.stringify(skill),
    }),
  uploadWASM: (file: File) => {
    const fd = new FormData();
    fd.append('file', file);
    return fetch(`${API_BASE}/tools/wasm`, {
      method: 'POST',
      headers: getAuthHeaders(),
      body: fd,
    }).then((r) => r.json());
  },

  // Skills Public Community Hub
  listHubCatalog: () => fetchJSON<{ catalog: HubSkillItem[]; count: number }>('/tools/hub/catalog'),
  installHubSkill: (skillId: string) =>
    fetchJSON<{ status: string; skill: string }>('/tools/hub/install', {
      method: 'POST',
      body: JSON.stringify({ skill_id: skillId }),
    }),
  uninstallHubSkill: (skillId: string) =>
    fetchJSON<{ status: string; skill: string }>('/tools/hub/uninstall', {
      method: 'POST',
      body: JSON.stringify({ skill_id: skillId }),
    }),

  // Sandboxed Workspace
  listWorkspaceFiles: (dir?: string) =>
    fetchJSON<{ files: WorkspaceFileItem[]; dir: string; count: number }>(
      `/workspace/files${dir ? `?subpath=${encodeURIComponent(dir)}&dir=${encodeURIComponent(dir)}` : ''}`
    ).then((r) => ({
      files: r.files || [],
      dir: r.dir || dir || '',
      count: r.count || (r.files ? r.files.length : 0),
    })),
  getWorkspaceFile: (path: string) =>
    fetchJSON<{ path: string; content: string; size?: number }>(`/workspace/file?path=${encodeURIComponent(path)}`),
  saveWorkspaceFile: (path: string, content: string) =>
    fetchJSON<{ path: string; written: number }>('/workspace/file', {
      method: 'POST',
      body: JSON.stringify({ path, content }),
    }),
  deleteWorkspaceFile: (path: string) =>
    fetchJSON<{ status: string }>(`/workspace/file?path=${encodeURIComponent(path)}`, { method: 'DELETE' }),
  mkdirWorkspace: (path: string) =>
    fetchJSON<{ status: string; path: string }>('/workspace/mkdir', {
      method: 'POST',
      body: JSON.stringify({ path }),
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
  getStorageInfo: () => fetchJSON<StorageInfoData>('/system/storage'),
  getIdentity: () => fetchJSON<UserIdentityProfile>('/system/identity'),
  saveIdentity: (profile: Partial<UserIdentityProfile>) =>
    fetchJSON<{ status: string; profile: UserIdentityProfile; updated_at: string }>('/system/identity', {
      method: 'PUT',
      body: JSON.stringify(profile),
    }),
  checkOTA: () =>
    fetchJSON<{ current_version: string; update_available: boolean; latest_version: string; last_checked: string }>(
      '/system/ota/check',
      { method: 'POST' }
    ),

  // SaaS Connectors & OAuth 2.1 PKCE
  listIntegrations: () => fetchJSON<{ integrations: ConnectorInfo[]; count: number }>('/integrations'),
  getAuthURL: (provider: string, client_id?: string, client_secret?: string, redirect_uri?: string) =>
    fetchJSON<{ provider: string; auth_url: string; state: string; redirect_uri: string }>(
      `/integrations/${provider}/auth-url`,
      {
        method: 'POST',
        body: JSON.stringify({ client_id, client_secret, redirect_uri }),
      }
    ),
  saveDirectToken: (provider: string, token: string) =>
    fetchJSON<{ status: string; provider: string; auth_type: string; identity: any }>(
      `/integrations/${provider}/token`,
      {
        method: 'POST',
        body: JSON.stringify({ token }),
      }
    ),
  saveConnectorConfig: (provider: string, client_id: string, client_secret: string) =>
    fetchJSON<{ status: string }>(`/integrations/${provider}/config`, {
      method: 'POST',
      body: JSON.stringify({ client_id, client_secret }),
    }),
  testConnector: (provider: string) =>
    fetchJSON<{ status: string; provider: string; latency_ms: number; identity: any }>(
      `/integrations/${provider}/test`,
      { method: 'POST' }
    ),
  disconnectConnector: (provider: string) =>
    fetchJSON<{ status: string; provider: string }>(`/integrations/${provider}/disconnect`, {
      method: 'POST',
    }),
  toggleIntegration: (provider: string) =>
    fetchJSON<{ provider: string; connected: boolean }>(`/integrations/${provider}/toggle`, { method: 'POST' }),

  // Chat Channels
  getChannels: () =>
    fetchJSON<{
      telegram: ChannelAccount[];
      discord: ChannelAccount[];
      whatsapp: ChannelAccount[];
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

  // Observability, Tokens & History
  getTokenUsage: () => fetchJSON<import('./types').TokenUsageSummary>('/system/token-usage'),
  getHeartbeatHistory: () => fetchJSON<import('./types').HeartbeatRun[]>('/system/heartbeat/history'),
  getCronHistory: (jobID?: string) =>
    fetchJSON<import('./types').CronExecutionRecord[]>(
      jobID ? `/cron/${jobID}/history` : '/cron/history'
    ),

  // HAL & Network
  getMetrics: () => fetchJSON<SystemMetrics>('/system/metrics'),
  getTailscale: () => fetchJSON<TailscaleStatus>('/system/tailscale'),
  scanWifi: () => fetchJSON<{ networks: any[]; count: number }>('/system/wifi/scan'),
  connectWifi: (ssid: string, password?: string) =>
    fetchJSON<{ status: string }>('/system/wifi/connect', {
      method: 'POST',
      body: JSON.stringify({ ssid, password }),
    }),
  restartDaemon: () => fetchJSON<{ status: string }>('/system/restart', { method: 'POST' }),
  downloadBackup: async () => {
    const res = await fetch(`${API_BASE}/system/backup`, {
      headers: getAuthHeaders(),
    });
    if (!res.ok) throw new Error(`Backup failed: ${res.statusText}`);
    const blob = await res.blob();
    const url = window.URL.createObjectURL(blob);
    const a = document.createElement('a');
    a.href = url;
    a.download = `actonos-backup-${new Date().toISOString().slice(0, 10)}.db`;
    document.body.appendChild(a);
    a.click();
    window.URL.revokeObjectURL(url);
    document.body.removeChild(a);
  },
};

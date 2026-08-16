import type { AgentManifest, ToolInfo, SystemMetrics, TailscaleStatus } from './types';

const API_BASE = '/api';

async function fetchJSON<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${endpoint}`, {
    ...options,
    headers: {
      'Content-Type': 'application/json',
      ...options?.headers,
    },
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

export const api = {
  // Health
  getHealth: () => fetchJSON<any>('/health'),

  // Agents
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
    fetch(`${API_BASE}/agents/${id}`, { method: 'DELETE' }),
  startAgent: (id: string) =>
    fetchJSON<{ status: string }>(`/agents/${id}/start`, { method: 'POST' }),
  stopAgent: (id: string) =>
    fetchJSON<{ status: string }>(`/agents/${id}/stop`, { method: 'POST' }),

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

  // Tools
  listTools: (category?: string) =>
    fetchJSON<{ tools: ToolInfo[]; count: number }>(`/tools${category ? `?category=${category}` : ''}`),
  connectMCP: (cfg: { id: string; command: string; args?: string[] }) =>
    fetchJSON<{ status: string }>('/tools/mcp', {
      method: 'POST',
      body: JSON.stringify({ transport: 'stdio', ...cfg }),
    }),
  disconnectMCP: (id: string) =>
    fetchJSON<{ status: string }>(`/tools/mcp/${id}`, { method: 'DELETE' }),
  executeTool: (name: string, input: any) =>
    fetchJSON<any>('/tools/execute', {
      method: 'POST',
      body: JSON.stringify({ name, input }),
    }),
  createSkill: (skill: { name: string; description: string; content: string }) =>
    fetchJSON<{ status: string; name: string; path: string }>('/tools/skill', {
      method: 'POST',
      body: JSON.stringify(skill),
    }),
  uploadWASM: (file: File) => {
    const fd = new FormData();
    fd.append('file', file);
    return fetch('/api/tools/wasm', { method: 'POST', body: fd }).then((r) => r.json());
  },

  // Workspace
  listWorkspaceFiles: (dir?: string) =>
    fetchJSON<{ files: any[]; dir: string; count: number }>(`/workspace/files${dir ? `?dir=${encodeURIComponent(dir)}` : ''}`),
  getWorkspaceFile: (path: string) =>
    fetchJSON<{ path: string; content: string; size: number }>(`/workspace/file?path=${encodeURIComponent(path)}`),
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

  // System & Keys
  getAPIKeys: () => fetchJSON<ProviderKeysData>('/system/keys'),
  saveAPIKeys: (keys: {
    anthropic_key?: string;
    gemini_key?: string;
    openai_key?: string;
    deepseek_key?: string;
    ollama_url?: string;
  }) =>
    fetchJSON<{ status: string }>('/system/keys', {
      method: 'POST',
      body: JSON.stringify(keys),
    }),
  testAPIKey: (provider: string, key: string, url?: string) =>
    fetchJSON<{ status: string; response: string; model: string }>('/system/keys/test', {
      method: 'POST',
      body: JSON.stringify({ provider, key, url }),
    }),
  getAuditLogs: () => fetchJSON<{ entries: AuditLogItem[]; count: number }>('/system/audit'),
  getStorageInfo: () => fetchJSON<StorageInfoData>('/system/storage'),
  checkOTA: () => fetchJSON<{ current_version: string; update_available: boolean; latest_version: string; last_checked: string }>('/system/ota/check', {
    method: 'POST',
  }),

  // Integrations & Channels
  listIntegrations: () => fetchJSON<{ integrations: any[]; count: number }>('/integrations'),
  toggleIntegration: (provider: string) =>
    fetchJSON<{ provider: string; connected: boolean }>(`/integrations/${provider}/toggle`, { method: 'POST' }),
  getChannels: () =>
    fetchJSON<{
      telegram_enabled: boolean;
      telegram_bot: string;
      discord_enabled: boolean;
      discord_bot: string;
      webhook_secret: string;
      webhook_url: string;
    }>('/integrations/channels'),
  saveChannels: (cfg: { telegram_token?: string; discord_token?: string; webhook_secret?: string }) =>
    fetchJSON<{ status: string }>('/integrations/channels', {
      method: 'POST',
      body: JSON.stringify(cfg),
    }),

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
};

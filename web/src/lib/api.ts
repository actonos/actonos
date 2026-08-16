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

  // System & HAL
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

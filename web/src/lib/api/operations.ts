import type {
  AgentRun,
  ApprovalRequest,
  CronExecutionRecord,
  HeartbeatRun,
  MutationResult,
  RunEvent,
  SystemMetrics,
  TailscaleStatus,
  TokenUsageRecord,
  TokenUsageSummary,
} from '../types';
import { API_BASE, fetchJSON, getAuthHeaders } from './client';

export interface WifiNetwork {
  ssid: string;
  bssid?: string;
  signal: number;
  signal_strength?: number;
  security: string;
}

export interface OTAStatus {
  current_version: string;
  update_available: boolean;
  latest_version: string;
  last_checked: string;
}

export const operationsApi = {
  listApprovals: (status = 'pending') =>
    fetchJSON<{ approvals: ApprovalRequest[] }>(
      `/approvals?status=${encodeURIComponent(status)}`
    ),
  approveAction: (id: string, reason = '') =>
    fetchJSON<{ approval: ApprovalRequest; result: unknown }>(
      `/approvals/${id}/approve`,
      { method: 'POST', body: JSON.stringify({ reason }) }
    ),
  rejectAction: (id: string, reason = '') =>
    fetchJSON<ApprovalRequest>(`/approvals/${id}/reject`, {
      method: 'POST',
      body: JSON.stringify({ reason }),
    }),
  listAgentRuns: (limit = 100) =>
    fetchJSON<{ runs: AgentRun[] }>(`/runs?limit=${limit}`),
  listRunEvents: (runID: string) =>
    fetchJSON<{ events: RunEvent[] }>(`/runs/${runID}/events`),
  getTokenUsage: () => fetchJSON<TokenUsageSummary>('/system/token-usage'),
  getTokenHistory: (params?: { agent_id?: string; source?: string }) => {
    const query = new URLSearchParams();
    if (params?.agent_id) query.set('agent_id', params.agent_id);
    if (params?.source) query.set('source', params.source);
    const suffix = query.size ? `?${query.toString()}` : '';
    return fetchJSON<TokenUsageRecord[]>(`/system/token-usage/history${suffix}`);
  },
  getHeartbeatHistory: () =>
    fetchJSON<HeartbeatRun[]>('/system/heartbeat/history'),
  getCronHistory: (jobID?: string) =>
    fetchJSON<CronExecutionRecord[]>(
      jobID ? `/cron/${jobID}/history` : '/cron/history'
    ),
  getMetrics: () => fetchJSON<SystemMetrics>('/system/metrics'),
  getTailscale: () => fetchJSON<TailscaleStatus>('/system/tailscale'),
  scanWifi: () =>
    fetchJSON<{ networks: WifiNetwork[]; count: number }>('/system/wifi/scan'),
  connectWifi: (ssid: string, password?: string) =>
    fetchJSON<{ status: string }>('/system/wifi/connect', {
      method: 'POST',
      body: JSON.stringify({ ssid, password }),
    }),
  restartDaemon: () =>
    fetchJSON<MutationResult<{ status: string }>>('/system/restart', {
      method: 'POST',
    }),
  downloadBackup: async () => {
    const response = await fetch(`${API_BASE}/system/backup`, {
      headers: getAuthHeaders(),
    });
    if (!response.ok) throw new Error(`Backup failed: ${response.statusText}`);
    const blobURL = window.URL.createObjectURL(await response.blob());
    const anchor = document.createElement('a');
    anchor.href = blobURL;
    anchor.download = `actonos-backup-${new Date().toISOString().slice(0, 10)}.db`;
    document.body.appendChild(anchor);
    anchor.click();
    window.URL.revokeObjectURL(blobURL);
    anchor.remove();
  },
};

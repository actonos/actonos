import type {
  AgentRun,
  ApprovalRequest,
  CronExecutionRecord,
  DontAskAgain,
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

export interface OTAJob {
  id: string;
  action: string;
  status: string;
  version?: string;
  error?: string;
  progress?: number;
  started_at?: string;
  updated_at?: string;
}

export interface OTAAsset {
  name: string;
  role: string;
  required: boolean;
  present: boolean;
  download_url?: string;
  checksum?: string;
  checksum_missing: boolean;
}

export interface OTAStatus {
  current_version: string;
  update_available: boolean;
  latest_version: string;
  last_checked: string;
  apply_supported?: boolean;
  apply_unsupported_reason?: string;
  can_install?: boolean;
  checksum_missing?: boolean;
  allow_unsigned?: boolean;
  embeddingd_required?: boolean;
  error_code?: string;
  error_message?: string;
  retry_after?: number;
  git_commit?: string;
  build_time?: string;
  active_binary?: string;
  previous_binary?: string;
  assets?: OTAAsset[];
  job?: OTAJob | null;
}

export const operationsApi = {
  listApprovals: (status = 'pending') =>
    fetchJSON<{ approvals: ApprovalRequest[] }>(
      `/approvals?status=${encodeURIComponent(status)}`
    ),
  approveAction: (id: string, reason = '', dontAskAgain?: DontAskAgain) =>
    fetchJSON<{ approval: ApprovalRequest; result: unknown }>(
      `/approvals/${id}/approve`,
      { method: 'POST', body: JSON.stringify({ reason, dont_ask_again: dontAskAgain || undefined }) }
    ),
  rejectAction: (id: string, reason = '') =>
    fetchJSON<ApprovalRequest>(`/approvals/${id}/reject`, {
      method: 'POST',
      body: JSON.stringify({ reason }),
    }),
  listAgentRuns: (filter?: { limit?: number; status?: string; agent_id?: string; source?: string }) => {
    const query = new URLSearchParams();
    query.set('limit', String(filter?.limit ?? 100));
    if (filter?.status) query.set('status', filter.status);
    if (filter?.agent_id) query.set('agent_id', filter.agent_id);
    if (filter?.source) query.set('source', filter.source);
    return fetchJSON<{ runs: AgentRun[] }>(`/runs?${query.toString()}`);
  },
  cancelAgentRun: (runID: string) =>
    fetchJSON<{ status: string; id: string }>(`/runs/${runID}/cancel`, { method: 'POST' }),
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
  checkOTA: (force = true) =>
    fetchJSON<OTAStatus>('/system/ota/check', {
      method: 'POST',
      body: JSON.stringify({ force }),
    }),
  otaStatus: () => fetchJSON<OTAStatus>('/system/ota/status'),
  applyOTA: () =>
    fetchJSON<MutationResult<{ status: string }>>('/system/ota/apply', { method: 'POST' }),
  rollbackOTA: () =>
    fetchJSON<MutationResult<{ status: string }>>('/system/ota/rollback', { method: 'POST' }),
  getAgentRun: (id: string) => fetchJSON<{ run: AgentRun }>(`/runs/${id}`),
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

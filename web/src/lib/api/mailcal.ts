import { fetchJSON } from './client';

export type MailcalKind = 'mail' | 'calendar' | 'both';
export type MailcalStatus = 'ok' | 'auth_expired' | 'paused' | 'error';

export interface MailcalAccount {
  id: string;
  label: string;
  plugin_id: string;
  kind: MailcalKind;
  email?: string;
  config?: Record<string, unknown>;
  auth: { type: 'none' | 'oauth' | 'password'; provider?: string };
  status: MailcalStatus;
  status_detail?: string;
  created_at: string;
  updated_at: string;
}

export interface MailcalProvider {
  plugin_id: string;
  name: string;
  version: string;
  abi: string[];
  ops: string[];
  limits: { max_recipients?: number; max_body_bytes?: number; max_attachment_bytes?: number };
}

export interface MailcalTestResult {
  ok: boolean;
  detail?: string;
  checks?: string[];
}

export interface MailcalAccountInput {
  label: string;
  plugin_id: string;
  kind: MailcalKind;
  email?: string;
  config?: Record<string, unknown>;
}

export const mailcalApi = {
  listProviders: () => fetchJSON<{ providers: MailcalProvider[] }>('/mailcal/providers'),
  listAccounts: () => fetchJSON<{ accounts: MailcalAccount[]; count: number }>('/mailcal/accounts'),
  createAccount: (input: MailcalAccountInput) =>
    fetchJSON<MailcalAccount>('/mailcal/accounts', { method: 'POST', body: JSON.stringify(input) }),
  updateAccount: (id: string, input: Pick<MailcalAccountInput, 'label' | 'email' | 'config'>) =>
    fetchJSON<MailcalAccount>(`/mailcal/accounts/${encodeURIComponent(id)}`, { method: 'PUT', body: JSON.stringify(input) }),
  deleteAccount: (id: string) =>
    fetchJSON<{ status: string }>(`/mailcal/accounts/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  testAccount: (id: string) =>
    fetchJSON<MailcalTestResult>(`/mailcal/accounts/${encodeURIComponent(id)}/test`, { method: 'POST' }),
};

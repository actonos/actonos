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
  /** Set when the plugin signs users in through ActonOS, for example "google". */
  oauth_provider?: string;
  /** Which sign-in form to show: OAuth handled by the core, or an app password. */
  auth_kind?: 'oauth' | 'password';
  /** True when the plugin carries a conformance certificate for exactly this build. Informational only. */
  verified?: boolean;
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
  auth?: { type: 'none' | 'oauth' | 'password' };
}

export type MailcalVerdict = 'allow' | 'needs_approval' | 'deny';

export interface MailcalDecisionResult {
  verdict: MailcalVerdict;
  rule_id?: string;
  reasons: string[];
}

export type MailcalRuleAction = 'reply' | 'send_new' | 'calendar_accept' | 'calendar_create' | 'calendar_update';

export interface MailcalRuleInput {
  enabled: boolean;
  account_id: string;
  action: MailcalRuleAction;
  effect: 'allow' | 'deny';
  allow_public_domain?: boolean;
  expires_at?: string | null;
  match: {
    recipients: { addresses?: string[]; domains?: string[] };
    reply_only?: boolean;
    body: { mode?: 'template' | 'free'; template?: string; allow_links?: boolean };
    calendar?: { working_hours?: string; max_minutes?: number; attendee_domains?: string[] };
  };
  limits: { per_hour?: number; per_day?: number };
}

export interface MailcalRule extends MailcalRuleInput {
  id: string;
  created_by: string;
  created_at: string;
  paused_reason?: string;
  paused_at?: string;
}

export interface MailcalProposal {
  id: string;
  source: 'agent' | 'user';
  agent_id?: string;
  rule: MailcalRuleInput;
  rationale?: string;
  status: 'pending' | 'approved' | 'rejected';
  rule_id?: string;
  created_at: string;
}

export interface MailcalDecision {
  id: string;
  at: string;
  phase: 'gate' | 'executed' | 'bounce';
  account_id: string;
  agent_id: string;
  tool: string;
  draft_id?: string;
  payload_hash?: string;
  verdict: MailcalVerdict;
  rule_id?: string;
  reasons?: string[];
  approved?: boolean;
  outcome?: string;
}

export interface MailcalDraft {
  id: string;
  account_id: string;
  created_by: string;
  payload: {
    to: { email: string; name?: string }[];
    cc?: { email: string; name?: string }[];
    bcc?: { email: string; name?: string }[];
    subject: string;
    body_text: string;
    in_reply_to?: string;
  };
  payload_hash: string;
  reply?: { verified: boolean; participants: string[] };
}

export interface MailcalOAuthStart {
  auth_url: string;
  redirect_uri: string;
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
  putPassword: (id: string, password: string) =>
    fetchJSON<{ configured: boolean }>(`/mailcal/accounts/${encodeURIComponent(id)}/password`, {
      method: 'PUT',
      body: JSON.stringify({ password }),
    }),
  startOAuth: (id: string) =>
    fetchJSON<MailcalOAuthStart>(`/mailcal/accounts/${encodeURIComponent(id)}/oauth/start`, { method: 'POST' }),
  getOAuthClient: (provider: string) =>
    fetchJSON<{ configured: boolean }>(`/mailcal/oauth/clients/${encodeURIComponent(provider)}`),
  putOAuthClient: (provider: string, clientId: string, clientSecret: string) =>
    fetchJSON<{ configured: boolean }>(`/mailcal/oauth/clients/${encodeURIComponent(provider)}`, {
      method: 'PUT',
      body: JSON.stringify({ client_id: clientId, client_secret: clientSecret }),
    }),
  getSettings: () => fetchJSON<{ enabled: boolean }>('/mailcal/settings'),
  putSettings: (enabled: boolean) =>
    fetchJSON<{ enabled: boolean }>('/mailcal/settings', { method: 'PUT', body: JSON.stringify({ enabled }) }),
  listRules: (account?: string) =>
    fetchJSON<{ rules: MailcalRule[]; count: number }>(`/mailcal/rules${account ? `?account=${encodeURIComponent(account)}` : ''}`),
  createRule: (rule: MailcalRuleInput) =>
    fetchJSON<MailcalRule>('/mailcal/rules', { method: 'POST', body: JSON.stringify(rule) }),
  setRuleEnabled: (id: string, enabled: boolean) =>
    fetchJSON<{ enabled: boolean }>(`/mailcal/rules/${encodeURIComponent(id)}/enabled`, {
      method: 'PUT',
      body: JSON.stringify({ enabled }),
    }),
  deleteRule: (id: string) => fetchJSON<{ status: string }>(`/mailcal/rules/${encodeURIComponent(id)}`, { method: 'DELETE' }),
  dryRun: (rule: MailcalRuleInput, draftId: string) =>
    fetchJSON<MailcalDecisionResult>('/mailcal/rules/dry-run', {
      method: 'POST',
      body: JSON.stringify({ rule, draft_id: draftId }),
    }),
  listProposals: (status = 'pending') =>
    fetchJSON<{ proposals: MailcalProposal[]; count: number }>(`/mailcal/proposals?status=${encodeURIComponent(status)}`),
  approveProposal: (id: string) =>
    fetchJSON<MailcalRule>(`/mailcal/proposals/${encodeURIComponent(id)}/approve`, { method: 'POST' }),
  rejectProposal: (id: string) =>
    fetchJSON<{ status: string }>(`/mailcal/proposals/${encodeURIComponent(id)}/reject`, { method: 'POST' }),
  proposeFromDraft: (draftId: string) =>
    fetchJSON<MailcalProposal>(`/mailcal/drafts/${encodeURIComponent(draftId)}/rule-proposal`, { method: 'POST' }),
  evaluateDraft: (draftId: string) =>
    fetchJSON<MailcalDecisionResult>(`/mailcal/drafts/${encodeURIComponent(draftId)}/evaluation`),
  getDraft: (draftId: string) => fetchJSON<MailcalDraft>(`/mailcal/drafts/${encodeURIComponent(draftId)}`),
  listDecisions: (q: { account?: string; verdict?: string; phase?: string; limit?: number } = {}) => {
    const params = new URLSearchParams();
    Object.entries(q).forEach(([k, v]) => {
      if (v !== undefined && v !== '') params.set(k, String(v));
    });
    const qs = params.toString();
    return fetchJSON<{ decisions: MailcalDecision[]; count: number }>(`/mailcal/decisions${qs ? `?${qs}` : ''}`);
  },
};

/** Tools whose effect leaves the device. The server refuses "don't ask again" for them. */
export const EXTERNAL_EFFECT_TOOLS: readonly string[] = [
  'native_mail_send',
  'native_mail_delete',
  'native_calendar_create_event',
  'native_calendar_update_event',
  'native_calendar_respond',
  'native_calendar_delete_event',
];

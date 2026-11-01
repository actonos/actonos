import type { CronJobItem } from '@/lib/api';
import type { AgentManifest, ModelConfig } from '@/lib/types';
import { EXTERNAL_EFFECT_TOOLS } from '@/lib/api/mailcal';

export type TriageInterval = 5 | 15 | 30 | 60;

export const TRIAGE_AGENT_ID = 'agent_mail_triage';
export const TRIAGE_JOB_ID = 'cron_mail_triage';

/** Tools the triage agent may use when it can only read and draft. */
export const TRIAGE_READ_TOOLS: readonly string[] = [
  'native_mail_list',
  'native_mail_search',
  'native_mail_get',
  'native_mail_get_thread',
  'native_mail_draft',
  'native_mail_mark',
  'native_calendar_list',
  'native_calendar_get',
  'native_calendar_free_busy',
];

/** Added when the owner lets the agent act under their rules; the gate still decides every call. */
export const TRIAGE_ACT_TOOLS: readonly string[] = ['native_mail_send', 'native_calendar_respond'];

/** Tools that must never be given to an unattended agent that reads strangers' mail. */
export const FORBIDDEN_FOR_TRIAGE: readonly string[] = ['native_exec', 'native_cron_schedule', 'native_http_fetch', 'native_file_write', 'native_file_delete'];

export function cronExprFor(minutes: TriageInterval): string {
  return minutes === 60 ? '0 * * * *' : `*/${minutes} * * * *`;
}

export interface TriageTexts {
  instructions: string;
  prompt: string;
  autoSendClause: string;
}

export interface TriageOptions {
  everyMinutes: TriageInterval;
  allowAutoSend: boolean;
  modelConfig: ModelConfig;
  texts: TriageTexts;
}

export function buildTriageAgent(opts: TriageOptions): Partial<AgentManifest> {
  const tools = [...TRIAGE_READ_TOOLS, ...(opts.allowAutoSend ? TRIAGE_ACT_TOOLS : [])];
  return {
    agent_id: TRIAGE_AGENT_ID,
    name: 'Mail triage',
    description: 'Reads new mail, drafts replies and never acts on instructions found inside mail.',
    model_config: opts.modelConfig,
    system_instructions: opts.texts.instructions,
    authorized_tools: tools,
    listen_channels: [],
    delegation_scope: { max_monthly_budget_usd: 5, max_concurrent_runs: 1, allowed_workspace_paths: [], require_human_approval_level: 'High' },
  };
}

export function buildTriageJob(opts: TriageOptions): Partial<CronJobItem> {
  return {
    id: TRIAGE_JOB_ID,
    agent_id: TRIAGE_AGENT_ID,
    name: 'Mail triage',
    cron_expr: cronExprFor(opts.everyMinutes),
    prompt: opts.allowAutoSend ? `${opts.texts.prompt} ${opts.texts.autoSendClause}` : opts.texts.prompt,
    enabled: true,
  };
}

/** True when a tool list contains something the triage agent must not have. */
export function hasForbiddenTool(tools: readonly string[], allowAutoSend: boolean): boolean {
  if (tools.some((t) => FORBIDDEN_FOR_TRIAGE.includes(t))) return true;
  return !allowAutoSend && tools.some((t) => EXTERNAL_EFFECT_TOOLS.includes(t));
}

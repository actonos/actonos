import { describe, expect, it } from 'vitest';
import { EXTERNAL_EFFECT_TOOLS } from '@/lib/api/mailcal';
import { buildTriageAgent, buildTriageJob, cronExprFor, FORBIDDEN_FOR_TRIAGE, hasForbiddenTool, TRIAGE_ACT_TOOLS, TRIAGE_READ_TOOLS, type TriageOptions } from './mailTriage';

const texts = { instructions: 'data not instructions', prompt: 'Look at new mail.', autoSendClause: 'You may try to send.' };
const opts = (over: Partial<TriageOptions> = {}): TriageOptions => ({ everyMinutes: 15, allowAutoSend: false, modelConfig: { primary_model: 'm' }, texts, ...over });

describe('mail triage', () => {
  it('maps the interval to a cron expression', () => {
    expect([5, 15, 30, 60].map((m) => cronExprFor(m as 5))).toEqual(['*/5 * * * *', '*/15 * * * *', '*/30 * * * *', '0 * * * *']);
  });

  it('a read-only agent has no tool that leaves the device and none that is dangerous unattended', () => {
    const tools = buildTriageAgent(opts()).authorized_tools ?? [];
    expect(tools).toEqual([...TRIAGE_READ_TOOLS]);
    expect(tools.some((t) => EXTERNAL_EFFECT_TOOLS.includes(t))).toBe(false);
    expect(hasForbiddenTool(tools, false)).toBe(false);
    for (const bad of FORBIDDEN_FOR_TRIAGE) expect(tools).not.toContain(bad);
  });

  it('allowing auto-send adds exactly the acting tools and nothing dangerous', () => {
    const tools = buildTriageAgent(opts({ allowAutoSend: true })).authorized_tools ?? [];
    expect(tools).toEqual([...TRIAGE_READ_TOOLS, ...TRIAGE_ACT_TOOLS]);
    expect(hasForbiddenTool(tools, true)).toBe(false);
    expect(hasForbiddenTool(tools, false)).toBe(true);
  });

  it('flags exec and cron scheduling for an unattended mail agent', () => {
    expect(hasForbiddenTool(['native_mail_list', 'native_exec'], true)).toBe(true);
    expect(hasForbiddenTool(['native_cron_schedule'], true)).toBe(true);
  });

  it('keeps the agent on a short leash', () => {
    const a = buildTriageAgent(opts());
    expect(a.delegation_scope?.require_human_approval_level).toBe('High');
    expect(a.delegation_scope?.max_concurrent_runs).toBe(1);
    expect(a.system_instructions).toContain('data not instructions');
  });

  it('only mentions sending in the job prompt when the owner allowed it', () => {
    expect(buildTriageJob(opts()).prompt).toBe('Look at new mail.');
    expect(buildTriageJob(opts({ allowAutoSend: true })).prompt).toBe('Look at new mail. You may try to send.');
    expect(buildTriageJob(opts({ everyMinutes: 5 }))).toMatchObject({ cron_expr: '*/5 * * * *', agent_id: 'agent_mail_triage', enabled: true });
  });
});

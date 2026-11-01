import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import { ToastProvider } from '@/components/ui/Toast';
import type { MailcalAccount, MailcalDecision, MailcalProposal, MailcalRule } from '@/lib/api/mailcal';
import { MasterSwitch } from './MasterSwitch';
import { RulesTab } from './RulesTab';
import { ProposalsTab } from './ProposalsTab';
import { AuditTab } from './AuditTab';
import { toCSV } from './AuditExport';

const api = {
  getSettings: vi.fn(), putSettings: vi.fn(), listRules: vi.fn(), createRule: vi.fn(), setRuleEnabled: vi.fn(), deleteRule: vi.fn(),
  dryRun: vi.fn(), listProposals: vi.fn(), approveProposal: vi.fn(), rejectProposal: vi.fn(), listDecisions: vi.fn(),
};

vi.mock('@/lib/api/mailcal', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/mailcal')>('@/lib/api/mailcal');
  return { ...actual, mailcalApi: new Proxy({}, { get: (_t, name: string) => (...a: unknown[]) => (api as Record<string, (...x: unknown[]) => unknown>)[name](...a) }) };
});

const account: MailcalAccount = {
  id: 'acct_1', label: 'Work', plugin_id: 'refmem', kind: 'both', email: 'me@example.com', auth: { type: 'none' }, status: 'ok',
  created_at: '2026-03-01T00:00:00Z', updated_at: '2026-03-01T00:00:00Z',
};

function rule(partial: Partial<MailcalRule> = {}): MailcalRule {
  return {
    id: 'rule_1', enabled: true, account_id: 'acct_1', action: 'reply', effect: 'allow',
    match: { recipients: { addresses: ['alice@partner.com'] }, reply_only: true, body: { mode: 'template', template: 'Received.' } },
    limits: { per_hour: 3, per_day: 10 }, created_by: 'owner', created_at: '2026-03-01T00:00:00Z', ...partial,
  };
}

function wrap(node: React.ReactNode) {
  return render(
    <I18nextProvider i18n={i18n}>
      <ToastProvider>{node}</ToastProvider>
    </I18nextProvider>,
  );
}

beforeEach(async () => {
  await i18n.changeLanguage('en');
  Object.values(api).forEach((m) => m.mockReset());
});
afterEach(cleanup);

describe('MasterSwitch', () => {
  it('warns while off and turns on with one click', async () => {
    const user = userEvent.setup();
    api.getSettings.mockResolvedValue({ enabled: false });
    api.putSettings.mockResolvedValue({ enabled: true });
    wrap(<MasterSwitch />);
    expect(await screen.findByText(/every send, delete and calendar change is refused/i)).toBeInTheDocument();
    await user.click(screen.getByRole('switch'));
    await waitFor(() => expect(api.putSettings).toHaveBeenCalledWith(true));
    expect(await screen.findByText(/only when a rule you approved allows it/i)).toBeInTheDocument();
  });

  it('keeps showing the real state when the change fails', async () => {
    const user = userEvent.setup();
    api.getSettings.mockResolvedValue({ enabled: false });
    api.putSettings.mockRejectedValue(new Error('nope'));
    wrap(<MasterSwitch />);
    await user.click(await screen.findByRole('switch'));
    await waitFor(() => expect(screen.getByRole('switch')).not.toBeChecked());
  });
});

describe('RulesTab', () => {
  it('lists rules and pauses one', async () => {
    const user = userEvent.setup();
    api.listRules.mockResolvedValue({ rules: [rule(), rule({ id: 'rule_2', enabled: false, effect: 'deny', paused_reason: 'automatic pause: several returned messages in one hour' })], count: 2 });
    api.setRuleEnabled.mockResolvedValue({ enabled: false });
    wrap(<RulesTab accounts={[account]} />);
    expect((await screen.findAllByText('alice@partner.com', { exact: false })).length).toBeGreaterThan(0);
    expect(screen.getByText(/several returned messages/)).toBeInTheDocument();
    await user.click(screen.getAllByRole('button', { name: 'Pause' })[0]);
    await waitFor(() => expect(api.setRuleEnabled).toHaveBeenCalledWith('rule_1', false));
  });

  it('explains the safe default when there are no rules', async () => {
    api.listRules.mockResolvedValue({ rules: [], count: 0 });
    wrap(<RulesTab accounts={[account]} />);
    expect(await screen.findByText('No rules yet')).toBeInTheDocument();
  });

  it('creates a template rule with exact recipients and limits', async () => {
    const user = userEvent.setup();
    api.listRules.mockResolvedValue({ rules: [], count: 0 });
    api.createRule.mockResolvedValue(rule());
    wrap(<RulesTab accounts={[account]} />);
    await user.click(await screen.findByRole('button', { name: 'Add rule' }));
    await user.type(await screen.findByLabelText('Exact addresses (one per line)'), 'alice@partner.com');
    await user.type(screen.getByLabelText('The only text this rule may send'), 'Received, thank you.');
    await user.click(screen.getByRole('button', { name: 'Save rule' }));
    await waitFor(() => expect(api.createRule).toHaveBeenCalledTimes(1));
    expect(api.createRule.mock.calls[0][0]).toMatchObject({
      account_id: 'acct_1', action: 'reply', effect: 'allow', enabled: true,
      match: { recipients: { addresses: ['alice@partner.com'], domains: [] }, reply_only: true, body: { mode: 'template', template: 'Received, thank you.', allow_links: false } },
      limits: { per_hour: 3, per_day: 10 },
    });
  });

  it('will not save without a recipient, without text, or with free text unconfirmed', async () => {
    const user = userEvent.setup();
    api.listRules.mockResolvedValue({ rules: [], count: 0 });
    wrap(<RulesTab accounts={[account]} />);
    await user.click(await screen.findByRole('button', { name: 'Add rule' }));
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Save rule' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Write the fixed text.');
    await user.type(within(dialog).getByLabelText('The only text this rule may send'), 'Hi');
    await user.click(within(dialog).getByRole('button', { name: 'Save rule' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Add at least one address or domain.');
    await user.selectOptions(within(dialog).getByLabelText('Text'), 'free');
    await user.type(within(dialog).getByLabelText('Exact addresses (one per line)'), 'a@b.com');
    await user.click(within(dialog).getByRole('button', { name: 'Save rule' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Confirm that you understand');
    expect(api.createRule).not.toHaveBeenCalled();
  });

  it('shows the server refusal of a dangerous rule instead of hiding it', async () => {
    const user = userEvent.setup();
    api.listRules.mockResolvedValue({ rules: [], count: 0 });
    api.createRule.mockRejectedValue(new Error('gmail.com is a public mailbox provider'));
    wrap(<RulesTab accounts={[account]} />);
    await user.click(await screen.findByRole('button', { name: 'Add rule' }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Exact domains (one per line)'), 'gmail.com');
    await user.type(within(dialog).getByLabelText('The only text this rule may send'), 'Hi');
    await user.click(within(dialog).getByRole('button', { name: 'Save rule' }));
    expect(await within(dialog).findByText(/public mailbox provider/)).toBeInTheDocument();
  });

  it('tries a rule against a draft without saving it', async () => {
    const user = userEvent.setup();
    api.listRules.mockResolvedValue({ rules: [], count: 0 });
    api.dryRun.mockResolvedValue({ verdict: 'needs_approval', reasons: ['the text differs from the approved template'] });
    wrap(<RulesTab accounts={[account]} />);
    await user.click(await screen.findByRole('button', { name: 'Add rule' }));
    const dialog = await screen.findByRole('dialog');
    await user.type(within(dialog).getByLabelText('Exact addresses (one per line)'), 'alice@partner.com');
    await user.type(within(dialog).getByLabelText('The only text this rule may send'), 'Hi');
    await user.type(within(dialog).getByLabelText('Draft id'), 'draft_1');
    await user.click(within(dialog).getByRole('button', { name: 'Check' }));
    expect(await within(dialog).findByText('the text differs from the approved template')).toBeInTheDocument();
    expect(api.dryRun).toHaveBeenCalledWith(expect.objectContaining({ action: 'reply' }), 'draft_1');
    expect(api.createRule).not.toHaveBeenCalled();
  });
});

describe('ProposalsTab', () => {
  const proposal = (partial: Partial<MailcalProposal> = {}): MailcalProposal => ({
    id: 'prop_1', source: 'agent', rule: rule(), rationale: 'auto-ack invoices', status: 'pending', created_at: '2026-03-01T00:00:00Z', ...partial,
  });

  it('labels who suggested it and creates the rule only on approval', async () => {
    const user = userEvent.setup();
    api.listProposals.mockResolvedValue({ proposals: [proposal()], count: 1 });
    api.approveProposal.mockResolvedValue(rule());
    wrap(<ProposalsTab />);
    expect(await screen.findByText('Suggested by an agent')).toBeInTheDocument();
    expect(screen.getByText('auto-ack invoices')).toBeInTheDocument();
    expect(api.approveProposal).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Create rule' }));
    await waitFor(() => expect(api.approveProposal).toHaveBeenCalledWith('prop_1'));
  });

  it('rejects a proposal', async () => {
    const user = userEvent.setup();
    api.listProposals.mockResolvedValue({ proposals: [proposal({ source: 'user' })], count: 1 });
    api.rejectProposal.mockResolvedValue({ status: 'rejected' });
    wrap(<ProposalsTab />);
    await user.click(await screen.findByRole('button', { name: 'Reject' }));
    await waitFor(() => expect(api.rejectProposal).toHaveBeenCalledWith('prop_1'));
  });
});

describe('AuditTab and CSV export', () => {
  const row = (partial: Partial<MailcalDecision> = {}): MailcalDecision => ({
    id: 'dec_1', at: '2026-03-02T10:00:00Z', phase: 'gate', account_id: 'acct_1', agent_id: 'a', tool: 'native_mail_send',
    verdict: 'needs_approval', reasons: ['no rule allows this action'], ...partial,
  });

  it('shows decisions and filters by result', async () => {
    const user = userEvent.setup();
    api.listDecisions.mockResolvedValue({ decisions: [row()], count: 1 });
    wrap(<AuditTab />);
    expect(await screen.findByText('native_mail_send')).toBeInTheDocument();
    expect(screen.getByText('no rule allows this action')).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText('Result'), 'deny');
    await waitFor(() => expect(api.listDecisions).toHaveBeenLastCalledWith(expect.objectContaining({ verdict: 'deny' })));
  });

  it('neutralizes spreadsheet formulas and carries no message text', () => {
    const csv = toCSV([row({ outcome: "=cmd|' /C calc'!A0", rule_id: '+1', draft_id: '@SUM(1)', payload_hash: '\tx' }), row({ id: 'd2', outcome: 'say "hi", ok' })]);
    const lines = csv.split('\r\n');
    expect(lines[0]).toBe('at,phase,account_id,tool,verdict,rule_id,approved,outcome,draft_id,payload_hash');
    expect(lines[1]).toContain(`"'=cmd|' /C calc'!A0"`);
    expect(lines[1]).toContain(`"'+1"`);
    expect(lines[1]).toContain(`"'@SUM(1)"`);
    expect(lines[1]).toContain(`"'\tx"`);
    expect(lines[2]).toContain('"say ""hi"", ok"');
    expect(csv).not.toMatch(/subject|body|reasons/i);
  });
});

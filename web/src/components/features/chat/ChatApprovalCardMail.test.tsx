import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import type { ApprovalRequest } from '@/lib/types';
import { ToastProvider } from '@/components/ui/Toast';
import { ChatApprovalCard } from './ChatApprovalCard';

const approveAction = vi.fn();
const rejectAction = vi.fn();
const getDraft = vi.fn();
const evaluateDraft = vi.fn();
const proposeFromDraft = vi.fn();

vi.mock('@/lib/api', () => ({ api: { approveAction: (...a: unknown[]) => approveAction(...a), rejectAction: (...a: unknown[]) => rejectAction(...a) } }));
vi.mock('@/lib/api/mailcal', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/mailcal')>('@/lib/api/mailcal');
  return {
    ...actual,
    mailcalApi: {
      getDraft: (...a: unknown[]) => getDraft(...a),
      evaluateDraft: (...a: unknown[]) => evaluateDraft(...a),
      proposeFromDraft: (...a: unknown[]) => proposeFromDraft(...a),
    },
  };
});

const approval = (tool: string, input: Record<string, unknown>): ApprovalRequest => ({
  id: 'apr_mail', trace_id: 't', agent_id: 'agent_system_core', tool_name: tool, risk_level: 'High', action_hash: 'h', input,
  status: 'pending', requested_at: new Date().toISOString(), expires_at: new Date().toISOString(), source: 'stream',
});

const wrap = (a: ApprovalRequest) =>
  render(<I18nextProvider i18n={i18n}><ToastProvider><ChatApprovalCard approval={a} /></ToastProvider></I18nextProvider>);

beforeEach(async () => {
  await i18n.changeLanguage('en');
  [approveAction, rejectAction, getDraft, evaluateDraft, proposeFromDraft].forEach((m) => m.mockReset());
  approveAction.mockResolvedValue({ status: 'continued' });
  getDraft.mockResolvedValue({ id: 'd1', account_id: 'a', created_by: 'agent', payload_hash: 'h', payload: { to: [{ email: 'alice@partner.com' }], subject: 'Re: Q3', body_text: 'Received.' } });
  evaluateDraft.mockResolvedValue({ verdict: 'needs_approval', reasons: ['no rule allows this action'] });
});
afterEach(cleanup);

describe('ChatApprovalCard for a mail send', () => {
  it('lets the person read exactly what will be sent before approving', async () => {
    const user = userEvent.setup();
    wrap(approval('native_mail_send', { draft_id: 'd1' }));
    expect(getDraft).not.toHaveBeenCalled();
    await user.click(screen.getByRole('button', { name: 'Show message' }));
    expect(await screen.findByText('Received.')).toBeInTheDocument();
    expect(getDraft).toHaveBeenCalledWith('d1');
    await user.click(screen.getByRole('button', { name: 'Hide message' }));
    expect(screen.queryByText('Received.')).not.toBeInTheDocument();
  });

  it('approves and then proposes a rule, which only creates a proposal', async () => {
    const user = userEvent.setup();
    proposeFromDraft.mockResolvedValue({ id: 'prop_1' });
    wrap(approval('native_mail_send', { draft_id: 'd1' }));
    await user.click(screen.getByRole('button', { name: 'Approve and propose a rule' }));
    await waitFor(() => expect(approveAction).toHaveBeenCalledTimes(1));
    await waitFor(() => expect(proposeFromDraft).toHaveBeenCalledWith('d1'));
    expect(await screen.findByText('Rule proposed')).toBeInTheDocument();
  });

  it('still counts the message as approved when proposing the rule fails', async () => {
    const user = userEvent.setup();
    proposeFromDraft.mockRejectedValue(new Error('draft has Bcc recipients'));
    wrap(approval('native_mail_send', { draft_id: 'd1' }));
    await user.click(screen.getByRole('button', { name: 'Approve and propose a rule' }));
    expect(await screen.findByText('The message was approved, but the rule could not be proposed')).toBeInTheDocument();
    expect(approveAction).toHaveBeenCalledTimes(1);
  });

  it('plain approve never proposes a rule', async () => {
    const user = userEvent.setup();
    wrap(approval('native_mail_send', { draft_id: 'd1' }));
    await user.click(screen.getByRole('button', { name: 'Approve' }));
    await waitFor(() => expect(approveAction).toHaveBeenCalled());
    expect(proposeFromDraft).not.toHaveBeenCalled();
  });

  it('shows no mail controls for other tools', () => {
    wrap(approval('native_file_write', { path: 'x' }));
    expect(screen.queryByRole('button', { name: 'Show message' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Approve and propose a rule' })).not.toBeInTheDocument();
  });
});

import { cleanup, render, screen, waitFor } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import { MailApprovalPreview, mailDraftIdOf } from './MailApprovalPreview';

const getDraft = vi.fn();
const evaluateDraft = vi.fn();

vi.mock('@/lib/api/mailcal', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/mailcal')>('@/lib/api/mailcal');
  return { ...actual, mailcalApi: { getDraft: (...a: unknown[]) => getDraft(...a), evaluateDraft: (...a: unknown[]) => evaluateDraft(...a) } };
});

const wrap = (id: string) => render(<I18nextProvider i18n={i18n}><MailApprovalPreview draftId={id} /></I18nextProvider>);

beforeEach(async () => {
  await i18n.changeLanguage('en');
  getDraft.mockReset();
  evaluateDraft.mockReset();
});
afterEach(cleanup);

describe('MailApprovalPreview', () => {
  it('shows exactly what will be sent and why no rule covered it', async () => {
    getDraft.mockResolvedValue({ id: 'd1', account_id: 'a', created_by: 'agent', payload_hash: 'h', reply: { verified: true, participants: ['alice@partner.com'] },
      payload: { to: [{ email: 'alice@partner.com' }], subject: 'Re: Q3', body_text: 'Received, thank you.' } });
    evaluateDraft.mockResolvedValue({ verdict: 'needs_approval', reasons: ['no rule allows this action'] });
    wrap('d1');
    expect(await screen.findByText('alice@partner.com')).toBeInTheDocument();
    expect(screen.getByText('Re: Q3')).toBeInTheDocument();
    expect(screen.getByText('Received, thank you.')).toBeInTheDocument();
    expect(screen.getByText('no rule allows this action')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('warns loudly about a recipient who is not part of the conversation', async () => {
    getDraft.mockResolvedValue({ id: 'd2', account_id: 'a', created_by: 'agent', payload_hash: 'h', reply: { verified: true, participants: ['alice@partner.com'] },
      payload: { to: [{ email: 'alice@partner.com' }, { email: 'mallory@evil.test' }], subject: 'x', body_text: 'y' } });
    evaluateDraft.mockResolvedValue({ verdict: 'needs_approval', reasons: [] });
    wrap('d2');
    expect(await screen.findByRole('alert')).toHaveTextContent('has not written in this conversation');
  });

  it('warns when the message is not a verified reply at all', async () => {
    getDraft.mockResolvedValue({ id: 'd3', account_id: 'a', created_by: 'agent', payload_hash: 'h', payload: { to: [{ email: 'x@y.com' }], subject: 's', body_text: 'b' } });
    evaluateDraft.mockResolvedValue({ verdict: 'needs_approval', reasons: [] });
    wrap('d3');
    expect(await screen.findByRole('alert')).toHaveTextContent('not a verified reply');
  });

  it('renders message text as plain text, never as HTML', async () => {
    getDraft.mockResolvedValue({ id: 'd4', account_id: 'a', created_by: 'agent', payload_hash: 'h',
      payload: { to: [{ email: 'x@y.com' }], subject: '<img src=x onerror=alert(1)>', body_text: '<script>alert(1)</script>' } });
    evaluateDraft.mockResolvedValue({ verdict: 'needs_approval', reasons: [] });
    const { container } = wrap('d4');
    await screen.findByText('<script>alert(1)</script>');
    expect(container.querySelector('script, img')).toBeNull();
  });

  it('tells the person not to approve when the message cannot be loaded', async () => {
    getDraft.mockRejectedValue(new Error('gone'));
    evaluateDraft.mockRejectedValue(new Error('gone'));
    wrap('d5');
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('Do not approve it.'));
  });
});

describe('mailDraftIdOf', () => {
  it('only recognises a send approval with a string draft id', () => {
    expect(mailDraftIdOf({ tool_name: 'native_mail_send', input: { draft_id: 'd1' } })).toBe('d1');
    expect(mailDraftIdOf({ tool_name: 'native_mail_send', input: { draft_id: 5 } })).toBe('');
    expect(mailDraftIdOf({ tool_name: 'native_file_write', input: { draft_id: 'd1' } })).toBe('');
  });
});

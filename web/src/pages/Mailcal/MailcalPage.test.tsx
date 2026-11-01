import { cleanup, render, screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import { ToastProvider } from '@/components/ui/Toast';
import type { MailcalAccount, MailcalProvider } from '@/lib/api/mailcal';
import { MailcalPage } from './MailcalPage';

const listAccounts = vi.fn();
const listProviders = vi.fn();
const createAccount = vi.fn();
const testAccount = vi.fn();
const deleteAccount = vi.fn();

vi.mock('@/lib/api/mailcal', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/mailcal')>('@/lib/api/mailcal');
  return {
    ...actual,
    mailcalApi: {
      listAccounts: (...a: unknown[]) => listAccounts(...a),
      listProviders: (...a: unknown[]) => listProviders(...a),
      createAccount: (...a: unknown[]) => createAccount(...a),
      testAccount: (...a: unknown[]) => testAccount(...a),
      deleteAccount: (...a: unknown[]) => deleteAccount(...a),
      updateAccount: vi.fn(),
      getSettings: () => Promise.resolve({ enabled: false }),
      listProposals: () => Promise.resolve({ proposals: [], count: 0 }),
      putPassword: vi.fn(),
    },
  };
});

const provider: MailcalProvider = {
  plugin_id: 'refmem', name: 'Reference', version: '1.0.0', abi: ['mail@1', 'calendar@1'], ops: [], limits: {},
};

function account(partial: Partial<MailcalAccount> = {}): MailcalAccount {
  return {
    id: 'acct_1', label: 'Work', plugin_id: 'refmem', kind: 'both', email: 'me@example.com',
    auth: { type: 'none' }, status: 'ok', created_at: '2026-03-01T00:00:00Z', updated_at: '2026-03-01T00:00:00Z',
    ...partial,
  };
}

function renderPage() {
  return render(
    <I18nextProvider i18n={i18n}>
      <ToastProvider>
        <MailcalPage />
      </ToastProvider>
    </I18nextProvider>,
  );
}

describe('MailcalPage', () => {
  beforeEach(async () => {
    await i18n.changeLanguage('en');
    [listAccounts, listProviders, createAccount, testAccount, deleteAccount].forEach((m) => m.mockReset());
    listProviders.mockResolvedValue({ providers: [provider] });
    listAccounts.mockResolvedValue({ accounts: [account()], count: 1 });
  });
  afterEach(cleanup);

  it('lists accounts with their status', async () => {
    listAccounts.mockResolvedValue({ accounts: [account(), account({ id: 'acct_2', label: 'Old', status: 'auth_expired', status_detail: 'token revoked' })], count: 2 });
    renderPage();
    expect(await screen.findByText('Work')).toBeInTheDocument();
    expect(screen.getByText('Connected')).toBeInTheDocument();
    expect(screen.getByText('Sign-in expired')).toBeInTheDocument();
    expect(screen.getByText('token revoked')).toBeInTheDocument();
  });

  it('tells the user to install a provider when none exists', async () => {
    listProviders.mockResolvedValue({ providers: [] });
    listAccounts.mockResolvedValue({ accounts: [], count: 0 });
    renderPage();
    expect(await screen.findByText('No provider plugin installed')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Add account' })).toBeDisabled();
  });

  it('adds an account through the dialog', async () => {
    const user = userEvent.setup();
    listAccounts.mockResolvedValueOnce({ accounts: [], count: 0 });
    createAccount.mockResolvedValue(account());
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Add account' }));
    await user.type(await screen.findByLabelText('Name'), 'Work');
    await user.type(screen.getByLabelText('Your address (optional)'), 'me@example.com');
    await user.click(screen.getByRole('button', { name: 'Add' }));
    await waitFor(() =>
      expect(createAccount).toHaveBeenCalledWith({ label: 'Work', plugin_id: 'refmem', kind: 'both', email: 'me@example.com' }),
    );
  });

  it('does not submit without a name', async () => {
    const user = userEvent.setup();
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Add account' }));
    expect(await screen.findByRole('button', { name: 'Add' })).toBeDisabled();
  });

  it('tests an account and reloads', async () => {
    const user = userEvent.setup();
    testAccount.mockResolvedValue({ ok: true, checks: ['mail: listed the inbox'] });
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Test' }));
    await waitFor(() => expect(testAccount).toHaveBeenCalledWith('acct_1'));
    await waitFor(() => expect(listAccounts).toHaveBeenCalledTimes(2));
  });

  it('asks before deleting', async () => {
    const user = userEvent.setup();
    deleteAccount.mockResolvedValue({ status: 'deleted' });
    renderPage();
    await user.click(await screen.findByRole('button', { name: 'Delete' }));
    expect(deleteAccount).not.toHaveBeenCalled();
    const dialog = await screen.findByRole('dialog');
    await user.click(within(dialog).getByRole('button', { name: 'Delete' }));
    await waitFor(() => expect(deleteAccount).toHaveBeenCalledWith('acct_1'));
  });

  it('shows a retryable error when loading fails', async () => {
    listAccounts.mockRejectedValue(new Error('boom'));
    renderPage();
    expect(await screen.findByText('Could not load accounts')).toBeInTheDocument();
  });
});

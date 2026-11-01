import { cleanup, render, screen, waitFor } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { I18nextProvider } from 'react-i18next';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import { ToastProvider } from '@/components/ui/Toast';
import type { MailcalAccount, MailcalProvider } from '@/lib/api/mailcal';
import { AccountModal } from './AccountModal';
import { ConnectButton } from './ConnectButton';
import { OAuthClientPanel } from './OAuthClientPanel';
import { ReconnectBanner } from './ReconnectBanner';
import { MailcalPage } from './MailcalPage';

const api = { getOAuthClient: vi.fn(), putOAuthClient: vi.fn(), startOAuth: vi.fn(), putPassword: vi.fn(), listAccounts: vi.fn(), listProviders: vi.fn(), createAccount: vi.fn(), getSettings: vi.fn(), listProposals: vi.fn() };

vi.mock('@/lib/api/mailcal', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/mailcal')>('@/lib/api/mailcal');
  return { ...actual, mailcalApi: new Proxy({}, { get: (_t, name: string) => (...a: unknown[]) => (api as Record<string, (...x: unknown[]) => unknown>)[name](...a) }) };
});

const google: MailcalProvider = { plugin_id: 'mailcal-gmail', name: 'Gmail', version: '1', abi: ['mail@1', 'calendar@1'], ops: [], limits: {}, oauth_provider: 'google', auth_kind: 'oauth' };
const caldav: MailcalProvider = { plugin_id: 'mailcal-caldav', name: 'CalDAV', version: '1', abi: ['calendar@1'], ops: [], limits: {}, auth_kind: 'password' };
const imap: MailcalProvider = { plugin_id: 'mailcal-imap-smtp', name: 'IMAP', version: '1', abi: ['mail@1'], ops: [], limits: {}, auth_kind: 'password' };

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

describe('OAuthClientPanel', () => {
  it('stores the client and never shows the secret again', async () => {
    const user = userEvent.setup();
    api.getOAuthClient.mockResolvedValue({ configured: false });
    api.putOAuthClient.mockResolvedValue({ configured: true });
    wrap(<OAuthClientPanel provider="google" />);
    await user.type(await screen.findByLabelText('Client id'), 'cid');
    await user.type(screen.getByLabelText('Client secret'), 'csecret');
    await user.click(screen.getByRole('button', { name: 'Save client' }));
    await waitFor(() => expect(api.putOAuthClient).toHaveBeenCalledWith('google', 'cid', 'csecret'));
    expect(await screen.findByText('Configured')).toBeInTheDocument();
    expect(screen.queryByLabelText('Client secret')).not.toBeInTheDocument();
    expect(screen.queryByDisplayValue('csecret')).not.toBeInTheDocument();
  });

  it('shows the redirect address to register and the right help text', async () => {
    api.getOAuthClient.mockResolvedValue({ configured: true });
    wrap(<OAuthClientPanel provider="microsoft" />);
    expect(await screen.findByText(/Microsoft Entra/)).toBeInTheDocument();
    expect(screen.getByText(`${window.location.origin}/api/mailcal/oauth/callback`)).toBeInTheDocument();
  });

  it('surfaces a refused save', async () => {
    const user = userEvent.setup();
    api.getOAuthClient.mockResolvedValue({ configured: false });
    api.putOAuthClient.mockRejectedValue(new Error('invalid provider name'));
    wrap(<OAuthClientPanel provider="google" />);
    await user.type(await screen.findByLabelText('Client id'), 'cid');
    await user.click(screen.getByRole('button', { name: 'Save client' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('invalid provider name');
  });
});

describe('ConnectButton', () => {
  it('navigates this tab to the provider', async () => {
    const user = userEvent.setup();
    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, assign, origin: window.location.origin });
    api.startOAuth.mockResolvedValue({ auth_url: 'https://accounts.example/auth?state=s', redirect_uri: 'x' });
    wrap(<ConnectButton accountId="acct_1" />);
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    await waitFor(() => expect(assign).toHaveBeenCalledWith('https://accounts.example/auth?state=s'));
    vi.unstubAllGlobals();
  });

  it('does not navigate when the sign-in cannot start', async () => {
    const user = userEvent.setup();
    const assign = vi.fn();
    vi.stubGlobal('location', { ...window.location, assign });
    api.startOAuth.mockRejectedValue(new Error('oauth client credentials are not configured'));
    wrap(<ConnectButton accountId="acct_1" />);
    await user.click(screen.getByRole('button', { name: 'Sign in' }));
    await waitFor(() => expect(api.startOAuth).toHaveBeenCalled());
    expect(assign).not.toHaveBeenCalled();
    vi.unstubAllGlobals();
  });
});

describe('ReconnectBanner', () => {
  const acct = (partial: Partial<MailcalAccount>): MailcalAccount => ({
    id: 'acct_1', label: 'W', plugin_id: 'p', kind: 'both', auth: { type: 'oauth' }, status: 'auth_expired', status_detail: 'the sign-in was revoked',
    created_at: '', updated_at: '', ...partial,
  });

  it('is silent for a healthy account', () => {
    wrap(<ReconnectBanner account={acct({ status: 'ok' })} onDone={vi.fn()} />);
    expect(screen.queryByRole('alert')).not.toBeInTheDocument();
  });

  it('asks an OAuth account to sign in again', () => {
    wrap(<ReconnectBanner account={acct({})} onDone={vi.fn()} />);
    expect(screen.getByRole('alert')).toHaveTextContent('the sign-in was revoked');
    expect(screen.getByRole('button', { name: 'Sign in' })).toBeInTheDocument();
  });

  it('takes a new password for a password account', async () => {
    const user = userEvent.setup();
    const done = vi.fn();
    api.putPassword.mockResolvedValue({ configured: true });
    wrap(<ReconnectBanner account={acct({ auth: { type: 'password' } })} onDone={done} />);
    await user.type(screen.getByLabelText('New password'), 'fresh');
    await user.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.putPassword).toHaveBeenCalledWith('acct_1', 'fresh'));
    expect(done).toHaveBeenCalled();
  });
});

describe('AccountModal sign-in forms', () => {
  const open = (providers: MailcalProvider[], onSubmit = vi.fn()) => {
    api.getOAuthClient.mockResolvedValue({ configured: true });
    wrap(<AccountModal isOpen providers={providers} saving={false} onClose={vi.fn()} onSubmit={onSubmit} />);
    return onSubmit;
  };

  it('creates an OAuth account without any secret', async () => {
    const user = userEvent.setup();
    const onSubmit = open([google]);
    await user.type(await screen.findByLabelText('Name'), 'Personal');
    await user.click(screen.getByRole('button', { name: 'Add' }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(onSubmit.mock.calls[0][0]).toEqual({ label: 'Personal', plugin_id: 'mailcal-gmail', kind: 'both', email: undefined, auth: { type: 'oauth' } });
    expect(onSubmit.mock.calls[0][1]).toBeUndefined();
  });

  it('keeps the CalDAV password out of the config and demands https', async () => {
    const user = userEvent.setup();
    const onSubmit = open([caldav]);
    await user.type(await screen.findByLabelText('Name'), 'Fastmail');
    await user.type(screen.getByLabelText('App password'), 'app-pw');
    await user.type(screen.getByLabelText('Calendar address (https)'), 'http://insecure.example/cal/');
    await user.click(screen.getByRole('button', { name: 'Add' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('must start with https://');
    expect(onSubmit).not.toHaveBeenCalled();
    await user.clear(screen.getByLabelText('Calendar address (https)'));
    await user.type(screen.getByLabelText('Calendar address (https)'), 'https://caldav.fastmail.com/dav/calendars/me/');
    await user.type(screen.getByLabelText('Your address (optional)'), 'me@fastmail.example');
    await user.click(screen.getByRole('button', { name: 'Add' }));
    expect(onSubmit).toHaveBeenCalledTimes(1);
    const [input, password] = onSubmit.mock.calls[0];
    expect(password).toBe('app-pw');
    expect(JSON.stringify(input)).not.toContain('app-pw');
    expect(input.config).toMatchObject({ calendar_url: 'https://caldav.fastmail.com/dav/calendars/me/', email: 'me@fastmail.example' });
    expect(input.auth).toEqual({ type: 'password' });
  });

  it('builds the IMAP and SMTP config and tells the user which net_tcp lines to add', async () => {
    const user = userEvent.setup();
    const onSubmit = open([imap]);
    await user.type(await screen.findByLabelText('Name'), 'Mail');
    await user.type(screen.getByLabelText('Your address (optional)'), 'me@example.com');
    await user.type(screen.getByLabelText('IMAP server'), 'imap.example.com');
    await user.type(screen.getByLabelText('SMTP server'), 'smtp.example.com');
    await user.type(screen.getByLabelText('App password'), 'pw');
    expect(screen.getByText(/imap\.example\.com:993/)).toBeInTheDocument();
    expect(screen.getByText(/smtp\.example\.com:587/)).toBeInTheDocument();
    await user.click(screen.getByRole('button', { name: 'Add' }));
    const [input, password] = onSubmit.mock.calls[0];
    expect(password).toBe('pw');
    expect(input.config).toMatchObject({ imap_host: 'imap.example.com', imap_port: 993, imap_mode: 'tls', smtp_host: 'smtp.example.com', smtp_port: 587, smtp_mode: 'starttls', email: 'me@example.com', sent_mailbox: 'Sent' });
  });

  it('rejects a bad port before submitting', async () => {
    const user = userEvent.setup();
    const onSubmit = open([imap]);
    await user.type(await screen.findByLabelText('Name'), 'Mail');
    await user.type(screen.getByLabelText('Your address (optional)'), 'me@example.com');
    await user.type(screen.getByLabelText('IMAP server'), 'a');
    await user.type(screen.getByLabelText('SMTP server'), 'b');
    await user.type(screen.getByLabelText('App password'), 'pw');
    await user.clear(screen.getByLabelText('IMAP port'));
    await user.type(screen.getByLabelText('IMAP port'), '99999');
    await user.click(screen.getByRole('button', { name: 'Add' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('Ports must be numbers');
    expect(onSubmit).not.toHaveBeenCalled();
  });
});

describe('MailcalPage after a provider sign-in', () => {
  it('reports the result carried in the URL hash and clears it', async () => {
    window.location.hash = '#/mailcal?oauth=no_refresh_token';
    api.listAccounts.mockResolvedValue({ accounts: [], count: 0 });
    api.listProviders.mockResolvedValue({ providers: [google] });
    api.getSettings.mockResolvedValue({ enabled: false });
    api.listProposals.mockResolvedValue({ proposals: [], count: 0 });
    wrap(<MailcalPage />);
    expect(await screen.findByText(/did not allow lasting access/)).toBeInTheDocument();
    expect(window.location.hash).toBe('#/mailcal');
  });

  it('treats an unknown result code as a failure rather than echoing it', async () => {
    window.location.hash = '#/mailcal?oauth=%3Cscript%3E';
    api.listAccounts.mockResolvedValue({ accounts: [], count: 0 });
    api.listProviders.mockResolvedValue({ providers: [google] });
    api.getSettings.mockResolvedValue({ enabled: false });
    api.listProposals.mockResolvedValue({ proposals: [], count: 0 });
    wrap(<MailcalPage />);
    expect(await screen.findByText(/The sign-in failed/)).toBeInTheDocument();
    expect(screen.queryByText(/script/)).not.toBeInTheDocument();
  });
});

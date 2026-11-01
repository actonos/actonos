import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import type { MailcalAccountInput, MailcalKind, MailcalProvider } from '@/lib/api/mailcal';
import { OAuthClientPanel } from './OAuthClientPanel';

interface AccountModalProps {
  isOpen: boolean;
  providers: MailcalProvider[];
  saving: boolean;
  onClose: () => void;
  onSubmit: (input: MailcalAccountInput, password?: string) => void | Promise<void>;
}

function kindsFor(provider: MailcalProvider | undefined): MailcalKind[] {
  if (!provider) return [];
  const mail = provider.abi.includes('mail@1');
  const calendar = provider.abi.includes('calendar@1');
  if (mail && calendar) return ['both', 'mail', 'calendar'];
  if (mail) return ['mail'];
  if (calendar) return ['calendar'];
  return [];
}

export function AccountModal({ isOpen, providers, saving, onClose, onSubmit }: AccountModalProps) {
  const { t } = useTranslation('mailcal');
  const [pluginId, setPluginId] = useState('');
  const [kind, setKind] = useState<MailcalKind>('both');
  const [label, setLabel] = useState('');
  const [email, setEmail] = useState('');
  const [username, setUsername] = useState('');
  const [password, setPassword] = useState('');
  const [calendarUrl, setCalendarUrl] = useState('');
  const [imapHost, setImapHost] = useState('');
  const [imapPort, setImapPort] = useState('993');
  const [imapMode, setImapMode] = useState<'tls' | 'starttls'>('tls');
  const [smtpHost, setSmtpHost] = useState('');
  const [smtpPort, setSmtpPort] = useState('587');
  const [smtpMode, setSmtpMode] = useState<'tls' | 'starttls'>('starttls');
  const [sentMailbox, setSentMailbox] = useState('Sent');
  const [trashMailbox, setTrashMailbox] = useState('Trash');
  const [formError, setFormError] = useState('');

  useEffect(() => {
    if (!isOpen) return;
    const first = providers[0];
    setPluginId(first?.plugin_id ?? '');
    setKind(kindsFor(first)[0] ?? 'both');
    setLabel('');
    setEmail('');
    setUsername('');
    setPassword('');
    setCalendarUrl('');
    setImapHost('');
    setImapPort('993');
    setImapMode('tls');
    setSmtpHost('');
    setSmtpPort('587');
    setSmtpMode('starttls');
    setSentMailbox('Sent');
    setTrashMailbox('Trash');
    setFormError('');
  }, [isOpen, providers]);

  const provider = providers.find((p) => p.plugin_id === pluginId);
  const kinds = kindsFor(provider);
  const canSubmit = Boolean(provider) && label.trim() !== '' && !saving;
  const authKind = provider?.auth_kind;
  const isMailPassword = authKind === 'password' && Boolean(provider?.abi.includes('mail@1'));
  const isCalendarPassword = authKind === 'password' && !isMailPassword;
  const netTcpLines = isMailPassword
    ? [`${imapHost.trim() || 'imap.example.com'}:${imapPort}`, `${smtpHost.trim() || 'smtp.example.com'}:${smtpPort}`]
    : [];

  const buildSubmission = (): { input: MailcalAccountInput; password?: string } | string => {
    if (!provider) return t('modal.errors.provider');
    const base: MailcalAccountInput = { label: label.trim(), plugin_id: pluginId, kind, email: email.trim() || undefined };
    if (authKind === 'oauth') return { input: { ...base, auth: { type: 'oauth' } } };
    if (authKind !== 'password') return { input: base };
    if (password === '') return t('modal.errors.password');
    const config: Record<string, unknown> = {};
    if (username.trim()) config.username = username.trim();
    if (isCalendarPassword) {
      if (!/^https:\/\//i.test(calendarUrl.trim())) return t('modal.errors.calendarUrl');
      config.calendar_url = calendarUrl.trim();
      if (email.trim()) config.email = email.trim();
    } else {
      const ports = [Number(imapPort), Number(smtpPort)];
      if (!imapHost.trim() || !smtpHost.trim() || !email.trim()) return t('modal.errors.imapRequired');
      if (ports.some((p) => !Number.isInteger(p) || p < 1 || p > 65535)) return t('modal.errors.port');
      Object.assign(config, {
        imap_host: imapHost.trim(), imap_port: Number(imapPort), imap_mode: imapMode,
        smtp_host: smtpHost.trim(), smtp_port: Number(smtpPort), smtp_mode: smtpMode,
        email: email.trim(), sent_mailbox: sentMailbox.trim() || 'Sent', trash_mailbox: trashMailbox.trim() || 'Trash',
      });
    }
    return { input: { ...base, config, auth: { type: 'password' } }, password };
  };

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('modal.title')} maxWidth="lg">
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          if (!canSubmit) return;
          const built = buildSubmission();
          if (typeof built === 'string') {
            setFormError(built);
            return;
          }
          setFormError('');
          void onSubmit(built.input, built.password);
        }}
      >
        <div className="flex flex-col gap-1.5">
          <label htmlFor="mailcal-provider" className="text-caption font-medium uppercase tracking-wider text-slate">
            {t('modal.provider')}
          </label>
          <select
            id="mailcal-provider"
            value={pluginId}
            onChange={(event) => {
              setPluginId(event.target.value);
              setKind(kindsFor(providers.find((p) => p.plugin_id === event.target.value))[0] ?? 'both');
            }}
            className="rounded-full border border-onyx/15 bg-canvas px-4 py-2.5 text-body-sm text-deep-ink"
          >
            {providers.map((p) => (
              <option key={p.plugin_id} value={p.plugin_id}>
                {p.name} ({p.version})
              </option>
            ))}
          </select>
        </div>

        <div className="flex flex-col gap-1.5">
          <label htmlFor="mailcal-kind" className="text-caption font-medium uppercase tracking-wider text-slate">
            {t('modal.kind')}
          </label>
          <select
            id="mailcal-kind"
            value={kind}
            onChange={(event) => setKind(event.target.value as MailcalKind)}
            className="rounded-full border border-onyx/15 bg-canvas px-4 py-2.5 text-body-sm text-deep-ink"
          >
            {kinds.map((k) => (
              <option key={k} value={k}>
                {t(`kind.${k}`)}
              </option>
            ))}
          </select>
        </div>

        <Input id="mailcal-label" label={t('modal.label')} value={label} onChange={(e) => setLabel(e.target.value)} placeholder={t('modal.labelPlaceholder')} />
        <Input id="mailcal-email" label={t('modal.email')} type="email" value={email} onChange={(e) => setEmail(e.target.value)} placeholder={t('modal.emailPlaceholder')} />
        {authKind === 'oauth' && provider?.oauth_provider && <OAuthClientPanel provider={provider.oauth_provider} />}
        {authKind === 'password' && (
          <>
            <Input id="mailcal-username" label={t('modal.username')} value={username} onChange={(e) => setUsername(e.target.value)} autoComplete="off" />
            {isCalendarPassword && (
              <Input id="mailcal-caldav" label={t('modal.calendarUrl')} value={calendarUrl} onChange={(e) => setCalendarUrl(e.target.value)} placeholder={t('modal.calendarUrlPlaceholder')} />
            )}
            {isMailPassword && (
              <>
                <div className="grid gap-3 sm:grid-cols-3">
                  <Input id="mailcal-imap-host" label={t('modal.imapHost')} value={imapHost} onChange={(e) => setImapHost(e.target.value)} />
                  <Input id="mailcal-imap-port" label={t('modal.imapPort')} inputMode="numeric" value={imapPort} onChange={(e) => setImapPort(e.target.value)} />
                  <div className="flex flex-col gap-1.5">
                    <label htmlFor="mailcal-imap-mode" className="text-caption font-medium uppercase tracking-wider text-slate">{t('modal.mode')}</label>
                    <select id="mailcal-imap-mode" value={imapMode} onChange={(e) => setImapMode(e.target.value as 'tls' | 'starttls')} className="rounded-full border border-onyx/15 bg-canvas px-4 py-2.5 text-body-sm text-deep-ink">
                      <option value="tls">{t('modal.tls')}</option>
                      <option value="starttls">{t('modal.starttls')}</option>
                    </select>
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-3">
                  <Input id="mailcal-smtp-host" label={t('modal.smtpHost')} value={smtpHost} onChange={(e) => setSmtpHost(e.target.value)} />
                  <Input id="mailcal-smtp-port" label={t('modal.smtpPort')} inputMode="numeric" value={smtpPort} onChange={(e) => setSmtpPort(e.target.value)} />
                  <div className="flex flex-col gap-1.5">
                    <label htmlFor="mailcal-smtp-mode" className="text-caption font-medium uppercase tracking-wider text-slate">{t('modal.mode')}</label>
                    <select id="mailcal-smtp-mode" value={smtpMode} onChange={(e) => setSmtpMode(e.target.value as 'tls' | 'starttls')} className="rounded-full border border-onyx/15 bg-canvas px-4 py-2.5 text-body-sm text-deep-ink">
                      <option value="tls">{t('modal.tls')}</option>
                      <option value="starttls">{t('modal.starttls')}</option>
                    </select>
                  </div>
                </div>
                <div className="grid gap-3 sm:grid-cols-2">
                  <Input id="mailcal-sent" label={t('modal.sentMailbox')} value={sentMailbox} onChange={(e) => setSentMailbox(e.target.value)} />
                  <Input id="mailcal-trash" label={t('modal.trashMailbox')} value={trashMailbox} onChange={(e) => setTrashMailbox(e.target.value)} />
                </div>
                <div className="rounded-2xl border border-onyx/15 p-3">
                  <p className="text-caption font-medium text-deep-ink">{t('modal.netTcpHint')}</p>
                  <pre className="mt-1 text-caption text-slate">{netTcpLines.join('\n')}</pre>
                </div>
              </>
            )}
            <Input id="mailcal-password" label={t('modal.password')} type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
          </>
        )}
        <p className="text-caption text-slate">{t('modal.secretsHint')}</p>
        {formError && <p role="alert" className="text-body-sm text-status-danger">{formError}</p>}

        <div className="flex justify-end gap-2">
          <Button type="button" variant="ghost" onClick={onClose}>
            {t('actions.cancel')}
          </Button>
          <Button type="submit" disabled={!canSubmit}>
            {t('actions.add')}
          </Button>
        </div>
      </form>
    </Modal>
  );
}

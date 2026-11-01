import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import type { MailcalAccountInput, MailcalKind, MailcalProvider } from '@/lib/api/mailcal';

interface AccountModalProps {
  isOpen: boolean;
  providers: MailcalProvider[];
  saving: boolean;
  onClose: () => void;
  onSubmit: (input: MailcalAccountInput) => void | Promise<void>;
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

  useEffect(() => {
    if (!isOpen) return;
    const first = providers[0];
    setPluginId(first?.plugin_id ?? '');
    setKind(kindsFor(first)[0] ?? 'both');
    setLabel('');
    setEmail('');
  }, [isOpen, providers]);

  const provider = providers.find((p) => p.plugin_id === pluginId);
  const kinds = kindsFor(provider);
  const canSubmit = Boolean(provider) && label.trim() !== '' && !saving;

  return (
    <Modal isOpen={isOpen} onClose={onClose} title={t('modal.title')} maxWidth="lg">
      <form
        className="flex flex-col gap-4"
        onSubmit={(event) => {
          event.preventDefault();
          if (!canSubmit) return;
          void onSubmit({ label: label.trim(), plugin_id: pluginId, kind, email: email.trim() || undefined });
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
        <p className="text-caption text-slate">{t('modal.secretsHint')}</p>

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

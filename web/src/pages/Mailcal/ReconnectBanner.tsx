import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { useToast } from '@/components/ui/Toast';
import { mailcalApi, type MailcalAccount } from '@/lib/api/mailcal';
import { ConnectButton } from './ConnectButton';

/** Shown on an account whose sign-in expired. OAuth accounts sign in again; password accounts take a new password. */
export function ReconnectBanner({ account, onDone }: { account: MailcalAccount; onDone: () => void }) {
  const { t } = useTranslation('mailcal');
  const { success, error } = useToast();
  const [password, setPassword] = useState('');
  const [busy, setBusy] = useState(false);

  if (account.status !== 'auth_expired') return null;

  const savePassword = async () => {
    setBusy(true);
    try {
      await mailcalApi.putPassword(account.id, password);
      setPassword('');
      success(t('reconnect.saved'));
      onDone();
    } catch (err) {
      error(t('reconnect.failed'), err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div role="alert" className="mt-3 flex flex-col gap-2 rounded-2xl border border-status-warning/30 bg-status-warning-soft p-3">
      <p className="text-body-sm text-status-warning">{account.status_detail || t('reconnect.generic')}</p>
      {account.auth.type === 'oauth' && (
        <div>
          <ConnectButton accountId={account.id} />
        </div>
      )}
      {account.auth.type === 'password' && (
        <div className="flex items-end gap-2">
          <div className="flex-1">
            <Input id={`pw-${account.id}`} label={t('reconnect.newPassword')} type="password" value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="new-password" />
          </div>
          <Button size="sm" disabled={busy || password === ''} onClick={() => void savePassword()}>{t('reconnect.save')}</Button>
        </div>
      )}
    </div>
  );
}

import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Link2 } from 'lucide-react';
import { Button } from '@/components/ui/Button';
import { useToast } from '@/components/ui/Toast';
import { mailcalApi } from '@/lib/api/mailcal';

/** Starts the provider sign-in. It navigates this tab so the nonce cookie set by the start call stays valid. */
export function ConnectButton({ accountId, reconnect = false }: { accountId: string; reconnect?: boolean }) {
  const { t } = useTranslation('mailcal');
  const { error } = useToast();
  const [busy, setBusy] = useState(false);

  const start = async () => {
    setBusy(true);
    try {
      const res = await mailcalApi.startOAuth(accountId);
      window.location.assign(res.auth_url);
    } catch (err) {
      error(t('oauth.startFailed'), err instanceof Error ? err.message : String(err));
      setBusy(false);
    }
  };

  return (
    <Button size="sm" icon={<Link2 className="h-4 w-4" />} disabled={busy} onClick={() => void start()}>
      {reconnect ? t('oauth.reconnect') : t('oauth.connect')}
    </Button>
  );
}

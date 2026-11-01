import { useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { useToast } from '@/components/ui/Toast';
import { mailcalApi } from '@/lib/api/mailcal';

/** The master switch for every write (send, delete, calendar changes). It starts off. */
export function MasterSwitch() {
  const { t } = useTranslation('mailcal');
  const { error } = useToast();
  const [enabled, setEnabled] = useState<boolean | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    let alive = true;
    mailcalApi
      .getSettings()
      .then((s) => alive && setEnabled(s.enabled))
      .catch(() => alive && setEnabled(false));
    return () => {
      alive = false;
    };
  }, []);

  if (enabled === null) return null;

  const toggle = async () => {
    setBusy(true);
    try {
      const next = await mailcalApi.putSettings(!enabled);
      setEnabled(next.enabled);
    } catch (err) {
      error(t('switch.failed'), err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <Card className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
      <div className="min-w-0">
        <p className="font-semibold text-deep-ink">{t('switch.label')}</p>
        <p className={`text-body-sm ${enabled ? 'text-slate' : 'text-status-warning'}`}>
          {enabled ? t('switch.onNote') : t('switch.offWarning')}
        </p>
      </div>
      <label className="flex shrink-0 items-center gap-2 text-body-sm text-deep-ink">
        <input type="checkbox" role="switch" aria-label={t('switch.label')} checked={enabled} disabled={busy} onChange={() => void toggle()} className="h-5 w-5" />
        <span>{enabled ? t('switch.on') : t('switch.off')}</span>
      </label>
    </Card>
  );
}

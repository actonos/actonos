import { Clock3 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Badge } from './Badge';

export function FreshnessBadge({ timestamp }: { timestamp?: string }) {
  const { t } = useTranslation('common');
  if (!timestamp) return <Badge variant="neutral">{t('status.noData')}</Badge>;
  const age = Math.max(0, Date.now() - Date.parse(timestamp));
  const stale = age > 30_000;
  return (
    <Badge variant={stale ? 'warning' : 'success'}>
      <Clock3 className="mr-1 h-3 w-3" aria-hidden="true" />
      {stale ? t('status.stale') : t('status.live')}
    </Badge>
  );
}

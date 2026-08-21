import { AlertTriangle, CheckCircle2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Badge } from '@/components/ui/Badge';
import { Card } from '@/components/ui/Card';

export interface AgentReviewItem { label: string; value: string }

export function AgentReviewSection({ errors, items }: { errors: string[]; items: AgentReviewItem[] }) {
  const { t } = useTranslation('agents');
  const valid = errors.length === 0;
  return (
    <Card className="space-y-5 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
      <div>
        <h3 className="font-serif text-heading-sm text-deep-ink">{t('studio.review.title')}</h3>
        <p className="text-caption text-slate">{t('studio.review.description')}</p>
      </div>
      <div className={`flex items-start gap-3 rounded-[18px] border p-4 ${valid ? 'border-status-success/30 bg-status-success-soft' : 'border-status-warning/30 bg-status-warning-soft'}`}>
        {valid ? <CheckCircle2 className="mt-0.5 h-5 w-5 shrink-0 text-status-success" /> : <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0 text-status-warning" />}
        <div>
          <p className="font-semibold text-deep-ink">{valid ? t('studio.review.ready') : t('studio.review.needsAttention')}</p>
          {!valid && <ul className="mt-2 list-disc space-y-1 pl-4 text-caption text-slate">{errors.map((item) => <li key={item}>{item}</li>)}</ul>}
        </div>
      </div>
      <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        {items.map((item) => (
          <div key={item.label} className="rounded-[18px] border border-onyx/10 bg-canvas p-4 shadow-xs">
            <dt className="text-caption font-semibold text-slate">{item.label}</dt>
            <dd className="mt-1 break-words text-body-sm font-semibold text-deep-ink">{item.value}</dd>
          </div>
        ))}
      </dl>
      <Badge variant={valid ? 'active' : 'warning'}>{valid ? t('studio.review.valid') : t('studio.review.invalid')}</Badge>
    </Card>
  );
}

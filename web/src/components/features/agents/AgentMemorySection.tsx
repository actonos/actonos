import { Brain, RefreshCw } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { EmptyState } from '@/components/ui/EmptyState';

export function AgentMemorySection({ value, refreshing, onRefresh }: {
  value: string;
  refreshing: boolean;
  onRefresh: () => void;
}) {
  const { t } = useTranslation('agents');
  return (
    <Card className="space-y-4 border border-onyx/10 bg-canvas/90 p-6">
      <div className="flex flex-col justify-between gap-3 border-b border-onyx/5 pb-3 sm:flex-row sm:items-center">
        <div>
          <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink">
            <Brain className="h-5 w-5" aria-hidden="true" />{t('studio.memory.title')}
          </h3>
          <p className="text-caption text-slate">{t('studio.memory.description')}</p>
        </div>
        <Button variant="ghost" size="sm" icon={<RefreshCw className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`} />} onClick={onRefresh} disabled={refreshing}>
          {t('studio.memory.refresh')}
        </Button>
      </div>
      {value ? (
        <textarea rows={18} value={value} readOnly className="w-full rounded-[20px] border border-onyx/10 bg-soft-meadow/80 p-4 font-mono text-body-sm leading-relaxed text-deep-ink" />
      ) : (
        <EmptyState icon={<Brain className="h-6 w-6" />} title={t('studio.memory.empty')} description={t('studio.memory.emptyDescription')} />
      )}
      <p className="text-right text-caption font-semibold text-deep-ink">{t('studio.memory.longTerm')}</p>
    </Card>
  );
}

import { useState } from 'react';
import { Brain, RefreshCw, Trash2 } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { EmptyState } from '@/components/ui/EmptyState';
import { ConfirmModal } from '@/components/ui/ConfirmModal';

export function AgentMemorySection({
  value,
  refreshing,
  clearing = false,
  onRefresh,
  onClear,
}: {
  value: string;
  refreshing: boolean;
  clearing?: boolean;
  onRefresh: () => void;
  onClear?: () => Promise<void>;
}) {
  const { t } = useTranslation('agents');
  const [confirmOpen, setConfirmOpen] = useState(false);

  return (
    <>
      <Card className="space-y-4 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
        <div className="flex flex-col justify-between gap-3 border-b border-onyx/5 pb-3 sm:flex-row sm:items-center">
          <div>
            <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink">
              <Brain className="h-5 w-5" aria-hidden="true" />
              {t('studio.memory.title')}
            </h3>
            <p className="text-caption text-slate">{t('studio.memory.description')}</p>
          </div>
          <div className="flex items-center gap-2">
            {onClear && (
              <Button
                variant="ghost"
                size="sm"
                icon={<Trash2 className="h-3.5 w-3.5 text-red-600" />}
                onClick={() => setConfirmOpen(true)}
                disabled={!value || refreshing || clearing}
                className="text-red-600 hover:bg-red-500/10"
              >
                {t('studio.memory.clear')}
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className={`h-3.5 w-3.5 ${refreshing ? 'animate-spin' : ''}`} />}
              onClick={onRefresh}
              disabled={refreshing || clearing}
            >
              {t('studio.memory.refresh')}
            </Button>
          </div>
        </div>
        {value ? (
          <textarea
            rows={18}
            value={value}
            readOnly
            className="w-full rounded-[20px] border border-onyx/15 bg-canvas/90 p-4 font-mono text-body-sm leading-relaxed text-deep-ink shadow-xs focus:outline-none"
          />
        ) : (
          <div className="rounded-[20px] border border-onyx/10 bg-canvas/70 p-4">
            <EmptyState
              icon={<Brain className="h-6 w-6" />}
              title={t('studio.memory.empty')}
              description={t('studio.memory.emptyDescription')}
            />
          </div>
        )}
        <div className="flex items-center justify-between text-caption font-mono text-slate">
          <span>{t('studio.length', { count: value.length })}</span>
          <span className="font-semibold text-deep-ink">{t('studio.memory.longTerm')}</span>
        </div>
      </Card>

      {onClear && (
        <ConfirmModal
          isOpen={confirmOpen}
          onClose={() => setConfirmOpen(false)}
          onConfirm={onClear}
          title={t('studio.memory.clearConfirmTitle')}
          description={t('studio.memory.clearConfirmDescription')}
          confirmLabel={t('studio.memory.clear')}
          variant="danger"
          loading={clearing}
        />
      )}
    </>
  );
}


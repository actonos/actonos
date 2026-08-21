import { Check, CheckCircle2, Wrench } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { ToolInfo } from '@/lib/types';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { EmptyState } from '@/components/ui/EmptyState';

export function AgentToolsSection({
  tools,
  authorizedTools,
  allSelected,
  onToggle,
  onClear,
  onSelectAll,
}: {
  tools: ToolInfo[];
  authorizedTools: string[];
  allSelected: boolean;
  onToggle: (name: string) => void;
  onClear: () => void;
  onSelectAll: () => void;
}) {
  const { t } = useTranslation('agents');
  return (
    <Card className="space-y-6 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
      <div className="flex flex-col justify-between gap-3 border-b border-onyx/5 pb-3 sm:flex-row sm:items-center">
        <div>
          <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink">
            <Wrench className="h-5 w-5" />
            {t('studio.tools.title')}
          </h3>
          <p className="text-caption text-slate">{t('studio.tools.description')}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="ghost" size="sm" onClick={onClear}>{t('studio.tools.clear')}</Button>
          <Button variant="primary" size="sm" onClick={onSelectAll}>{t('studio.tools.selectAll')}</Button>
        </div>
      </div>

      {allSelected && (
        <div className="flex items-center gap-2.5 rounded-[18px] border border-status-success/30 bg-status-success-soft p-3.5 text-body-sm text-deep-ink shadow-xs">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-status-success" />
          <span><strong>{t('studio.tools.fullTitle')}</strong> {t('studio.tools.fullDescription')}</span>
        </div>
      )}

      {tools.length === 0 ? (
        <div className="rounded-[20px] border border-onyx/10 bg-canvas/70 p-4">
          <EmptyState compact title={t('studio.tools.empty')} />
        </div>
      ) : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {tools.map((tool) => {
            const selected = allSelected || authorizedTools.includes(tool.name);
            return (
              <button
                key={tool.name}
                type="button"
                aria-pressed={selected}
                onClick={() => onToggle(tool.name)}
                className={`flex flex-col justify-between rounded-[20px] p-4 text-left transition-all cursor-pointer select-none ${
                  selected
                    ? 'border-2 border-deep-ink bg-canvas shadow-xs ring-1 ring-deep-ink/10'
                    : 'border border-onyx/15 bg-canvas/50 opacity-70 hover:opacity-100 hover:border-onyx/35 hover:bg-canvas'
                }`}
              >
                <div>
                  <div className="mb-2 flex items-center justify-between gap-2">
                    <span className="truncate font-mono text-body-sm font-bold text-deep-ink">
                      {tool.name}
                    </span>
                    <span
                      className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-full transition-all ${
                        selected
                          ? 'bg-deep-ink text-hi-yellow'
                          : 'border-2 border-onyx/25 bg-transparent'
                      }`}
                    >
                      {selected && <Check className="h-3 w-3 stroke-[3]" />}
                    </span>
                  </div>
                  <div className="mb-2">
                    <Badge variant="neutral" className="text-[10px] uppercase">
                      {tool.category}
                    </Badge>
                  </div>
                  <p className="line-clamp-2 text-caption text-slate">
                    {tool.description || t('studio.tools.noDescription')}
                  </p>
                </div>

                <div className="mt-3.5 flex items-center justify-between border-t border-onyx/10 pt-3 text-caption font-mono">
                  <span
                    className={`flex items-center gap-1.5 font-semibold ${
                      selected ? 'text-status-success' : 'text-slate'
                    }`}
                  >
                    {selected ? (
                      <>
                        <CheckCircle2 className="h-3.5 w-3.5" />
                        {t('studio.tools.authorized')}
                      </>
                    ) : (
                      t('studio.tools.disabled')
                    )}
                  </span>
                </div>
              </button>
            );
          })}
        </div>
      )}
    </Card>
  );
}


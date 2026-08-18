import { CheckCircle2, Wrench } from 'lucide-react';
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
    <Card className="space-y-6 border border-onyx/10 bg-canvas/90 p-6">
      <div className="flex flex-col justify-between gap-3 border-b border-onyx/5 pb-3 sm:flex-row sm:items-center">
        <div>
          <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink"><Wrench className="h-5 w-5" />{t('studio.tools.title')}</h3>
          <p className="text-caption text-slate">{t('studio.tools.description')}</p>
        </div>
        <div className="flex gap-2">
          <Button variant="ghost" size="sm" onClick={onClear}>{t('studio.tools.clear')}</Button>
          <Button variant="primary" size="sm" onClick={onSelectAll}>{t('studio.tools.selectAll')}</Button>
        </div>
      </div>
      {allSelected && (
        <div className="flex items-center gap-2.5 rounded-[18px] border border-success/30 bg-success/10 p-3.5 text-body-sm text-deep-ink">
          <CheckCircle2 className="h-4 w-4 shrink-0 text-success" />
          <span><strong>{t('studio.tools.fullTitle')}</strong> {t('studio.tools.fullDescription')}</span>
        </div>
      )}
      {tools.length === 0 ? <EmptyState compact title={t('studio.tools.empty')} /> : (
        <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {tools.map((tool) => {
            const selected = allSelected || authorizedTools.includes(tool.name);
            return (
              <button
                key={tool.name}
                type="button"
                aria-pressed={selected}
                onClick={() => onToggle(tool.name)}
                className={`flex flex-col justify-between rounded-[18px] border p-4 text-left transition-colors ${
                  selected ? 'border-deep-ink/30 bg-soft-meadow' : 'border-onyx/10 bg-canvas text-slate'
                }`}
              >
                <span>
                  <span className="mb-1.5 flex items-center justify-between gap-2">
                    <span className="truncate font-mono text-body-sm font-semibold text-deep-ink">{tool.name}</span>
                    <Badge variant="neutral">{tool.category}</Badge>
                  </span>
                  <span className="line-clamp-2 text-caption">{tool.description || t('studio.tools.noDescription')}</span>
                </span>
                <span className={`mt-3 border-t border-onyx/5 pt-3 text-caption font-semibold ${selected ? 'text-success' : 'text-slate'}`}>
                  {selected ? t('studio.tools.authorized') : t('studio.tools.disabled')}
                </span>
              </button>
            );
          })}
        </div>
      )}
    </Card>
  );
}

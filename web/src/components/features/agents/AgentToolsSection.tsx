import { useState, useMemo } from 'react';
import { Check, CheckCircle2, Wrench, Sparkles, Search, X } from 'lucide-react';
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
  const [searchQuery, setSearchQuery] = useState('');

  const isSkill = (tool: ToolInfo) =>
    tool.category === 'skill' || tool.name.startsWith('skill_');

  // Separate all tools into Skills vs System Tools
  const allSkills = useMemo(() => tools.filter(isSkill), [tools]);
  const allSystemTools = useMemo(() => tools.filter((t) => !isSkill(t)), [tools]);

  // Filtered by search
  const query = searchQuery.trim().toLowerCase();
  const filterFn = (tool: ToolInfo) => {
    if (!query) return true;
    return (
      tool.name.toLowerCase().includes(query) ||
      (tool.description || '').toLowerCase().includes(query) ||
      (tool.category || '').toLowerCase().includes(query)
    );
  };

  const filteredSkills = useMemo(() => allSkills.filter(filterFn), [allSkills, query]);
  const filteredSystemTools = useMemo(() => allSystemTools.filter(filterFn), [allSystemTools, query]);

  // Selected counts
  const selectedSkillsCount = allSelected
    ? allSkills.length
    : allSkills.filter((s) => authorizedTools.includes(s.name)).length;

  const selectedToolsCount = allSelected
    ? allSystemTools.length
    : allSystemTools.filter((t) => authorizedTools.includes(t.name)).length;

  const renderToolCard = (tool: ToolInfo, isSkillItem: boolean) => {
    const selected = allSelected || authorizedTools.includes(tool.name);
    const displayName = isSkillItem ? tool.name.replace(/^skill_/, '') : tool.name;

    return (
      <button
        key={tool.name}
        type="button"
        aria-pressed={selected}
        onClick={() => onToggle(tool.name)}
        className={`flex flex-col justify-between rounded-[20px] p-4 text-left transition-all cursor-pointer select-none ${
          selected
            ? 'border-2 border-deep-ink bg-canvas shadow-xs ring-1 ring-deep-ink/10'
            : 'border border-onyx/15 bg-canvas/50 opacity-75 hover:opacity-100 hover:border-onyx/35 hover:bg-canvas'
        }`}
      >
        <div>
          <div className="mb-2 flex items-center justify-between gap-2">
            <span
              className="truncate font-mono text-body-sm font-bold text-deep-ink"
              title={tool.name}
            >
              {displayName}
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

          <div className="mb-2 flex items-center gap-1.5 flex-wrap">
            <Badge
              variant={isSkillItem ? 'active' : 'neutral'}
              className="text-[10px] uppercase font-mono tracking-wider"
            >
              {tool.category || (isSkillItem ? 'skill' : 'tool')}
            </Badge>
            {isSkillItem && tool.name.startsWith('skill_') && (
              <span className="text-[10px] font-mono text-slate truncate">
                {tool.name}
              </span>
            )}
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
  };

  return (
    <div className="space-y-8">
      {/* Global Controls Card */}
      <Card className="space-y-4 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
        <div className="flex flex-col justify-between gap-4 sm:flex-row sm:items-center">
          <div>
            <h3 className="flex items-center gap-2 font-serif text-heading-sm text-deep-ink">
              <Wrench className="h-5 w-5 text-deep-ink" />
              {t('studio.tools.title')}
            </h3>
            <p className="text-caption text-slate">{t('studio.tools.description')}</p>
          </div>

          <div className="flex items-center gap-2 self-start sm:self-auto">
            <Button variant="ghost" size="sm" onClick={onClear}>
              {t('studio.tools.clear')}
            </Button>
            <Button variant="primary" size="sm" onClick={onSelectAll}>
              {t('studio.tools.selectAll')}
            </Button>
          </div>
        </div>

        {allSelected && (
          <div className="flex items-center gap-2.5 rounded-[18px] border border-status-success/30 bg-status-success-soft p-3.5 text-body-sm text-deep-ink shadow-xs">
            <CheckCircle2 className="h-4 w-4 shrink-0 text-status-success" />
            <span>
              <strong>{t('studio.tools.fullTitle')}</strong> {t('studio.tools.fullDescription')}
            </span>
          </div>
        )}

        {/* Quick Search Filter */}
        <div className="relative">
          <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder={t('studio.tools.searchPlaceholder', 'Filter skills and tools by name or description...')}
            className="w-full bg-canvas text-deep-ink pl-10 pr-9 py-2 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink transition-all"
          />
          {searchQuery && (
            <button
              onClick={() => setSearchQuery('')}
              className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink p-1"
            >
              <X className="w-3.5 h-3.5" />
            </button>
          )}
        </div>
      </Card>

      {/* SECTION 1: AGENT SKILLS */}
      <Card className="space-y-5 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-onyx/10 pb-3.5">
          <div className="flex items-start gap-3">
            <div className="w-9 h-9 rounded-full bg-deep-ink text-hi-yellow flex items-center justify-center shrink-0 mt-0.5">
              <Sparkles className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h4 className="font-serif text-heading-sm text-deep-ink">
                  {t('studio.tools.skillsSectionTitle', 'Agent Skills')}
                </h4>
                <Badge variant="active" className="text-[11px] font-mono">
                  {t('studio.tools.selectedCount', {
                    selected: selectedSkillsCount,
                    total: allSkills.length,
                    defaultValue: `${selectedSkillsCount} of ${allSkills.length} selected`,
                  })}
                </Badge>
              </div>
              <p className="text-caption text-slate mt-0.5">
                {t('studio.tools.skillsSectionDesc', 'Domain-specific instructions, prompt procedures, and automated skill workflows.')}
              </p>
            </div>
          </div>
        </div>

        {allSkills.length === 0 ? (
          <div className="rounded-[20px] border border-onyx/10 bg-canvas/70 p-6 text-center">
            <EmptyState
              compact
              title={t('studio.tools.noSkills', 'No skills installed yet')}
            />
          </div>
        ) : filteredSkills.length === 0 ? (
          <div className="rounded-[20px] border border-onyx/10 bg-canvas/70 p-6 text-center text-caption font-sans text-slate">
            {t('studio.tools.empty')}
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {filteredSkills.map((skill) => renderToolCard(skill, true))}
          </div>
        )}
      </Card>

      {/* SECTION 2: SYSTEM & NATIVE TOOLS */}
      <Card className="space-y-5 border border-onyx/15 bg-soft-meadow p-6 shadow-xs">
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 border-b border-onyx/10 pb-3.5">
          <div className="flex items-start gap-3">
            <div className="w-9 h-9 rounded-full bg-soft-meadow border border-onyx/15 text-deep-ink flex items-center justify-center shrink-0 mt-0.5">
              <Wrench className="w-4 h-4 text-deep-ink" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h4 className="font-serif text-heading-sm text-deep-ink">
                  {t('studio.tools.toolsSectionTitle', 'System & Native Tools')}
                </h4>
                <Badge variant="neutral" className="text-[11px] font-mono">
                  {t('studio.tools.selectedCount', {
                    selected: selectedToolsCount,
                    total: allSystemTools.length,
                    defaultValue: `${selectedToolsCount} of ${allSystemTools.length} selected`,
                  })}
                </Badge>
              </div>
              <p className="text-caption text-slate mt-0.5">
                {t('studio.tools.toolsSectionDesc', 'Built-in operating system capabilities, workspace file tools, and MCP connector tools.')}
              </p>
            </div>
          </div>
        </div>

        {allSystemTools.length === 0 ? (
          <div className="rounded-[20px] border border-onyx/10 bg-canvas/70 p-6 text-center">
            <EmptyState
              compact
              title={t('studio.tools.noTools', 'No system tools registered')}
            />
          </div>
        ) : filteredSystemTools.length === 0 ? (
          <div className="rounded-[20px] border border-onyx/10 bg-canvas/70 p-6 text-center text-caption font-sans text-slate">
            {t('studio.tools.empty')}
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {filteredSystemTools.map((tool) => renderToolCard(tool, false))}
          </div>
        )}
      </Card>
    </div>
  );
}


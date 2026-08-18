import { useTranslation } from 'react-i18next';
import { Search } from 'lucide-react';
import type { ConnectorCategory } from '@/lib/types';

interface ConnectorFilterBarProps {
  selectedCategory: ConnectorCategory;
  onSelectCategory: (category: ConnectorCategory) => void;
  statusFilter: 'all' | 'connected' | 'disconnected';
  onSelectStatusFilter: (status: 'all' | 'connected' | 'disconnected') => void;
  searchQuery: string;
  onSearchChange: (query: string) => void;
  categoryCounts: Record<ConnectorCategory, number>;
}

export function ConnectorFilterBar({
  selectedCategory,
  onSelectCategory,
  statusFilter,
  onSelectStatusFilter,
  searchQuery,
  onSearchChange,
  categoryCounts,
}: ConnectorFilterBarProps) {
  const { t } = useTranslation('connectors');

  const categories: { id: ConnectorCategory; labelKey: string; fallback: string }[] = [
    { id: 'all', labelKey: 'categories.all', fallback: 'All Connectors' },
    { id: 'productivity', labelKey: 'categories.productivity', fallback: 'Productivity' },
    { id: 'development', labelKey: 'categories.development', fallback: 'Development' },
    { id: 'knowledge', labelKey: 'categories.knowledge', fallback: 'Knowledge & AI' },
    { id: 'messaging', labelKey: 'categories.messaging', fallback: 'Messaging' },
    { id: 'database', labelKey: 'categories.database', fallback: 'Databases & Storage' },
  ];

  return (
    <div className="space-y-4 mb-6">
      {/* Category Pills & Search Bar Row */}
      <div className="flex flex-col lg:flex-row lg:items-center justify-between gap-4">
        {/* Category horizontal scrolling pills */}
        <div className="flex items-center gap-1.5 overflow-x-auto pb-1 max-w-full">
          {categories.map((cat) => {
            const count = categoryCounts[cat.id] || 0;
            const isSelected = selectedCategory === cat.id;

            return (
              <button
                key={cat.id}
                type="button"
                onClick={() => onSelectCategory(cat.id)}
                className={`px-3.5 py-1.5 rounded-full text-caption font-sans font-medium whitespace-nowrap transition-all flex items-center gap-1.5 cursor-pointer shrink-0 ${isSelected
                  ? 'bg-deep-ink text-white shadow-xs'
                  : 'bg-canvas text-deep-ink border border-onyx/10 hover:border-onyx/30 hover:bg-soft-meadow'
                  }`}
              >
                <span>{t(cat.labelKey, cat.fallback)}</span>
                <span
                  className={`text-[10px] font-mono px-1.5 py-0.2 rounded-full ${isSelected ? 'bg-white/20 text-white' : 'bg-onyx/5 text-slate'
                    }`}
                >
                  {count}
                </span>
              </button>
            );
          })}
        </div>

        {/* Right side: Status Filter & Search Input */}
        <div className="flex items-center gap-2.5 shrink-0">
          <div className="flex items-center gap-1 bg-soft-meadow p-1 rounded-full border border-onyx/10">
            <button
              type="button"
              onClick={() => onSelectStatusFilter('all')}
              className={`px-3 py-1 rounded-full text-[11px] font-medium transition-colors cursor-pointer ${statusFilter === 'all'
                ? 'bg-deep-ink text-white shadow-xs'
                : 'text-deep-ink hover:text-slate'
                }`}
            >
              {t('filter.all', 'All')}
            </button>
            <button
              type="button"
              onClick={() => onSelectStatusFilter('connected')}
              className={`px-3 py-1 rounded-full text-[11px] font-medium transition-colors cursor-pointer ${statusFilter === 'connected'
                ? 'bg-emerald-600 text-white shadow-xs'
                : 'text-deep-ink hover:text-slate'
                }`}
            >
              {t('status.connected', 'Connected')}
            </button>
            <button
              type="button"
              onClick={() => onSelectStatusFilter('disconnected')}
              className={`px-3 py-1 rounded-full text-[11px] font-medium transition-colors cursor-pointer ${statusFilter === 'disconnected'
                ? 'bg-deep-ink text-white shadow-xs'
                : 'text-deep-ink hover:text-slate'
                }`}
            >
              {t('status.available', 'Available')}
            </button>
          </div>

          <div className="relative w-48 sm:w-60">
            <Search className="w-3.5 h-3.5 text-slate absolute left-3 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder={t('actions.searchConnectors', 'Search connectors or scopes...')}
              value={searchQuery}
              onChange={(e) => onSearchChange(e.target.value)}
              className="w-full pl-8 pr-3 py-1.5 text-caption bg-canvas rounded-full border border-onyx/10 focus:outline-none focus:border-onyx/30 shadow-xs"
            />
          </div>
        </div>
      </div>
    </div>
  );
}

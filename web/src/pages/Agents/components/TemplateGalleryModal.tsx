import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { api } from '@/lib/api';
import type { AgentTemplate } from '@/lib/types';
import {
  Sparkles,
  Check,
  Bot,
  HeartPulse,
  Code2,
  CalendarCheck,
  Globe,
  Headphones,
  Mail,
  Bug,
  GitPullRequest,
  Calendar,
  FileText,
  BookOpen,
  BarChart3,
  ShieldAlert,
  Coins,
  Database,
  CheckSquare,
} from 'lucide-react';

const ICON_MAP: Record<string, React.ElementType> = {
  Code2,
  CalendarCheck,
  Globe,
  Headphones,
  Mail,
  Bug,
  GitPullRequest,
  Calendar,
  FileText,
  BookOpen,
  BarChart3,
  ShieldAlert,
  Coins,
  Database,
  CheckSquare,
};

export interface TemplateGalleryModalProps {
  isOpen: boolean;
  onClose: () => void;
  onSelectTemplate: (template: AgentTemplate) => void;
}

export function TemplateGalleryModal({
  isOpen,
  onClose,
  onSelectTemplate,
}: TemplateGalleryModalProps) {
  const { t } = useTranslation('agents');
  const [templates, setTemplates] = useState<AgentTemplate[]>([]);
  const [loading, setLoading] = useState(true);
  const [categoryFilter, setCategoryFilter] = useState<string>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedTemplate, setSelectedTemplate] = useState<AgentTemplate | null>(null);

  useEffect(() => {
    if (!isOpen) return;
    const fetchTemplates = async () => {
      setLoading(true);
      try {
        const res = await api.listAgentTemplates();
        setTemplates(res.templates || []);
      } catch {
        // Fallback handled gracefully
      } finally {
        setLoading(false);
      }
    };
    fetchTemplates();
  }, [isOpen]);

  const categories = [
    { id: 'all', label: t('templates.categories.all', 'All Categories') },
    { id: 'development', label: t('templates.categories.dev', 'Development') },
    { id: 'operations', label: t('templates.categories.ops', 'Operations & FinOps') },
    { id: 'productivity', label: t('templates.categories.prod', 'Productivity & Support') },
    { id: 'security', label: t('templates.categories.sec', 'Security & Audit') },
    { id: 'analysis', label: t('templates.categories.analysis', 'Data & SEO') },
  ];

  const filteredTemplates = useMemo(() => {
    return templates.filter((tmpl) => {
      if (categoryFilter !== 'all' && tmpl.category !== categoryFilter) {
        return false;
      }
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const matchName = tmpl.name.toLowerCase().includes(q);
        const matchDesc = tmpl.description.toLowerCase().includes(q);
        const matchTags = tmpl.tags.some((tag) => tag.toLowerCase().includes(q));
        if (!matchName && !matchDesc && !matchTags) {
          return false;
        }
      }
      return true;
    });
  }, [templates, categoryFilter, searchQuery]);

  const handleApplyTemplate = (tmpl: AgentTemplate) => {
    onSelectTemplate(tmpl);
    onClose();
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('templates.modalTitle', 'Agent Templates Marketplace')}
      maxWidth="max-w-4xl"
    >
      <div className="space-y-5">
        {/* Subtitle & Search Bar */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3">
          <p className="text-body-sm text-slate">
            {t('templates.modalDesc', 'Choose a battle-tested template to instantly instantiate a specialized autonomous agent.')}
          </p>

          <div className="w-full sm:w-72">
            <Input
              placeholder={t('templates.searchPlaceholder', 'Search templates or tags...')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="text-body-sm"
            />
          </div>
        </div>

        {/* Category Pills */}
        <div className="flex flex-wrap items-center gap-1.5 border-b border-onyx/10 pb-3">
          {categories.map((cat) => (
            <button
              key={cat.id}
              type="button"
              onClick={() => setCategoryFilter(cat.id)}
              className={`px-3 py-1 rounded-full text-caption font-medium transition-colors cursor-pointer ${
                categoryFilter === cat.id
                  ? 'bg-deep-ink text-white font-semibold shadow-2xs'
                  : 'bg-soft-meadow text-deep-ink hover:bg-soft-meadow/80'
              }`}
            >
              {cat.label}
            </button>
          ))}
        </div>

        {/* Templates Grid */}
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4 max-h-[60vh] overflow-y-auto pr-1">
          {loading ? (
            <div className="col-span-2 py-16 text-center text-slate">
              <Sparkles className="w-6 h-6 mx-auto animate-spin text-slate mb-2" />
              <p className="text-body-sm font-medium">Loading templates...</p>
            </div>
          ) : filteredTemplates.length === 0 ? (
            <div className="col-span-2 py-16 text-center text-slate">
              <Bot className="w-8 h-8 mx-auto text-slate/50 mb-2" />
              <p className="text-body-sm font-medium">No templates matching your filter criteria.</p>
            </div>
          ) : (
            filteredTemplates.map((tmpl) => {
              const Icon = ICON_MAP[tmpl.icon] || Bot;
              const isSelected = selectedTemplate?.id === tmpl.id;

              return (
                <Card
                  key={tmpl.id}
                  onClick={() => setSelectedTemplate(tmpl)}
                  className={`p-4 border transition-all cursor-pointer flex flex-col justify-between space-y-3 relative group ${
                    isSelected
                      ? 'border-deep-ink bg-soft-meadow/60 ring-2 ring-deep-ink/20 shadow-xs'
                      : 'border-onyx/10 bg-canvas hover:bg-soft-meadow/30'
                  }`}
                >
                  <div className="space-y-2">
                    <div className="flex items-start justify-between gap-2">
                      <div className="flex items-center gap-3">
                        <div className="w-9 h-9 rounded-xl bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink shrink-0 group-hover:scale-105 transition-transform">
                          <Icon className="w-5 h-5" />
                        </div>
                        <div>
                          <h4 className="text-body-sm font-bold text-deep-ink leading-tight">
                            {tmpl.name}
                          </h4>
                          <span className="text-[11px] text-slate capitalize font-mono">
                            {tmpl.category} • v{tmpl.version}
                          </span>
                        </div>
                      </div>

                      <Badge variant="neutral" className="text-[10px] shrink-0 font-mono">
                        {tmpl.manifest.model_config.primary_model.split('/')[1] || tmpl.manifest.model_config.primary_model}
                      </Badge>
                    </div>

                    <p className="text-caption text-slate line-clamp-2">
                      {tmpl.description}
                    </p>

                    {/* Tag chips */}
                    <div className="flex flex-wrap gap-1 pt-1">
                      {tmpl.tags.slice(0, 4).map((tag) => (
                        <span
                          key={tag}
                          className="px-1.5 py-0.5 rounded bg-onyx/5 text-[10px] font-mono text-slate"
                        >
                          #{tag}
                        </span>
                      ))}
                    </div>
                  </div>

                  {/* Footer Action */}
                  <div className="flex items-center justify-between pt-2 border-t border-onyx/5">
                    <div className="flex items-center gap-2 text-[11px] text-slate font-mono">
                      <span>{tmpl.manifest.authorized_tools?.length || 0} tools</span>
                      {tmpl.manifest.heartbeat_config?.enabled && (
                        <span className="flex items-center gap-0.5 text-emerald-700">
                          <HeartPulse className="w-3 h-3" /> 24/7
                        </span>
                      )}
                    </div>

                    <Button
                      variant="primary"
                      size="sm"
                      onClick={(e) => {
                        e.stopPropagation();
                        handleApplyTemplate(tmpl);
                      }}
                      icon={<Check className="w-3.5 h-3.5" />}
                      className="text-xs py-1 px-2.5 h-7"
                    >
                      {t('templates.useTemplate', 'Use Template')}
                    </Button>
                  </div>
                </Card>
              );
            })
          )}
        </div>
      </div>
    </Modal>
  );
}

import { useState, useEffect, useMemo } from 'react';
import { getErrorMessage } from '@/lib/errors';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { readHashParams, setHashParam } from '@/lib/url-state';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import {
  Plus,
  RefreshCw,
  Search,
  Download,
  Trash2,
  Sparkles,
  Github,
  Activity,
  CheckCircle2,
  AlertTriangle,
  Star,
  FileText,
  Layers,
  ExternalLink,
  Eye,
  Terminal,
  Cpu,
  BarChart3,
  X,
} from 'lucide-react';
import { api, type HubSkillItem } from '@/lib/api';
import type { ToolInfo } from '@/lib/types';
import { useActionProgress } from '@/lib/useActionProgress';

export function SkillsPage() {
  const { t } = useTranslation('skills');
  const { success, error, info } = useToast();
  const [installedSkills, setInstalledSkills] = useState<ToolInfo[]>([]);
  const [hubCatalog, setHubCatalog] = useState<HubSkillItem[]>([]);
  const [activeTab, setActiveTab] = useState<'installed' | 'hub'>(() =>
    readHashParams().get('view') === 'catalog' ? 'hub' : 'installed'
  );
  const selectTab = (tab: 'installed' | 'hub') => {
    setActiveTab(tab);
    setHashParam('view', tab === 'installed' ? undefined : 'catalog');
  };

  // Search, Filters & Sorting
  const [searchQuery, setSearchQuery] = useState('');
  const [selectedCategory, setSelectedCategory] = useState<string>('all');
  const [selectedStatus, setSelectedStatus] = useState<'all' | 'installed' | 'available'>('all');
  const [sortBy, setSortBy] = useState<'stars' | 'recent' | 'name'>('stars');
  const [pageSize, setPageSize] = useState<number>(18);

  const [loading, setLoading] = useState(true);
  const [uninstallingSkill, setUninstallingSkill] = useState<string | null>(null);
  const [togglingSkill, setTogglingSkill] = useState<string | null>(null);

  // Skill inspector modal
  const [inspectSkill, setInspectSkill] = useState<HubSkillItem | null>(null);

  // Global Action & Approval Progress Manager
  const { executeAction, isExecuting } = useActionProgress();

  // Create skill form modal
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [skillName, setSkillName] = useState('');
  const [skillDesc, setSkillDesc] = useState('');
  const [skillContent, setSkillContent] = useState('');

  const loadData = async () => {
    try {
      setLoading(true);
      const [toolsRes, hubRes] = await Promise.all([
        api.listTools('skill').catch(() => ({ tools: [], count: 0 })),
        api.listHubCatalog().catch(() => ({ catalog: [], count: 0 })),
      ]);
      setInstalledSkills(toolsRes.tools || []);
      setHubCatalog(hubRes.catalog || []);
    } catch (err) {
      error('Failed to load skills', getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();
    const handleUpdates = () => {
      loadData();
    };
    window.addEventListener('actonos:tools-updated', handleUpdates);
    window.addEventListener('actonos:approval-decided', handleUpdates);
    return () => {
      window.removeEventListener('actonos:tools-updated', handleUpdates);
      window.removeEventListener('actonos:approval-decided', handleUpdates);
    };
  }, []);

  // Toggle Installed Skill Enable/Disable
  const handleToggleSkill = async (skill: ToolInfo) => {
    if (skill.requirements_met === false) {
      info(
        t('requirements.missing', 'Missing Requirements'),
        skill.missing_requirements?.join(', ') || t('requirements.unmetExplanation')
      );
      return;
    }

    const newEnabled = !(skill.enabled ?? true);
    setTogglingSkill(skill.name);
    try {
      await api.toggleSkill(skill.name, newEnabled);
      setInstalledSkills((prev) =>
        prev.map((s) => (s.name === skill.name ? { ...s, enabled: newEnabled } : s))
      );
      if (newEnabled) {
        success(
          t('toggle.enabled', 'Active'),
          t('toggle.successEnabled', { name: skill.name, defaultValue: `Skill ${skill.name} is now enabled.` })
        );
      } else {
        info(
          t('toggle.disabled', 'Disabled'),
          t('toggle.successDisabled', { name: skill.name, defaultValue: `Skill ${skill.name} is now disabled.` })
        );
      }
    } catch (err) {
      error('Toggle failed', getErrorMessage(err));
    } finally {
      setTogglingSkill(null);
    }
  };

  const handleInstall = (skillId: string, displayName?: string) => {
    const targetName = displayName || skillId;
    executeAction({
      targetId: skillId,
      title: t('hub.installingTitle', { name: targetName, defaultValue: `Installing Skill: ${targetName}` }),
      subtitle: t('hub.installingSubtitle', { defaultValue: 'Downloading package files, verifying dependencies, and hot-reloading into kernel.' }),
      steps: [
        { id: 'auth', label: t('actionProgress.steps.auth', { defaultValue: 'Security Authorization & Approval' }) },
        { id: 'download', label: t('actionProgress.steps.download', { defaultValue: 'Package Acquisition & Download' }) },
        { id: 'verify', label: t('actionProgress.steps.verify', { defaultValue: 'Requirements Verification & Registration' }) },
      ],
      action: () => api.installHubSkill(skillId),
      onSuccess: () => {
        success(t('hub.installed', 'Installed'), `Skill ${targetName} is now active.`);
        loadData();
      },
    });
  };

  const handleConfirmUninstall = () => {
    if (!uninstallingSkill) return;
    const target = uninstallingSkill;
    setUninstallingSkill(null);
    executeAction({
      targetId: target,
      title: t('hub.uninstallingTitle', { name: target, defaultValue: `Uninstalling Skill: ${target}` }),
      subtitle: t('hub.uninstallingSubtitle', { defaultValue: 'Removing skill package directory and unregistering tools.' }),
      steps: [
        { id: 'auth', label: t('actionProgress.steps.auth', { defaultValue: 'Security Authorization & Approval' }) },
        { id: 'remove', label: t('hub.removingFiles', { defaultValue: 'Removing files & unregistering from runtime' }) },
      ],
      action: () => api.uninstallHubSkill(target),
      onSuccess: () => {
        success(t('hub.uninstall', 'Uninstalled'), `Skill ${target} removed.`);
        if (inspectSkill && (inspectSkill.id === target || inspectSkill.slug === target)) {
          setInspectSkill(null);
        }
        const targetClean = target.toLowerCase().replace(/^skill_/, '').replace(/[-_]/g, '');
        setInstalledSkills((prev) =>
          prev.filter((s) => {
            const sClean = s.name.toLowerCase().replace(/^skill_/, '').replace(/[-_]/g, '');
            return sClean !== targetClean;
          })
        );
        setHubCatalog((prev) =>
          prev.map((s) => {
            const sCleanId = s.id.toLowerCase().replace(/[-_]/g, '');
            const sCleanSlug = (s.slug || '').toLowerCase().replace(/[-_]/g, '');
            if (sCleanId === targetClean || sCleanSlug === targetClean) {
              return { ...s, installed: false };
            }
            return s;
          })
        );
        loadData();
      },
    });
  };

  const handleCreateSkill = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!skillName.trim()) return;
    const name = skillName.trim().toLowerCase().replace(/\s+/g, '_');
    const desc = skillDesc.trim();
    const content = skillContent.trim();
    setIsCreateOpen(false);

    executeAction({
      targetId: name,
      title: t('createModal.creatingTitle', { name, defaultValue: `Creating Custom Skill: ${name}` }),
      steps: [
        { id: 'auth', label: t('actionProgress.steps.auth', { defaultValue: 'Security Authorization' }) },
        { id: 'write', label: t('createModal.writingFiles', { defaultValue: 'Writing SKILL.md and registering tool' }) },
      ],
      action: () => api.createSkill({ name, description: desc, content }),
      onSuccess: () => {
        success(t('createModal.submit', 'Created'), `Skill ${name} created.`);
        setSkillName('');
        setSkillDesc('');
        setSkillContent('');
        loadData();
      },
    });
  };

  // Distinct categories with counts from catalog
  const categoryCounts = useMemo(() => {
    const counts: Record<string, number> = { all: hubCatalog.length };
    for (const item of hubCatalog) {
      const cat = item.category || 'other';
      counts[cat] = (counts[cat] || 0) + 1;
    }
    return counts;
  }, [hubCatalog]);

  const availableCategories = useMemo(() => {
    const cats = Object.keys(categoryCounts).filter((c) => c !== 'all');
    cats.sort((a, b) => (categoryCounts[b] || 0) - (categoryCounts[a] || 0));
    return ['all', ...cats];
  }, [categoryCounts]);

  // Filtered & Sorted Community Catalog
  const filteredHub = useMemo(() => {
    let list = hubCatalog.filter((s) => {
      // Search
      const query = searchQuery.trim().toLowerCase();
      if (query) {
        const matchesName = s.name.toLowerCase().includes(query);
        const matchesDesc = s.description?.toLowerCase().includes(query);
        const matchesSlug = s.slug?.toLowerCase().includes(query);
        const matchesAuthor = s.author?.toLowerCase().includes(query);
        const matchesTags = s.tags?.some((tg) => tg.toLowerCase().includes(query));
        if (!matchesName && !matchesDesc && !matchesSlug && !matchesAuthor && !matchesTags) {
          return false;
        }
      }

      // Category
      if (selectedCategory !== 'all' && s.category !== selectedCategory) {
        return false;
      }

      // Status
      if (selectedStatus === 'installed' && !s.installed) return false;
      if (selectedStatus === 'available' && s.installed) return false;

      return true;
    });

    // Sorting
    list = [...list].sort((a, b) => {
      if (sortBy === 'stars') {
        return (b.stars || 0) - (a.stars || 0);
      }
      if (sortBy === 'recent') {
        const dateA = Number(a.updatedAt) || 0;
        const dateB = Number(b.updatedAt) || 0;
        return dateB - dateA;
      }
      return a.name.localeCompare(b.name);
    });

    return list;
  }, [hubCatalog, searchQuery, selectedCategory, selectedStatus, sortBy]);

  // Filtered Installed Skills
  const filteredInstalled = useMemo(() => {
    return installedSkills.filter((s) => {
      const query = searchQuery.trim().toLowerCase();
      if (!query) return true;
      return (
        s.name.toLowerCase().includes(query) ||
        s.description?.toLowerCase().includes(query) ||
        s.category?.toLowerCase().includes(query)
      );
    });
  }, [installedSkills, searchQuery]);

  const paginatedHub = useMemo(() => {
    return filteredHub.slice(0, pageSize);
  }, [filteredHub, pageSize]);

  const getCategoryIcon = (category?: string) => {
    switch (category) {
      case 'software-dev':
      case 'developer':
      case 'devops':
      case 'automation':
        return <Terminal className="w-5 h-5 text-deep-ink" />;
      case 'data-research':
      case 'research':
      case 'analytics':
        return <Search className="w-5 h-5 text-deep-ink" />;
      case 'marketing-sales':
      case 'finance-legal':
      case 'finance':
      case 'sales':
      case 'marketing':
        return <BarChart3 className="w-5 h-5 text-deep-ink" />;
      case 'sre':
      case 'operations':
        return <Activity className="w-5 h-5 text-deep-ink" />;
      case 'utility':
      case 'productivity':
        return <Cpu className="w-5 h-5 text-deep-ink" />;
      case 'human-resources':
      case 'hr':
      case 'customer-support':
      case 'support':
      case 'communication':
      case 'education':
      case 'design':
      case 'creative':
      default:
        return <Sparkles className="w-5 h-5 text-deep-ink" />;
    }
  };

  const formatStars = (stars?: number) => {
    if (!stars) return '0';
    if (stars >= 1000) {
      return `${(stars / 1000).toFixed(stars >= 10000 ? 0 : 1)}k`;
    }
    return stars.toString();
  };

  const clearAllFilters = () => {
    setSearchQuery('');
    setSelectedCategory('all');
    setSelectedStatus('all');
  };

  return (
    <div className="relative min-h-[calc(100vh-64px)] pb-16">
      <BlobBackdrop />

      <PageContainer maxWidth="wide">
        <PageHeader
          eyebrow={t('eyebrow')}
          title={t('title')}
          description={t('subtitle')}
          actions={
            <>
              <Button variant="ghost" size="sm" icon={<RefreshCw />} onClick={loadData}>
                {t('actions.refresh')}
              </Button>
              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="w-3.5 h-3.5" />}
                onClick={() => setIsCreateOpen(true)}
              >
                {t('actions.newSkill', 'Create Skill')}
              </Button>
            </>
          }
        />

        {/* Primary View Switcher & Search Bar */}
        <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-4 mb-6">
          <div className="flex items-center gap-1.5 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs self-start">
            <button
              onClick={() => selectTab('installed')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${activeTab === 'installed'
                  ? 'bg-deep-ink text-white font-semibold shadow-xs'
                  : 'text-deep-ink hover:text-slate'
                }`}
            >
              {t('tabs.installed', { count: installedSkills.length })}
            </button>
            <button
              onClick={() => selectTab('hub')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${activeTab === 'hub'
                  ? 'bg-deep-ink text-white font-semibold shadow-xs'
                  : 'text-deep-ink hover:text-slate'
                }`}
            >
              {t('tabs.hub', { count: hubCatalog.length })}
            </button>
          </div>

          <div className="relative w-full md:max-w-md">
            <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder={t('actions.search', 'Search by name, description, author, tags...')}
              value={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.target.value);
                setPageSize(18);
              }}
              className="w-full bg-soft-meadow text-deep-ink pl-10 pr-9 py-2 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink transition-all"
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery('')}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink"
                title={t('actions.clearFilters', 'Clear')}
              >
                <X className="w-3.5 h-3.5" />
              </button>
            )}
          </div>
        </div>

        {/* Installed Skills Tab */}
        {activeTab === 'installed' && (
          <div>
            {loading ? (
              <div className="py-20 text-center text-slate font-sans">{t('loading')}</div>
            ) : filteredInstalled.length === 0 ? (
              <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-md mx-auto border border-onyx/10 shadow-xs">
                <Sparkles className="w-12 h-12 text-deep-ink mx-auto mb-3 opacity-40" />
                <p className="font-sans text-body-sm text-slate mb-2 font-semibold">
                  {searchQuery ? t('empty.noMatches', 'No matching skills found') : t('empty.title', 'No skills installed')}
                </p>
                <p className="font-sans text-caption text-slate mb-5">
                  {searchQuery ? t('empty.noMatches') : t('empty.subtitle')}
                </p>
                {searchQuery ? (
                  <Button variant="ghost" size="sm" onClick={() => setSearchQuery('')}>
                    {t('actions.clearFilters', 'Clear Search')}
                  </Button>
                ) : (
                  <Button variant="primary" size="sm" onClick={() => selectTab('hub')}>
                    {t('tabs.hub', { count: hubCatalog.length })}
                  </Button>
                )}
              </div>
            ) : (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                {filteredInstalled.map((skill) => {
                  const isEnabled = skill.enabled ?? true;
                  const isRequirementsMet = skill.requirements_met !== false;
                  const missingReqs = skill.missing_requirements || [];

                  return (
                    <Card
                      key={skill.name}
                      className={`p-6 border flex flex-col justify-between transition-all duration-200 ${isEnabled
                          ? 'bg-canvas/95 border-onyx/10 shadow-xs hover:border-onyx/20'
                          : 'bg-soft-meadow/50 border-onyx/5 opacity-75'
                        }`}
                    >
                      <div>
                        {/* Header with Icon, Status Badge & Toggle Switch */}
                        <div className="flex items-center justify-between mb-4">
                          <div className="w-10 h-10 rounded-full bg-soft-meadow flex items-center justify-center border border-onyx/10">
                            {getCategoryIcon(skill.category)}
                          </div>

                          <div className="flex items-center gap-2">
                            {/* Requirements Status Badge */}
                            {isRequirementsMet ? (
                              <Badge variant="active" className="flex items-center gap-1 text-[11px]">
                                <CheckCircle2 className="w-3 h-3" />
                                {t('requirements.met', 'Ready')}
                              </Badge>
                            ) : (
                              <span title={missingReqs.join('\n')}>
                                <Badge variant="danger" className="flex items-center gap-1 text-[11px]">
                                  <AlertTriangle className="w-3 h-3" />
                                  {t('requirements.missing', 'Requirements Missing')}
                                </Badge>
                              </span>
                            )}

                            {/* Enable/Disable Toggle Switch */}
                            <button
                              type="button"
                              onClick={() => handleToggleSkill(skill)}
                              disabled={togglingSkill === skill.name || !isRequirementsMet}
                              title={
                                !isRequirementsMet
                                  ? missingReqs.join(', ')
                                  : isEnabled
                                    ? t('toggle.disable', 'Disable skill')
                                    : t('toggle.enable', 'Enable skill')
                              }
                              className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${!isRequirementsMet
                                  ? 'bg-onyx/10 cursor-not-allowed opacity-50'
                                  : isEnabled
                                    ? 'bg-deep-ink'
                                    : 'bg-onyx/20 hover:bg-onyx/30'
                                }`}
                            >
                              <span
                                className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow ring-0 transition duration-200 ease-in-out ${isEnabled ? 'translate-x-5' : 'translate-x-0'
                                  }`}
                              />
                            </button>
                          </div>
                        </div>

                        <h3 className="font-serif text-heading-sm text-deep-ink mb-1 flex items-center gap-2">
                          {skill.name}
                        </h3>
                        <p className="font-sans text-body-sm text-slate mb-3">
                          {skill.description || 'Custom skill instructions and tools.'}
                        </p>

                        {/* Missing Requirements Alert Box */}
                        {!isRequirementsMet && missingReqs.length > 0 && (
                          <div className="mb-4 p-3 rounded-[12px] bg-red-500/10 border border-red-500/20 text-caption font-sans text-deep-ink space-y-1">
                            <div className="font-semibold flex items-center gap-1.5 text-red-700">
                              <AlertTriangle className="w-3.5 h-3.5" />
                              {t('requirements.missing', 'Missing Requirements')}
                            </div>
                            <ul className="list-disc list-inside text-slate space-y-0.5 font-mono text-[11px]">
                              {missingReqs.map((m, idx) => (
                                <li key={idx}>{m}</li>
                              ))}
                            </ul>
                          </div>
                        )}
                      </div>

                      {/* Card Footer */}
                      <div className="pt-4 mt-2 border-t border-soft-meadow flex items-center justify-between">
                        <span className="text-caption text-slate font-mono uppercase tracking-wider">
                          {skill.category || 'skill'}
                        </span>
                        <div className="flex items-center gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="text-red-600 hover:text-red-700 hover:bg-red-50"
                            icon={<Trash2 className="w-3.5 h-3.5" />}
                            onClick={() => setUninstallingSkill(skill.name.replace(/^skill_/, ''))}
                          >
                            {t('hub.uninstall', 'Uninstall')}
                          </Button>
                        </div>
                      </div>
                    </Card>
                  );
                })}
              </div>
            )}
          </div>
        )}

        {/* Community Hub Tab */}
        {activeTab === 'hub' && (
          <div>
            {/* Category Filter Pills */}
            <div className="flex items-center gap-2 overflow-x-auto pb-3 mb-4 scrollbar-none">
              {availableCategories.map((cat) => {
                const isSelected = selectedCategory === cat;
                const count = categoryCounts[cat] || 0;
                return (
                  <button
                    key={cat}
                    onClick={() => {
                      setSelectedCategory(cat);
                      setPageSize(18);
                    }}
                    className={`inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-full text-caption font-sans font-medium whitespace-nowrap transition-all cursor-pointer border ${isSelected
                        ? 'bg-deep-ink text-white border-deep-ink shadow-xs font-semibold'
                        : 'bg-canvas/90 text-deep-ink border-onyx/10 hover:border-onyx/30 hover:bg-soft-meadow'
                      }`}
                  >
                    <span>{t(`categories.${cat}`, cat)}</span>
                    <span
                      className={`text-[10px] px-1.5 py-0.2 rounded-full font-mono ${isSelected ? 'bg-white/20 text-white' : 'bg-soft-meadow text-slate'
                        }`}
                    >
                      {count}
                    </span>
                  </button>
                );
              })}
            </div>

            {/* Controls Bar: Status Filter + Sort + Count */}
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-6 p-3 rounded-[16px] bg-canvas/80 border border-onyx/10">
              <div className="flex items-center gap-2">
                <span className="text-caption font-semibold uppercase tracking-wider text-slate px-1">
                  {t('hub.title', 'Community Registry')}
                </span>
                <span className="text-caption font-mono text-slate bg-soft-meadow px-2 py-0.5 rounded-full border border-onyx/5">
                  {filteredHub.length} {t('tabs.hub', { count: filteredHub.length })}
                </span>
              </div>

              <div className="flex items-center gap-3 self-end sm:self-auto">
                {/* Status Segment */}
                <div className="flex items-center bg-soft-meadow rounded-full p-0.5 border border-onyx/10 text-caption">
                  <button
                    onClick={() => setSelectedStatus('all')}
                    className={`px-2.5 py-1 rounded-full transition-all cursor-pointer ${selectedStatus === 'all' ? 'bg-deep-ink text-white font-medium shadow-2xs' : 'text-slate hover:text-deep-ink'
                      }`}
                  >
                    {t('statusFilter.all', 'All')}
                  </button>
                  <button
                    onClick={() => setSelectedStatus('installed')}
                    className={`px-2.5 py-1 rounded-full transition-all cursor-pointer ${selectedStatus === 'installed' ? 'bg-deep-ink text-white font-medium shadow-2xs' : 'text-slate hover:text-deep-ink'
                      }`}
                  >
                    {t('statusFilter.installed', 'Installed')}
                  </button>
                  <button
                    onClick={() => setSelectedStatus('available')}
                    className={`px-2.5 py-1 rounded-full transition-all cursor-pointer ${selectedStatus === 'available' ? 'bg-deep-ink text-white font-medium shadow-2xs' : 'text-slate hover:text-deep-ink'
                      }`}
                  >
                    {t('statusFilter.available', 'Available')}
                  </button>
                </div>

                {/* Sort Dropdown */}
                <div className="flex items-center gap-1.5 text-caption font-sans">
                  <span className="text-slate hidden sm:inline">{t('sort.label', 'Sort:')}</span>
                  <select
                    value={sortBy}
                    onChange={(e) => setSortBy(e.target.value as 'stars' | 'recent' | 'name')}
                    className="bg-soft-meadow text-deep-ink border border-onyx/10 rounded-full px-3 py-1 text-caption focus:outline-none focus:ring-1 focus:ring-deep-ink cursor-pointer"
                  >
                    <option value="stars">{t('sort.stars', 'Most Popular (Stars)')}</option>
                    <option value="recent">{t('sort.recent', 'Recently Updated')}</option>
                    <option value="name">{t('sort.name', 'Name (A-Z)')}</option>
                  </select>
                </div>
              </div>
            </div>

            {/* Grid of Community Skills */}
            {loading ? (
              <div className="py-20 text-center text-slate font-sans">{t('loading')}</div>
            ) : filteredHub.length === 0 ? (
              <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-md mx-auto border border-onyx/10 shadow-xs">
                <Search className="w-12 h-12 text-deep-ink mx-auto mb-3 opacity-30" />
                <p className="font-sans text-body-sm text-deep-ink mb-1 font-semibold">
                  {t('empty.noMatches', 'No skills match your filter criteria')}
                </p>
                <p className="font-sans text-caption text-slate mb-4">
                  Try adjusting your search keywords, status filter, or category.
                </p>
                <Button variant="primary" size="sm" onClick={clearAllFilters}>
                  {t('actions.clearFilters', 'Clear Filters')}
                </Button>
              </div>
            ) : (
              <>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                  {paginatedHub.map((skill) => {
                    const isInstalled = skill.installed;
                    const fileCount = skill.files?.length || (skill.isMultiFile ? 2 : 1);

                    return (
                      <Card
                        key={skill.id}
                        className="p-6 border border-onyx/10 flex flex-col justify-between bg-canvas/95 shadow-xs hover:border-onyx/25 transition-all duration-200 group"
                      >
                        <div>
                          {/* Card Top Row */}
                          <div className="flex items-center justify-between mb-3.5">
                            <div className="w-10 h-10 rounded-full bg-soft-meadow flex items-center justify-center border border-onyx/10 group-hover:scale-105 transition-transform">
                              {getCategoryIcon(skill.category)}
                            </div>

                            <div className="flex items-center gap-1.5">
                              {/* Star Count Badge */}
                              {skill.stars ? (
                                <span className="inline-flex items-center gap-1 text-[11px] font-mono font-medium text-amber-900 bg-amber-500/10 px-2 py-0.5 rounded-full border border-amber-500/20">
                                  <Star className="w-3 h-3 fill-amber-500 text-amber-500" />
                                  {formatStars(skill.stars)}
                                </span>
                              ) : null}

                              {/* Multi-file badge */}
                              {fileCount > 1 ? (
                                <span className="inline-flex items-center gap-1 text-[11px] font-mono text-slate bg-soft-meadow px-2 py-0.5 rounded-full border border-onyx/5">
                                  <Layers className="w-3 h-3" />
                                  {t('card.multiFile', { count: fileCount })}
                                </span>
                              ) : null}

                              {/* Installed Badge */}
                              {isInstalled ? (
                                <Badge variant="active" className="flex items-center gap-1 text-[11px]">
                                  <CheckCircle2 className="w-3 h-3" /> {t('hub.installed', 'Installed')}
                                </Badge>
                              ) : null}
                            </div>
                          </div>

                          {/* Skill Name & Author */}
                          <div className="mb-1.5">
                            <h3 className="font-serif text-heading-sm text-deep-ink group-hover:text-onyx transition-colors">
                              {skill.name}
                            </h3>
                            <p className="text-[11px] text-slate font-sans truncate">
                              {t('hub.byAuthor', { author: skill.author || 'community' })}
                            </p>
                          </div>
                          <p className="font-sans text-body-sm text-slate mb-4 line-clamp-3">
                            {skill.description}
                          </p>

                          {/* Tags Pills */}
                          {skill.tags && skill.tags.length > 0 && (
                            <div className="flex flex-wrap gap-1.5 mb-4">
                              {skill.tags.slice(0, 4).map((tag) => (
                                <span
                                  key={tag}
                                  onClick={() => setSearchQuery(tag)}
                                  className="text-[11px] font-mono bg-soft-meadow px-2 py-0.5 rounded-full text-slate border border-onyx/5 hover:border-onyx/20 cursor-pointer transition-colors"
                                >
                                  #{tag}
                                </span>
                              ))}
                            </div>
                          )}
                        </div>

                        {/* Card Bottom Row: Inspect + Install Button */}
                        <div className="pt-3.5 border-t border-soft-meadow flex items-center justify-end gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            icon={<Eye className="w-3.5 h-3.5" />}
                            onClick={() => setInspectSkill(skill)}
                            title={t('card.viewDetails', 'Inspect details')}
                          >
                            {t('card.viewDetails', 'Details')}
                          </Button>

                          {isInstalled ? (
                            <Button
                              variant="danger"
                              size="sm"
                              icon={<Trash2 className="w-3.5 h-3.5" />}
                              onClick={() => setUninstallingSkill(skill.slug || skill.id)}
                              disabled={isExecuting(skill.slug || skill.id)}
                            >
                              {t('hub.uninstall', 'Uninstall')}
                            </Button>
                          ) : (
                            <Button
                              variant="primary"
                              size="sm"
                              icon={<Download className="w-3.5 h-3.5" />}
                              onClick={() => handleInstall(skill.slug || skill.id, skill.name)}
                              disabled={isExecuting(skill.slug || skill.id)}
                            >
                              {t('hub.install', 'Install')}
                            </Button>
                          )}
                        </div>
                      </Card>
                    );
                  })}
                </div>

                {/* Load More Pagination */}
                {pageSize < filteredHub.length && (
                  <div className="mt-10 text-center">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setPageSize((prev) => prev + 18)}
                      className="px-6 py-2 rounded-full border border-onyx/15 shadow-2xs font-medium"
                    >
                      {t('pagination.loadMore', 'Load More Skills')} ({t('pagination.showing', { shown: paginatedHub.length, total: filteredHub.length })})
                    </Button>
                  </div>
                )}
              </>
            )}
          </div>
        )}
      </PageContainer>

      {/* Skill Inspector Modal */}
      {inspectSkill && (
        <Modal
          isOpen={!!inspectSkill}
          onClose={() => setInspectSkill(null)}
          title={inspectSkill.name}
        >
          <div className="space-y-6 max-h-[75vh] overflow-y-auto pr-1">
            {/* Header info */}
            <div className="flex items-start justify-between gap-4 p-4 rounded-[16px] bg-soft-meadow border border-onyx/10">
              <div>
                <div className="flex items-center gap-2 mb-1.5">
                  <Badge variant="neutral" className="text-caption uppercase font-mono">
                    {inspectSkill.category}
                  </Badge>
                  {inspectSkill.stars ? (
                    <span className="inline-flex items-center gap-1 text-[11px] font-mono font-semibold text-amber-800 bg-amber-100 px-2 py-0.5 rounded-full">
                      <Star className="w-3 h-3 fill-amber-500 text-amber-500" />
                      {inspectSkill.stars.toLocaleString()} stars
                    </span>
                  ) : null}
                </div>
                <p className="font-sans text-body-sm text-deep-ink font-medium">
                  {t('detailModal.author')}: <span className="text-slate">{inspectSkill.author}</span>
                </p>
              </div>

              {inspectSkill.installed ? (
                <Badge variant="active" className="flex items-center gap-1">
                  <CheckCircle2 className="w-3.5 h-3.5" /> {t('hub.installed', 'Installed')}
                </Badge>
              ) : (
                <Badge variant="neutral">{t('hub.available', 'Available')}</Badge>
              )}
            </div>

            {/* Description */}
            <div>
              <h4 className="text-caption uppercase font-semibold text-slate tracking-wider mb-2">
                {t('detailModal.overview', 'Overview')}
              </h4>
              <p className="font-sans text-body-sm text-deep-ink leading-relaxed whitespace-pre-wrap bg-canvas p-4 rounded-[16px] border border-onyx/10">
                {inspectSkill.description}
              </p>
            </div>

            {/* Package Files */}
            {inspectSkill.files && inspectSkill.files.length > 0 && (
              <div>
                <h4 className="text-caption uppercase font-semibold text-slate tracking-wider mb-2">
                  {t('detailModal.filesList', { count: inspectSkill.files.length, defaultValue: `Package Files (${inspectSkill.files.length})` })}
                </h4>
                <div className="space-y-1.5 bg-canvas p-3 rounded-[16px] border border-onyx/10">
                  {inspectSkill.files.map((file, idx) => (
                    <div
                      key={idx}
                      className="flex items-center gap-2 text-caption font-mono text-deep-ink px-2.5 py-1.5 rounded-[8px] bg-soft-meadow/70 border border-onyx/5"
                    >
                      <FileText className="w-3.5 h-3.5 text-slate shrink-0" />
                      <span className="truncate">{file}</span>
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Tags */}
            {inspectSkill.tags && inspectSkill.tags.length > 0 && (
              <div>
                <h4 className="text-caption uppercase font-semibold text-slate tracking-wider mb-2">Tags</h4>
                <div className="flex flex-wrap gap-1.5">
                  {inspectSkill.tags.map((tag) => (
                    <span
                      key={tag}
                      className="text-caption font-mono bg-soft-meadow px-2.5 py-1 rounded-full text-slate border border-onyx/10"
                    >
                      #{tag}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Links & Source Code */}
            <div className="flex flex-wrap gap-3 pt-2">
              {inspectSkill.sourceGithubUrl && (
                <a
                  href={inspectSkill.sourceGithubUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-full text-caption font-sans font-medium bg-canvas border border-onyx/15 hover:border-onyx/40 text-deep-ink transition-colors"
                >
                  <Github className="w-3.5 h-3.5" />
                  {t('detailModal.openGithub', 'View on GitHub')}
                  <ExternalLink className="w-3 h-3 text-slate" />
                </a>
              )}
              {inspectSkill.skillUrl && (
                <a
                  href={inspectSkill.skillUrl}
                  target="_blank"
                  rel="noreferrer"
                  className="inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-full text-caption font-sans font-medium bg-canvas border border-onyx/15 hover:border-onyx/40 text-deep-ink transition-colors"
                >
                  <Sparkles className="w-3.5 h-3.5" />
                  {t('detailModal.openMarketplace', 'View on Marketplace')}
                  <ExternalLink className="w-3 h-3 text-slate" />
                </a>
              )}
            </div>

            {/* Actions in Modal */}
            <div className="pt-4 border-t border-onyx/10 flex items-center justify-end gap-3">
              {inspectSkill.installed ? (
                <Button
                  variant="danger"
                  size="sm"
                  icon={<Trash2 className="w-3.5 h-3.5" />}
                  onClick={() => setUninstallingSkill(inspectSkill.slug || inspectSkill.id)}
                  disabled={isExecuting(inspectSkill.slug || inspectSkill.id)}
                >
                  {t('hub.uninstall', 'Uninstall')}
                </Button>
              ) : (
                <Button
                  variant="primary"
                  size="sm"
                  icon={<Download className="w-3.5 h-3.5" />}
                  onClick={() => handleInstall(inspectSkill.slug || inspectSkill.id, inspectSkill.name)}
                  disabled={isExecuting(inspectSkill.slug || inspectSkill.id)}
                >
                  {t('hub.install', 'Install Skill')}
                </Button>
              )}
            </div>
          </div>
        </Modal>
      )}

      {/* Create Skill Modal */}
      <Modal
        isOpen={isCreateOpen}
        onClose={() => setIsCreateOpen(false)}
        title={t('createModal.title', 'Create New Skill')}
      >
        <form onSubmit={handleCreateSkill} className="space-y-4">
          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              {t('createModal.nameLabel', 'Skill Name')}
            </label>
            <Input
              placeholder={t('createModal.namePlaceholder', 'e.g. market_researcher')}
              value={skillName}
              onChange={(e) => setSkillName(e.target.value)}
              required
            />
          </div>
          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              {t('createModal.descLabel', 'Description')}
            </label>
            <Input
              placeholder={t('createModal.descPlaceholder')}
              value={skillDesc}
              onChange={(e) => setSkillDesc(e.target.value)}
            />
          </div>
          <div>
            <label className="text-caption uppercase text-slate font-semibold block mb-1">
              {t('createModal.contentLabel', 'Instructions (Markdown)')}
            </label>
            <textarea
              rows={6}
              value={skillContent}
              onChange={(e) => setSkillContent(e.target.value)}
              placeholder={t('createModal.contentPlaceholder')}
              className="w-full bg-canvas text-deep-ink font-mono text-body-sm p-4 rounded-[16px] border border-onyx/20 focus:outline-none focus:ring-2 focus:ring-deep-ink"
            />
          </div>
          <Button
            type="submit"
            variant="primary"
            disabled={isExecuting() || !skillName.trim()}
            className="w-full justify-center"
          >
            {isExecuting() ? t('createModal.creating', 'Creating...') : t('createModal.submit', 'Create Skill')}
          </Button>
        </form>
      </Modal>

      {/* Uninstall Confirmation Modal */}
      <ConfirmModal
        isOpen={!!uninstallingSkill}
        onClose={() => setUninstallingSkill(null)}
        onConfirm={handleConfirmUninstall}
        title={t('hub.uninstall', 'Uninstall Skill')}
        description={`Remove "${uninstallingSkill}" from your system? Agents will no longer have access to this skill.`}
        confirmLabel={t('hub.uninstall', 'Uninstall')}
        variant="danger"
      />
    </div>
  );
}

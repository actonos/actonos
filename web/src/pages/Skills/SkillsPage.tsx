import { useState, useEffect } from 'react';
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
  Cloud,
  Activity,
  CheckCircle2,
} from 'lucide-react';
import { api, type HubSkillItem } from '@/lib/api';
import { isApprovalRequired, type ToolInfo } from '@/lib/types';

export function SkillsPage() {
  const { t } = useTranslation('skills');
  const { success, error, info } = useToast();
  const [installedSkills, setInstalledSkills] = useState<ToolInfo[]>([]);
  const [hubCatalog, setHubCatalog] = useState<HubSkillItem[]>([]);
  const [activeTab, setActiveTab] = useState<'installed' | 'hub'>(() => readHashParams().get('view') === 'catalog' ? 'hub' : 'installed');
  const selectTab = (tab: 'installed' | 'hub') => {
    setActiveTab(tab);
    setHashParam('view', tab === 'installed' ? undefined : 'catalog');
  };
  const [searchQuery, setSearchQuery] = useState('');
  const [loading, setLoading] = useState(true);
  const [installingId, setInstallingId] = useState<string | null>(null);
  const [uninstallingSkill, setUninstallingSkill] = useState<string | null>(null);

  // Create skill form
  const [isCreateOpen, setIsCreateOpen] = useState(false);
  const [skillName, setSkillName] = useState('');
  const [skillDesc, setSkillDesc] = useState('');
  const [skillContent, setSkillContent] = useState('');
  const [creating, setCreating] = useState(false);

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
  }, []);

  const handleInstall = async (skillId: string) => {
    setInstallingId(skillId);
    try {
      const result = await api.installHubSkill(skillId);
      if (isApprovalRequired(result)) {
        info(t('common:approval.queuedTitle'), t('common:approval.queuedDescription'));
        return;
      }
      success(t('hub.installed', 'Installed'), `Skill ${skillId} is now active.`);
      loadData();
    } catch (err) {
      error('Install failed', getErrorMessage(err));
    } finally {
      setInstallingId(null);
    }
  };

  const handleConfirmUninstall = async () => {
    if (!uninstallingSkill) return;
    setInstallingId(uninstallingSkill);
    try {
      const result = await api.uninstallHubSkill(uninstallingSkill);
      if (isApprovalRequired(result)) {
        info(t('common:approval.queuedTitle'), t('common:approval.queuedDescription'));
        setUninstallingSkill(null);
        return;
      }
      success(t('hub.uninstall', 'Uninstalled'), `Skill ${uninstallingSkill} removed.`);
      setUninstallingSkill(null);
      loadData();
    } catch (err) {
      error('Uninstall failed', getErrorMessage(err));
    } finally {
      setInstallingId(null);
    }
  };

  const handleCreateSkill = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!skillName.trim()) return;
    setCreating(true);
    try {
      const result = await api.createSkill({
        name: skillName.trim().toLowerCase().replace(/\s+/g, '_'),
        description: skillDesc.trim(),
        content: skillContent.trim(),
      });
      if (isApprovalRequired(result)) {
        info(t('common:approval.queuedTitle'), t('common:approval.queuedDescription'));
        setIsCreateOpen(false);
        return;
      }
      success(t('createModal.submit', 'Created'), `Skill ${skillName} created.`);
      setIsCreateOpen(false);
      setSkillName('');
      setSkillDesc('');
      setSkillContent('');
      loadData();
    } catch (err) {
      error('Create failed', getErrorMessage(err));
    } finally {
      setCreating(false);
    }
  };

  const filteredInstalled = installedSkills.filter(
    (s) =>
      s.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      s.description.toLowerCase().includes(searchQuery.toLowerCase())
  );

  const filteredHub = hubCatalog.filter(
    (s) =>
      s.name.toLowerCase().includes(searchQuery.toLowerCase()) ||
      s.description.toLowerCase().includes(searchQuery.toLowerCase()) ||
      s.tags?.some((tg) => tg.toLowerCase().includes(searchQuery.toLowerCase()))
  );

  const getHubIcon = (iconName: string) => {
    switch (iconName) {
      case 'github':
        return <Github className="w-5 h-5 text-deep-ink" />;
      case 'cloud':
        return <Cloud className="w-5 h-5 text-deep-ink" />;
      case 'activity':
        return <Activity className="w-5 h-5 text-deep-ink" />;
      default:
        return <Sparkles className="w-5 h-5 text-deep-ink" />;
    }
  };

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        <PageHeader eyebrow={t('eyebrow')} title={t('title')} description={t('subtitle')} actions={(
          <Button variant="ghost" size="sm" icon={<RefreshCw />} onClick={loadData}>{t('actions.refresh')}</Button>
        )} />
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Skills')}
            </span>
            <h1 className="hidden font-serif text-heading-lg text-deep-ink tracking-tight" aria-hidden="true">
              {t('title', 'Agent Skills')}
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t('subtitle', 'Install and manage skills for agents. Explore community skills.')}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button variant="ghost" size="sm" icon={<RefreshCw className="w-3.5 h-3.5" />} onClick={loadData}>
              {t('actions.refresh', 'Refresh')}
            </Button>
            <Button
              variant="primary"
              size="sm"
              icon={<Plus className="w-3.5 h-3.5" />}
              onClick={() => setIsCreateOpen(true)}
            >
              {t('actions.newSkill', 'Create Skill')}
            </Button>
          </div>
        </div>

        {/* Tabs & Search */}
        <div className="flex flex-col sm:flex-row items-center justify-between gap-4 mb-8">
          <div className="flex items-center gap-1.5 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs self-start sm:self-auto">
            <button
              onClick={() => selectTab('installed')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${
                activeTab === 'installed'
                  ? 'bg-deep-ink text-white font-semibold shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              {t('tabs.installed', { count: installedSkills.length })}
            </button>
            <button
              onClick={() => selectTab('hub')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer ${
                activeTab === 'hub'
                  ? 'bg-deep-ink text-white font-semibold shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              {t('tabs.hub', { count: hubCatalog.length })}
            </button>
          </div>

          <div className="relative max-w-xs w-full">
            <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder={t('actions.search', 'Search skills...')}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="w-full bg-soft-meadow text-deep-ink pl-10 pr-4 py-1.5 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink"
            />
          </div>
        </div>

        {/* Installed Skills View */}
        {activeTab === 'installed' && (
          loading ? (
            <div className="py-20 text-center text-slate font-sans">{t('loading')}</div>
          ) : filteredInstalled.length === 0 ? (
            <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-md mx-auto border border-onyx/10">
              <Sparkles className="w-12 h-12 text-deep-ink mx-auto mb-3 opacity-40" />
              <p className="font-sans text-body-sm text-slate mb-2 font-semibold">{t('empty.title', 'No skills found')}</p>
              <p className="font-sans text-caption text-slate mb-4">{t('empty.subtitle')}</p>
              <Button variant="primary" size="sm" onClick={() => selectTab('hub')}>
                {t('tabs.hub', { count: hubCatalog.length })}
              </Button>
            </div>
          ) : (
            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {filteredInstalled.map((skill) => (
                <Card key={skill.name} className="p-6 border border-onyx/10 flex flex-col justify-between bg-canvas/90">
                  <div>
                    <div className="flex items-center justify-between mb-3">
                      <div className="w-10 h-10 rounded-full bg-soft-meadow flex items-center justify-center border border-onyx/10">
                        <Sparkles className="w-5 h-5 text-deep-ink" />
                      </div>
                      <Badge variant="active" className="flex items-center gap-1">
                        <CheckCircle2 className="w-3 h-3" /> {t('hub.installed', 'Installed')}
                      </Badge>
                    </div>
                    <h3 className="font-serif text-heading-sm text-deep-ink mb-1">{skill.name}</h3>
                    <p className="font-sans text-body-sm text-slate">{skill.description || 'Custom skill'}</p>
                  </div>
                  <div className="pt-4 mt-4 border-t border-soft-meadow flex items-center justify-between">
                    <span className="text-caption text-slate font-mono">{skill.category}</span>
                  </div>
                </Card>
              ))}
            </div>
          )
        )}

        {/* Community Hub View */}
        {activeTab === 'hub' && (
          <div>
            <div className="mb-4 flex items-center justify-between">
              <span className="text-caption font-semibold uppercase tracking-wider text-slate">
                {t('hub.title', 'Community Skills')}
              </span>
              <span className="text-caption text-slate">
                {t('hub.subtitle', '1-click install')}
              </span>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
              {filteredHub.map((skill) => (
                <Card key={skill.id} className="p-6 border border-onyx/10 flex flex-col justify-between bg-canvas/90 shadow-xs">
                  <div>
                    <div className="flex items-center justify-between mb-3">
                      <div className="w-10 h-10 rounded-full bg-soft-meadow flex items-center justify-center border border-onyx/10">
                        {getHubIcon(skill.icon)}
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant="neutral" className="text-caption font-mono">
                          v{skill.version}
                        </Badge>
                        {skill.installed ? (
                          <Badge variant="active" className="flex items-center gap-1">
                            <CheckCircle2 className="w-3 h-3" /> {t('hub.installed', 'Installed')}
                          </Badge>
                        ) : (
                          <Badge variant="neutral">{t('hub.available', 'Available')}</Badge>
                        )}
                      </div>
                    </div>

                    <h3 className="font-serif text-heading-sm text-deep-ink mb-1">{skill.name}</h3>
                    <p className="font-sans text-body-sm text-slate mb-4">{skill.description}</p>

                    <div className="flex flex-wrap gap-1.5 mb-4">
                      {skill.tags?.map((tag) => (
                        <span key={tag} className="text-caption font-mono bg-soft-meadow px-2 py-0.5 rounded-full text-slate border border-onyx/5">
                          #{tag}
                        </span>
                      ))}
                    </div>
                  </div>

                  <div className="pt-4 border-t border-soft-meadow flex items-center justify-between">
                    <span className="text-caption text-slate">{t('hub.byAuthor', { author: skill.author })}</span>
                    {skill.installed ? (
                      <Button
                        variant="danger"
                        size="sm"
                        icon={<Trash2 className="w-3.5 h-3.5" />}
                        onClick={() => setUninstallingSkill(skill.id)}
                        disabled={installingId === skill.id}
                      >
                        {installingId === skill.id ? t('hub.uninstalling', 'Uninstalling...') : t('hub.uninstall', 'Uninstall')}
                      </Button>
                    ) : (
                      <Button
                        variant="primary"
                        size="sm"
                        icon={<Download className="w-3.5 h-3.5" />}
                        onClick={() => handleInstall(skill.id)}
                        disabled={installingId === skill.id}
                      >
                        {installingId === skill.id ? t('hub.installing', 'Installing...') : t('hub.install', 'Install')}
                      </Button>
                    )}
                  </div>
                </Card>
              ))}
            </div>
          </div>
        )}
      </PageContainer>

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
            disabled={creating || !skillName.trim()}
            className="w-full justify-center"
          >
            {creating ? t('createModal.creating', 'Creating...') : t('createModal.submit', 'Create Skill')}
          </Button>
        </form>
      </Modal>

      {/* Uninstall Confirmation */}
      <ConfirmModal
        isOpen={!!uninstallingSkill}
        onClose={() => setUninstallingSkill(null)}
        onConfirm={handleConfirmUninstall}
        title={t('hub.uninstall', 'Uninstall Skill')}
        description={`Remove "${uninstallingSkill}" from your system?`}
        confirmLabel={t('hub.uninstall', 'Uninstall')}
        variant="danger"
      />
    </div>
  );
}

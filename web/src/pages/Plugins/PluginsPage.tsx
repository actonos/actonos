import { useState, useEffect, useMemo, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { PluginInfo, RegistryPlugin } from '@/lib/types';
import { readHashParams, setHashParam } from '@/lib/url-state';
import { useActionProgress } from '@/lib/useActionProgress';
import {
  Boxes,
  Activity,
  Plus,
  RefreshCw,
  Search,
  X,
  Radio,
  Network,
  Wrench,
  Globe,
  Key,
  Database,
  Terminal,
  Shield,
  Trash2,
  Bot,
  Sliders,
  Download,
  CheckCircle2,
  Star,
  Eye,
  Sparkles,
} from 'lucide-react';
import { PluginUploadModal } from './PluginUploadModal';
import { PluginDetailModal } from './PluginDetailModal';
import { PluginLogsModal } from './PluginLogsModal';
import { PluginHubDetailModal } from './PluginHubDetailModal';
import { getErrorMessage } from '@/lib/errors';

export function PluginsPage() {
  const { t } = useTranslation('plugins');
  const { success, error, info } = useToast();
  const { executeAction, isExecuting } = useActionProgress();

  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [hubCatalog, setHubCatalog] = useState<RegistryPlugin[]>([]);
  const [loading, setLoading] = useState(true);

  // Primary view switcher ('installed' vs 'available')
  const [activeView, setActiveView] = useState<'installed' | 'available'>(() =>
    readHashParams().get('view') === 'available' ? 'available' : 'installed'
  );

  const selectView = (view: 'installed' | 'available') => {
    setActiveView(view);
    setHashParam('view', view === 'installed' ? undefined : 'available');
  };

  // Search & Filters
  const [searchQuery, setSearchQuery] = useState('');
  const [installedCategory, setInstalledCategory] = useState<'all' | 'channel' | 'connector' | 'tool'>('all');
  const [installedStatusFilter, setInstalledStatusFilter] = useState<'all' | 'running' | 'stopped' | 'error'>('all');

  const [hubCategory, setHubCategory] = useState<string>('all');
  const [hubStatusFilter, setHubStatusFilter] = useState<'all' | 'installed' | 'available'>('all');
  const [hubSortBy, setHubSortBy] = useState<'stars' | 'recent' | 'name'>('stars');
  const [hubPageSize, setHubPageSize] = useState<number>(18);

  const [togglingID, setTogglingID] = useState<string | null>(null);

  // Modals state
  const [isUploadOpen, setIsUploadOpen] = useState(false);
  const [selectedPlugin, setSelectedPlugin] = useState<PluginInfo | null>(null);
  const [detailInitialTab, setDetailInitialTab] = useState<'overview' | 'config' | 'tools' | 'raw'>('overview');
  const [isDetailOpen, setIsDetailOpen] = useState(false);
  const [isLogsOpen, setIsLogsOpen] = useState(false);
  const [inspectHubPlugin, setInspectHubPlugin] = useState<RegistryPlugin | null>(null);

  const fetchData = useCallback(async () => {
    try {
      setLoading(true);
      const [pluginsRes, hubRes] = await Promise.all([
        api.listPlugins().catch(() => ({ plugins: [], count: 0 })),
        api.listAvailablePlugins().catch(() => ({ catalog: [], count: 0 })),
      ]);
      setPlugins(pluginsRes.plugins || []);
      setHubCatalog(hubRes.catalog || []);
    } catch (err) {
      error(t('errors.loadFailed'), getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [error, t]);

  useEffect(() => {
    fetchData();

    const handleUpdates = () => {
      fetchData();
    };

    window.addEventListener('actonos:plugins-updated', handleUpdates);
    window.addEventListener('actonos:tools-updated', handleUpdates);
    window.addEventListener('actonos:approval-decided', handleUpdates);

    return () => {
      window.removeEventListener('actonos:plugins-updated', handleUpdates);
      window.removeEventListener('actonos:tools-updated', handleUpdates);
      window.removeEventListener('actonos:approval-decided', handleUpdates);
    };
  }, [fetchData]);

  // Toggle active/inactive
  const handleToggle = async (plugin: PluginInfo) => {
    const id = plugin.manifest.id;
    const shouldEnable = plugin.status !== 'running';
    setTogglingID(id);
    try {
      if (shouldEnable) {
        await api.enablePlugin(id);
        success(t('actions.enable'), t('actions.enabledSuccess', { name: plugin.manifest.name || id }));
      } else {
        await api.disablePlugin(id);
        info(t('actions.disable'), t('actions.disabledSuccess', { name: plugin.manifest.name || id }));
      }
      await fetchData();
    } catch (err) {
      error(t('errors.toggleFailed'), getErrorMessage(err));
    } finally {
      setTogglingID(null);
    }
  };

  // Uninstall plugin
  const handleUninstall = async (pluginId: string, displayName?: string) => {
    const name = displayName || pluginId;
    if (!window.confirm(t('actions.confirmUninstall', { name }))) {
      return;
    }
    try {
      await api.deletePlugin(pluginId);
      success(t('actions.uninstall'), t('actions.uninstalledSuccess', { name }));
      if (selectedPlugin?.manifest.id === pluginId) {
        setIsDetailOpen(false);
        setIsLogsOpen(false);
      }
      if (inspectHubPlugin?.id === pluginId) {
        setInspectHubPlugin((prev) => (prev ? { ...prev, installed: false } : null));
      }
      await fetchData();
    } catch (err) {
      error(t('errors.uninstallFailed'), getErrorMessage(err));
    }
  };

  // 1-Click Install from Official Registry Hub
  const handleInstallFromHub = (pluginId: string, displayName?: string, downloadUrl?: string) => {
    const targetName = displayName || pluginId;
    executeAction({
      targetId: pluginId,
      title: t('hub.installingTitle', { name: targetName, defaultValue: `Installing Plugin: ${targetName}` }),
      subtitle: t('hub.installingSubtitle'),
      steps: [
        { id: 'auth', label: t('actionProgress.steps.auth') },
        { id: 'download', label: t('actionProgress.steps.download') },
        { id: 'verify', label: t('actionProgress.steps.verify') },
      ],
      action: () => api.installAvailablePlugin(pluginId, downloadUrl),
      onSuccess: () => {
        success(t('hub.installed'), t('hub.installedSuccessDesc', { name: targetName }));
        fetchData();
        if (inspectHubPlugin?.id === pluginId) {
          setInspectHubPlugin((prev) => (prev ? { ...prev, installed: true } : null));
        }
      },
    });
  };

  // Metrics
  const stats = useMemo(() => {
    const total = plugins.length;
    const active = plugins.filter((p) => p.status === 'running').length;
    const channels = plugins.filter((p) => p.manifest.capabilities?.includes('channel')).length;
    const connectors = plugins.filter((p) => p.manifest.capabilities?.includes('connector')).length;
    const tools = plugins.filter((p) => p.manifest.capabilities?.includes('tool')).length;
    return { total, active, channels, connectors, tools };
  }, [plugins]);

  // Filtered Installed Plugins
  const filteredInstalled = useMemo(() => {
    return plugins.filter((p) => {
      // Tab filter
      if (installedCategory !== 'all') {
        const caps = p.manifest.capabilities || [];
        if (!caps.includes(installedCategory)) return false;
      }
      // Status filter
      if (installedStatusFilter !== 'all') {
        if (installedStatusFilter === 'running' && p.status !== 'running') return false;
        if (installedStatusFilter === 'stopped' && p.status !== 'stopped' && p.status !== 'disabled') return false;
        if (installedStatusFilter === 'error' && p.status !== 'error') return false;
      }
      // Search query
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const id = p.manifest.id.toLowerCase();
        const name = (p.manifest.name || '').toLowerCase();
        const desc = (p.manifest.description || '').toLowerCase();
        const author = (p.manifest.author || '').toLowerCase();
        const domains = (p.manifest.permissions?.net_outbound || []).join(' ').toLowerCase();
        const toolsList = (p.manifest.tools || []).map((t) => t.name).join(' ').toLowerCase();
        return id.includes(q) || name.includes(q) || desc.includes(q) || author.includes(q) || domains.includes(q) || toolsList.includes(q);
      }
      return true;
    });
  }, [plugins, installedCategory, installedStatusFilter, searchQuery]);

  // Distinct categories with counts for Available Registry Hub
  const hubCategoryCounts = useMemo(() => {
    const counts: Record<string, number> = { all: hubCatalog.length };
    for (const item of hubCatalog) {
      const caps = item.capabilities || [item.category || 'tool'];
      for (const cap of caps) {
        counts[cap] = (counts[cap] || 0) + 1;
      }
    }
    return counts;
  }, [hubCatalog]);

  const availableHubCategories = useMemo(() => {
    const standard = ['all', 'channel', 'connector', 'tool'];
    const extra = Object.keys(hubCategoryCounts).filter((c) => !standard.includes(c));
    return [...standard, ...extra];
  }, [hubCategoryCounts]);

  // Filtered Available Registry Catalog
  const filteredHub = useMemo(() => {
    let list = hubCatalog.filter((p) => {
      // Category filter
      if (hubCategory !== 'all') {
        const caps = p.capabilities || [p.category || 'tool'];
        if (!caps.includes(hubCategory) && p.category !== hubCategory) return false;
      }

      // Status filter
      if (hubStatusFilter === 'installed' && !p.installed) return false;
      if (hubStatusFilter === 'available' && p.installed) return false;

      // Search query
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const id = p.id.toLowerCase();
        const name = (p.name || '').toLowerCase();
        const desc = (p.description || '').toLowerCase();
        const author = (p.author || '').toLowerCase();
        const tags = (p.tags || []).join(' ').toLowerCase();
        const domains = (p.permissions?.net_outbound || []).join(' ').toLowerCase();
        const toolsList = (p.tools || []).map((t) => t.name).join(' ').toLowerCase();
        return (
          id.includes(q) ||
          name.includes(q) ||
          desc.includes(q) ||
          author.includes(q) ||
          tags.includes(q) ||
          domains.includes(q) ||
          toolsList.includes(q)
        );
      }
      return true;
    });

    // Sorting
    list = [...list].sort((a, b) => {
      if (hubSortBy === 'stars') {
        return (b.stars || 0) - (a.stars || 0);
      }
      if (hubSortBy === 'recent') {
        return (b.version || '').localeCompare(a.version || '');
      }
      return (a.name || a.id).localeCompare(b.name || b.id);
    });

    return list;
  }, [hubCatalog, hubCategory, hubStatusFilter, hubSortBy, searchQuery]);

  const paginatedHub = useMemo(() => {
    return filteredHub.slice(0, hubPageSize);
  }, [filteredHub, hubPageSize]);

  const getCapabilityIcon = (capabilities?: string[]) => {
    const caps = capabilities || ['tool'];
    if (caps.includes('channel')) {
      return <Radio className="w-6 h-6" />;
    }
    if (caps.includes('connector')) {
      return <Network className="w-6 h-6" />;
    }
    return <Wrench className="w-6 h-6" />;
  };

  const getCapabilityBadgeStyle = (cap: string) => {
    if (cap === 'channel') return 'bg-purple-500/15 text-purple-700';
    if (cap === 'connector') return 'bg-blue-500/15 text-blue-700';
    return 'bg-emerald-500/15 text-emerald-700';
  };

  return (
    <div className="relative min-h-[calc(100vh-64px)] pb-16">
      <PageContainer maxWidth="wide">
        {/* Header */}
        <PageHeader
          eyebrow={t('eyebrow')}
          title={t('title')}
          description={t('subtitle')}
          actions={
            <div className="flex flex-wrap items-center gap-3">
              <Button
                variant="ghost"
                size="sm"
                icon={<RefreshCw className={loading ? 'animate-spin' : ''} />}
                onClick={fetchData}
              >
                {t('actions.refresh', 'Refresh')}
              </Button>
              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="w-4 h-4" />}
                onClick={() => setIsUploadOpen(true)}
              >
                {t('upload', 'Upload Plugin')}
              </Button>
            </div>
          }
        />

        {/* 4 Metric Cards */}
        <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 mb-8">
          <Card className="p-5 border border-onyx/10 bg-soft-meadow/80 rounded-[22px] shadow-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-caption font-semibold uppercase tracking-wider text-slate">
                  {t('stats.total', 'Total Installed')}
                </p>
                <h3 className="font-serif text-heading font-bold text-deep-ink mt-1">{stats.total}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-deep-ink/5 text-deep-ink flex items-center justify-center">
                <Boxes className="w-6 h-6" />
              </div>
            </div>
            <p className="text-caption text-slate mt-2 flex items-center gap-1.5">
              <span>{t('stats.toolsActive', { count: stats.tools, defaultValue: '{{count}} Tool modules active' })}</span>
            </p>
          </Card>

          <Card className="p-5 border border-onyx/10 bg-soft-meadow/80 rounded-[22px] shadow-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-caption font-semibold uppercase tracking-wider text-slate">
                  {t('stats.active', 'Active & Running')}
                </p>
                <h3 className="font-serif text-heading font-bold text-emerald-600 mt-1">{stats.active}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-emerald-500/10 text-emerald-600 flex items-center justify-center">
                <Activity className="w-6 h-6" />
              </div>
            </div>
            <p className="text-caption text-slate mt-2 flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full bg-emerald-500" />
              <span>{t('stats.jitSafe', 'JIT hot-reloaded memory safe')}</span>
            </p>
          </Card>

          <Card className="p-5 border border-onyx/10 bg-soft-meadow/80 rounded-[22px] shadow-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-caption font-semibold uppercase tracking-wider text-slate">
                  {t('stats.channels', 'Chat Channels')}
                </p>
                <h3 className="font-serif text-heading font-bold text-purple-600 mt-1">{stats.channels}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-purple-500/10 text-purple-600 flex items-center justify-center">
                <Radio className="w-6 h-6" />
              </div>
            </div>
            <p className="text-caption text-slate mt-2 flex items-center gap-1.5">
              <span>{t('stats.adaptersActive', 'Dynamic messaging adapters')}</span>
            </p>
          </Card>

          <Card className="p-5 border border-onyx/10 bg-soft-meadow/80 rounded-[22px] shadow-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-caption font-semibold uppercase tracking-wider text-slate">
                  {t('stats.connectors', 'SaaS Connectors')}
                </p>
                <h3 className="font-serif text-heading font-bold text-blue-600 mt-1">{stats.connectors}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-blue-500/10 text-blue-600 flex items-center justify-center">
                <Network className="w-6 h-6" />
              </div>
            </div>
            <p className="text-caption text-slate mt-2 flex items-center gap-1.5">
              <span>{t('stats.vaultBrokered', 'Vault token brokered')}</span>
            </p>
          </Card>
        </div>

        {/* Primary View Switcher & Search Bar */}
        <div className="flex flex-col md:flex-row items-stretch md:items-center justify-between gap-4 mb-6">
          {/* Main Views Segment: Installed vs Available */}
          <div className="flex items-center gap-1.5 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 shadow-xs self-start">
            <button
              onClick={() => selectView('installed')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer flex items-center gap-2 ${
                activeView === 'installed'
                  ? 'bg-deep-ink text-white font-semibold shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              <Boxes className="w-3.5 h-3.5" />
              <span>{t('views.installed', { count: plugins.length })}</span>
            </button>

            <button
              onClick={() => selectView('available')}
              className={`px-4 py-1.5 rounded-full text-caption font-sans font-medium transition-all cursor-pointer flex items-center gap-2 ${
                activeView === 'available'
                  ? 'bg-deep-ink text-white font-semibold shadow-xs'
                  : 'text-deep-ink hover:text-slate'
              }`}
            >
              <Sparkles className="w-3.5 h-3.5" />
              <span>{t('views.available', { count: hubCatalog.length })}</span>
            </button>
          </div>

          {/* Search Box */}
          <div className="relative w-full md:max-w-md">
            <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
            <input
              type="text"
              placeholder={t('search.placeholder', 'Search plugins by name, ID, capability, domain, author...')}
              value={searchQuery}
              onChange={(e) => {
                setSearchQuery(e.target.value);
                setHubPageSize(18);
              }}
              className="w-full bg-soft-meadow text-deep-ink pl-10 pr-9 py-2 rounded-full border border-onyx/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink transition-all"
            />
            {searchQuery && (
              <button
                onClick={() => setSearchQuery('')}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink cursor-pointer"
                title={t('actions.clearSearch', 'Clear search')}
              >
                <X className="w-3.5 h-3.5" />
              </button>
            )}
          </div>
        </div>

        {/* View 1: Installed Plugins Tab */}
        {activeView === 'installed' && (
          <div>
            {/* Installed Categories & Status Filter Row */}
            <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between gap-4 mb-6">
              <div className="flex items-center gap-1 overflow-x-auto max-w-full pb-1 scrollbar-none">
                <button
                  onClick={() => setInstalledCategory('all')}
                  className={`px-3.5 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-1.5 border ${
                    installedCategory === 'all'
                      ? 'bg-deep-ink text-white border-deep-ink shadow-xs'
                      : 'bg-canvas/90 text-deep-ink border-onyx/10 hover:border-onyx/30 hover:bg-soft-meadow'
                  }`}
                >
                  <Boxes className="w-3 h-3" />
                  <span>{t('tabs.all', 'All')}</span>
                  <span className={`px-1.5 py-0.2 text-[10px] rounded-full font-mono ${installedCategory === 'all' ? 'bg-white/20' : 'bg-soft-meadow text-slate'}`}>
                    {stats.total}
                  </span>
                </button>

                <button
                  onClick={() => setInstalledCategory('channel')}
                  className={`px-3.5 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-1.5 border ${
                    installedCategory === 'channel'
                      ? 'bg-purple-600 text-white border-purple-600 shadow-xs'
                      : 'bg-canvas/90 text-deep-ink border-onyx/10 hover:border-onyx/30 hover:bg-soft-meadow'
                  }`}
                >
                  <Radio className="w-3 h-3" />
                  <span>{t('tabs.channel', 'Chat Channels')}</span>
                  <span className={`px-1.5 py-0.2 text-[10px] rounded-full font-mono ${installedCategory === 'channel' ? 'bg-white/20' : 'bg-soft-meadow text-slate'}`}>
                    {stats.channels}
                  </span>
                </button>

                <button
                  onClick={() => setInstalledCategory('connector')}
                  className={`px-3.5 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-1.5 border ${
                    installedCategory === 'connector'
                      ? 'bg-blue-600 text-white border-blue-600 shadow-xs'
                      : 'bg-canvas/90 text-deep-ink border-onyx/10 hover:border-onyx/30 hover:bg-soft-meadow'
                  }`}
                >
                  <Network className="w-3 h-3" />
                  <span>{t('tabs.connector', 'SaaS Connectors')}</span>
                  <span className={`px-1.5 py-0.2 text-[10px] rounded-full font-mono ${installedCategory === 'connector' ? 'bg-white/20' : 'bg-soft-meadow text-slate'}`}>
                    {stats.connectors}
                  </span>
                </button>

                <button
                  onClick={() => setInstalledCategory('tool')}
                  className={`px-3.5 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-1.5 border ${
                    installedCategory === 'tool'
                      ? 'bg-emerald-600 text-white border-emerald-600 shadow-xs'
                      : 'bg-canvas/90 text-deep-ink border-onyx/10 hover:border-onyx/30 hover:bg-soft-meadow'
                  }`}
                >
                  <Wrench className="w-3 h-3" />
                  <span>{t('tabs.tool', 'Tool Extensions')}</span>
                  <span className={`px-1.5 py-0.2 text-[10px] rounded-full font-mono ${installedCategory === 'tool' ? 'bg-white/20' : 'bg-soft-meadow text-slate'}`}>
                    {stats.tools}
                  </span>
                </button>
              </div>

              <select
                value={installedStatusFilter}
                onChange={(e) => setInstalledStatusFilter(e.target.value as 'all' | 'running' | 'stopped' | 'error')}
                className="bg-soft-meadow text-deep-ink px-3.5 py-1.5 rounded-full border border-onyx/10 text-caption font-semibold focus:outline-none cursor-pointer self-start sm:self-auto"
              >
                <option value="all">{t('search.statusFilter', 'All Statuses')}</option>
                <option value="running">{t('status.running', 'Running')}</option>
                <option value="stopped">{t('status.stopped', 'Stopped')}</option>
                <option value="error">{t('status.error', 'Error')}</option>
              </select>
            </div>

            {loading ? (
              <div className="py-20 text-center text-slate font-sans flex flex-col items-center justify-center gap-3">
                <RefreshCw className="w-8 h-8 animate-spin text-deep-ink" />
                <p className="text-body-sm font-semibold">{t('loading', 'Loading WASM Plugins...')}</p>
              </div>
            ) : filteredInstalled.length > 0 ? (
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mb-12">
                {filteredInstalled.map((plugin) => {
                  const manifest = plugin.manifest;
                  const isRunning = plugin.status === 'running';
                  const isToggling = togglingID === manifest.id;
                  const capabilities = manifest.capabilities || ['tool'];
                  const permissions = manifest.permissions || {};
                  const tools = manifest.tools || [];

                  return (
                    <Card
                      key={manifest.id}
                      className="relative flex flex-col justify-between p-6 rounded-[24px] border border-onyx/10 bg-soft-meadow/90 hover:shadow-md transition-all group"
                    >
                      <div>
                        {/* Top Header with Icon & Active Toggle Switch */}
                        <div className="flex items-start justify-between gap-3 mb-3">
                          <div className="flex items-center gap-3">
                            <div
                              className={`w-12 h-12 rounded-2xl flex items-center justify-center shrink-0 ${
                                capabilities.includes('channel')
                                  ? 'bg-purple-500/15 text-purple-600'
                                  : capabilities.includes('connector')
                                    ? 'bg-blue-500/15 text-blue-600'
                                    : 'bg-emerald-500/15 text-emerald-600'
                              }`}
                            >
                              {getCapabilityIcon(capabilities)}
                            </div>

                            <div>
                              <div className="flex items-center gap-2">
                                <h3 className="font-serif text-subheading font-bold text-deep-ink leading-tight">
                                  {manifest.name || manifest.id}
                                </h3>
                              </div>
                              <p className="font-mono text-caption text-slate mt-0.5">@{manifest.id}</p>
                            </div>
                          </div>

                          {/* Toggle Switch */}
                          <button
                            type="button"
                            onClick={() => handleToggle(plugin)}
                            disabled={isToggling}
                            title={isRunning ? t('actions.disable') : t('actions.enable')}
                            className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${
                              isRunning ? 'bg-emerald-500' : 'bg-onyx/20'
                            } ${isToggling ? 'opacity-50 cursor-not-allowed' : ''}`}
                          >
                            <span
                              className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out ${
                                isRunning ? 'translate-x-5' : 'translate-x-0'
                              }`}
                            />
                          </button>
                        </div>

                        {/* Description */}
                        <p className="text-caption text-slate line-clamp-2 mb-4 leading-relaxed">
                          {manifest.description || t('empty.noDescription')}
                        </p>

                        {/* Capability Tags */}
                        <div className="flex flex-wrap gap-1.5 mb-4">
                          {capabilities.map((cap) => (
                            <span
                              key={cap}
                              className={`px-2.5 py-1 rounded-md text-[11px] font-semibold uppercase tracking-wider ${getCapabilityBadgeStyle(
                                cap
                              )}`}
                            >
                              {t(`capabilities.${cap}`, cap)}
                            </span>
                          ))}

                          {tools.length > 0 && (
                            <span className="px-2.5 py-1 rounded-md bg-amber-500/15 text-amber-800 text-[11px] font-semibold flex items-center gap-1">
                              <Bot className="w-3 h-3" />
                              <span>{t('stats.toolsCount', { count: tools.length, defaultValue: '{{count}} Tools' })}</span>
                            </span>
                          )}
                        </div>

                        {/* Permissions / Sandbox Scope Summary */}
                        <div className="p-3 rounded-2xl bg-canvas/60 border border-onyx/5 space-y-1.5 text-caption font-mono mb-4">
                          <div className="flex items-center justify-between text-slate">
                            <span className="flex items-center gap-1.5">
                              <Globe className="w-3.5 h-3.5 text-slate" /> {t('permissions.netOutbound', 'Egress')}:
                            </span>
                            <span className="truncate max-w-[170px] text-deep-ink">
                              {permissions.net_outbound?.length ? permissions.net_outbound.join(', ') : t('hub.modal.noEgress', 'Blocked (None)')}
                            </span>
                          </div>

                          <div className="flex items-center justify-between text-slate">
                            <span className="flex items-center gap-1.5">
                              <Key className="w-3.5 h-3.5 text-slate" /> {t('permissions.secrets', 'Vault')}:
                            </span>
                            <span className="truncate max-w-[170px] text-deep-ink">
                              {permissions.secrets?.length ? permissions.secrets.join(', ') : t('hub.modal.noSecrets', 'None')}
                            </span>
                          </div>

                          {permissions.storage && (
                            <div className="flex items-center justify-between text-slate">
                              <span className="flex items-center gap-1.5">
                                <Database className="w-3.5 h-3.5 text-slate" /> {t('permissions.storage', 'Storage')}:
                              </span>
                              <span className="text-emerald-600 font-semibold">{t('hub.modal.storageGranted', 'Enabled')}</span>
                            </div>
                          )}
                        </div>
                      </div>

                      {/* Actions Footer */}
                      <div className="flex items-center justify-between pt-3 border-t border-onyx/10">
                        <div className="flex items-center gap-1.5 flex-wrap">
                          <button
                            type="button"
                            onClick={() => {
                              setSelectedPlugin(plugin);
                              setDetailInitialTab('config');
                              setIsDetailOpen(true);
                            }}
                            className="px-3 py-1.5 rounded-full bg-deep-ink text-white text-caption font-semibold transition-all flex items-center gap-1.5 cursor-pointer shadow-2xs hover:opacity-90"
                          >
                            <Sliders className="w-3.5 h-3.5" />
                            <span>{t('actions.configure', 'Configure')}</span>
                          </button>

                          <button
                            type="button"
                            onClick={() => {
                              setSelectedPlugin(plugin);
                              setDetailInitialTab('overview');
                              setIsDetailOpen(true);
                            }}
                            className="px-3 py-1.5 rounded-full bg-soft-meadow hover:bg-onyx/10 text-caption font-semibold text-deep-ink transition-all flex items-center gap-1.5 cursor-pointer"
                          >
                            <Shield className="w-3.5 h-3.5" />
                            <span>{t('actions.details', 'Details')}</span>
                          </button>

                          <button
                            type="button"
                            onClick={() => {
                              setSelectedPlugin(plugin);
                              setIsLogsOpen(true);
                            }}
                            className="px-3 py-1.5 rounded-full bg-soft-meadow hover:bg-onyx/10 text-caption font-semibold text-deep-ink transition-all flex items-center gap-1.5 cursor-pointer"
                          >
                            <Terminal className="w-3.5 h-3.5" />
                            <span>{t('actions.viewLogs', 'Logs')}</span>
                          </button>
                        </div>

                        <button
                          type="button"
                          onClick={() => handleUninstall(manifest.id, manifest.name)}
                          title={t('actions.uninstall')}
                          className="p-2 rounded-full text-slate hover:text-red-600 hover:bg-red-500/10 transition-all cursor-pointer"
                        >
                          <Trash2 className="w-4 h-4" />
                        </button>
                      </div>
                    </Card>
                  );
                })}
              </div>
            ) : (
              <div className="p-8 sm:p-12 mb-12 rounded-[28px] border border-onyx/10 bg-soft-meadow/60 text-center max-w-xl mx-auto shadow-xs">
                <div className="w-16 h-16 rounded-full bg-deep-ink/5 text-deep-ink mx-auto mb-4 flex items-center justify-center">
                  <Boxes className="w-8 h-8 opacity-80" />
                </div>
                <h3 className="font-serif text-heading-sm font-bold text-deep-ink mb-2">
                  {searchQuery ? t('empty.noMatches', 'No plugins match your search') : t('empty.title', 'No Plugins Installed Yet')}
                </h3>
                <p className="text-caption text-slate mb-6 max-w-md mx-auto leading-relaxed">
                  {searchQuery ? t('empty.noMatches') : t('empty.description')}
                </p>

                <div className="flex flex-wrap items-center justify-center gap-3">
                  {searchQuery ? (
                    <Button variant="ghost" size="sm" onClick={() => setSearchQuery('')}>
                      {t('actions.clearFilters', 'Clear Filters')}
                    </Button>
                  ) : (
                    <>
                      <Button variant="primary" size="sm" onClick={() => selectView('available')}>
                        {t('views.available', { count: hubCatalog.length })}
                      </Button>
                      <Button variant="ghost" size="sm" icon={<Plus className="w-4 h-4" />} onClick={() => setIsUploadOpen(true)}>
                        {t('upload', 'Upload Package')}
                      </Button>
                    </>
                  )}
                </div>
              </div>
            )}
          </div>
        )}

        {/* View 2: Available Plugins Registry Hub */}
        {activeView === 'available' && (
          <div>
            {/* Category Filter Pills */}
            <div className="flex items-center gap-2 overflow-x-auto pb-3 mb-4 scrollbar-none">
              {availableHubCategories.map((cat) => {
                const isSelected = hubCategory === cat;
                const count = hubCategoryCounts[cat] || 0;
                return (
                  <button
                    key={cat}
                    onClick={() => {
                      setHubCategory(cat);
                      setHubPageSize(18);
                    }}
                    className={`inline-flex items-center gap-1.5 px-3.5 py-1.5 rounded-full text-caption font-sans font-medium whitespace-nowrap transition-all cursor-pointer border ${
                      isSelected
                        ? 'bg-deep-ink text-white border-deep-ink shadow-xs font-semibold'
                        : 'bg-canvas/90 text-deep-ink border-onyx/10 hover:border-onyx/30 hover:bg-soft-meadow'
                    }`}
                  >
                    <span>{t(`hub.categories.${cat}`, cat)}</span>
                    <span
                      className={`text-[10px] px-1.5 py-0.2 rounded-full font-mono ${
                        isSelected ? 'bg-white/20 text-white' : 'bg-soft-meadow text-slate'
                      }`}
                    >
                      {count}
                    </span>
                  </button>
                );
              })}
            </div>

            {/* Controls Bar: Status Filter + Sort + Count */}
            <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-6 p-3 rounded-[16px] bg-canvas/80 border border-onyx/10 shadow-2xs">
              <div className="flex items-center gap-2">
                <span className="text-caption font-semibold uppercase tracking-wider text-slate px-1">
                  {t('hub.title', 'Official Plugin Registry')}
                </span>
                <span className="text-caption font-mono text-slate bg-soft-meadow px-2 py-0.5 rounded-full border border-onyx/5">
                  {filteredHub.length} {t('tabs.all', { count: filteredHub.length })}
                </span>
              </div>

              <div className="flex items-center gap-3 self-end sm:self-auto">
                {/* Status Segment */}
                <div className="flex items-center bg-soft-meadow rounded-full p-0.5 border border-onyx/10 text-caption">
                  <button
                    onClick={() => setHubStatusFilter('all')}
                    className={`px-2.5 py-1 rounded-full transition-all cursor-pointer ${
                      hubStatusFilter === 'all' ? 'bg-deep-ink text-white font-medium shadow-2xs' : 'text-slate hover:text-deep-ink'
                    }`}
                  >
                    {t('hub.statusFilter.all', 'All')}
                  </button>
                  <button
                    onClick={() => setHubStatusFilter('installed')}
                    className={`px-2.5 py-1 rounded-full transition-all cursor-pointer ${
                      hubStatusFilter === 'installed' ? 'bg-deep-ink text-white font-medium shadow-2xs' : 'text-slate hover:text-deep-ink'
                    }`}
                  >
                    {t('hub.statusFilter.installed', 'Installed')}
                  </button>
                  <button
                    onClick={() => setHubStatusFilter('available')}
                    className={`px-2.5 py-1 rounded-full transition-all cursor-pointer ${
                      hubStatusFilter === 'available' ? 'bg-deep-ink text-white font-medium shadow-2xs' : 'text-slate hover:text-deep-ink'
                    }`}
                  >
                    {t('hub.statusFilter.available', 'Available')}
                  </button>
                </div>

                {/* Sort Dropdown */}
                <div className="flex items-center gap-1.5 text-caption font-sans">
                  <span className="text-slate hidden sm:inline">{t('hub.sort.label', 'Sort:')}</span>
                  <select
                    value={hubSortBy}
                    onChange={(e) => setHubSortBy(e.target.value as 'stars' | 'recent' | 'name')}
                    className="bg-soft-meadow text-deep-ink border border-onyx/10 rounded-full px-3 py-1 text-caption focus:outline-none focus:ring-1 focus:ring-deep-ink cursor-pointer"
                  >
                    <option value="stars">{t('hub.sort.stars', 'Most Popular (Stars)')}</option>
                    <option value="recent">{t('hub.sort.recent', 'Recently Updated')}</option>
                    <option value="name">{t('hub.sort.name', 'Name (A-Z)')}</option>
                  </select>
                </div>
              </div>
            </div>

            {/* Grid of Available Registry Plugins */}
            {loading ? (
              <div className="py-20 text-center text-slate font-sans flex flex-col items-center justify-center gap-3">
                <RefreshCw className="w-8 h-8 animate-spin text-deep-ink" />
                <p className="text-body-sm font-semibold">{t('hub.loading', 'Loading Official Plugin Registry...')}</p>
              </div>
            ) : filteredHub.length === 0 ? (
              <div className="bg-soft-meadow rounded-[24px] p-12 text-center max-w-md mx-auto border border-onyx/10 shadow-xs">
                <Search className="w-12 h-12 text-deep-ink mx-auto mb-3 opacity-30" />
                <p className="font-sans text-body-sm text-deep-ink mb-1 font-semibold">
                  {t('empty.noMatches', 'No plugins match your filter criteria')}
                </p>
                <p className="font-sans text-caption text-slate mb-4">
                  {t('empty.adjustFilters', 'Try adjusting your search keywords, status filter, or category.')}
                </p>
                <Button
                  variant="primary"
                  size="sm"
                  onClick={() => {
                    setSearchQuery('');
                    setHubCategory('all');
                    setHubStatusFilter('all');
                  }}
                >
                  {t('actions.clearFilters', 'Clear Filters')}
                </Button>
              </div>
            ) : (
              <>
                <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mb-12">
                  {paginatedHub.map((plugin) => {
                    const capabilities = plugin.capabilities || [plugin.category || 'tool'];
                    const permissions = plugin.permissions || {};
                    const tools = plugin.tools || [];
                    const isInstalled = plugin.installed;
                    const isRunningAction = isExecuting(plugin.id);

                    return (
                      <Card
                        key={plugin.id}
                        className="relative flex flex-col justify-between p-6 rounded-[24px] border border-onyx/10 bg-canvas/95 hover:border-onyx/25 hover:shadow-md transition-all duration-200 group"
                      >
                        <div>
                          {/* Top Row: Icon + Stars + Status Badge */}
                          <div className="flex items-start justify-between gap-3 mb-3.5">
                            <div
                              className={`w-12 h-12 rounded-2xl flex items-center justify-center shrink-0 group-hover:scale-105 transition-transform ${
                                capabilities.includes('channel')
                                  ? 'bg-purple-500/15 text-purple-600'
                                  : capabilities.includes('connector')
                                    ? 'bg-blue-500/15 text-blue-600'
                                    : 'bg-emerald-500/15 text-emerald-600'
                              }`}
                            >
                              {getCapabilityIcon(capabilities)}
                            </div>

                            <div className="flex items-center gap-1.5">
                              {plugin.stars ? (
                                <span className="inline-flex items-center gap-1 text-[11px] font-mono font-medium text-amber-900 bg-amber-500/10 px-2 py-0.5 rounded-full border border-amber-500/20">
                                  <Star className="w-3 h-3 fill-amber-500 text-amber-500" />
                                  {plugin.stars}
                                </span>
                              ) : null}

                              {isInstalled ? (
                                <Badge variant="active" className="flex items-center gap-1 text-[11px]">
                                  <CheckCircle2 className="w-3 h-3" />
                                  {t('hub.installed', 'Installed')}
                                </Badge>
                              ) : null}
                            </div>
                          </div>

                          {/* Plugin Name & Author */}
                          <div className="mb-1.5">
                            <h3 className="font-serif text-subheading font-bold text-deep-ink group-hover:text-onyx transition-colors">
                              {plugin.name || plugin.id}
                            </h3>
                            <p className="text-[11px] text-slate font-sans truncate mt-0.5">
                              {t('hub.byAuthor', { author: plugin.author || 'ActonOS Core Team' })}
                              {plugin.version ? ` • v${plugin.version}` : ''}
                            </p>
                          </div>

                          {/* Description */}
                          <p className="font-sans text-body-sm text-slate mb-4 line-clamp-2 leading-relaxed">
                            {plugin.description}
                          </p>

                          {/* Capabilities & Tools Badges */}
                          <div className="flex flex-wrap gap-1.5 mb-4">
                            {capabilities.map((cap) => (
                              <span
                                key={cap}
                                className={`px-2.5 py-0.5 rounded-md text-[11px] font-semibold uppercase tracking-wider ${getCapabilityBadgeStyle(
                                  cap
                                )}`}
                              >
                                {t(`capabilities.${cap}`, cap)}
                              </span>
                            ))}

                            {tools.length > 0 && (
                              <span className="px-2.5 py-0.5 rounded-md bg-amber-500/15 text-amber-800 text-[11px] font-semibold flex items-center gap-1">
                                <Bot className="w-3 h-3" />
                                <span>{t('stats.toolsCount', { count: tools.length, defaultValue: '{{count}} Tools' })}</span>
                              </span>
                            )}
                          </div>

                          {/* Tags */}
                          {plugin.tags && plugin.tags.length > 0 && (
                            <div className="flex flex-wrap gap-1 mb-4">
                              {plugin.tags.slice(0, 3).map((tag) => (
                                <span
                                  key={tag}
                                  onClick={() => setSearchQuery(tag)}
                                  className="text-[10px] font-mono bg-soft-meadow px-2 py-0.5 rounded-full text-slate border border-onyx/5 hover:border-onyx/20 cursor-pointer transition-colors"
                                >
                                  #{tag}
                                </span>
                              ))}
                            </div>
                          )}

                          {/* Permissions Scope Preview */}
                          <div className="p-2.5 rounded-[16px] bg-soft-meadow/50 border border-onyx/5 text-caption font-mono mb-4 space-y-1">
                            <div className="flex items-center justify-between text-slate text-[11px]">
                              <span className="flex items-center gap-1">
                                <Globe className="w-3 h-3 text-slate" /> {t('permissions.netOutbound', 'Egress')}:
                              </span>
                              <span className="truncate max-w-[150px] text-deep-ink">
                                {permissions.net_outbound?.length ? permissions.net_outbound.join(', ') : t('hub.modal.noEgress', 'Blocked (None)')}
                              </span>
                            </div>
                            {permissions.secrets && permissions.secrets.length > 0 && (
                              <div className="flex items-center justify-between text-slate text-[11px]">
                                <span className="flex items-center gap-1">
                                  <Key className="w-3 h-3 text-slate" /> {t('permissions.secrets', 'Vault')}:
                                </span>
                                <span className="truncate max-w-[150px] text-deep-ink">
                                  {permissions.secrets.join(', ')}
                                </span>
                              </div>
                            )}
                          </div>
                        </div>

                        {/* Card Actions Footer */}
                        <div className="pt-3.5 border-t border-soft-meadow flex items-center justify-between gap-2">
                          <Button
                            variant="ghost"
                            size="sm"
                            icon={<Eye className="w-3.5 h-3.5" />}
                            onClick={() => setInspectHubPlugin(plugin)}
                            title={t('actions.details', 'Inspect Details')}
                          >
                            {t('actions.details', 'Details')}
                          </Button>

                          {isInstalled ? (
                            <div className="flex items-center gap-1.5">
                              <button
                                type="button"
                                onClick={() => {
                                  const inst = plugins.find((p) => p.manifest.id === plugin.id);
                                  if (inst) {
                                    setSelectedPlugin(inst);
                                    setDetailInitialTab('config');
                                    setIsDetailOpen(true);
                                  } else {
                                    setInspectHubPlugin(plugin);
                                  }
                                }}
                                className="px-3 py-1.5 rounded-full bg-soft-meadow hover:bg-onyx/10 text-caption font-semibold text-deep-ink transition-all flex items-center gap-1 cursor-pointer"
                              >
                                <Sliders className="w-3.5 h-3.5" />
                                <span>{t('actions.configure', 'Configure')}</span>
                              </button>
                            </div>
                          ) : (
                            <Button
                              variant="primary"
                              size="sm"
                              icon={<Download className="w-3.5 h-3.5" />}
                              disabled={isRunningAction}
                              onClick={() => handleInstallFromHub(plugin.id, plugin.name, plugin.download_url || plugin.url)}
                            >
                              {isRunningAction ? t('hub.installing', 'Installing...') : t('hub.install', 'Install')}
                            </Button>
                          )}
                        </div>
                      </Card>
                    );
                  })}
                </div>

                {/* Load More Pagination */}
                {hubPageSize < filteredHub.length && (
                  <div className="text-center mb-12">
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => setHubPageSize((prev) => prev + 18)}
                      className="px-6 py-2 rounded-full border border-onyx/15 shadow-2xs font-medium"
                    >
                      {t('hub.pagination.loadMore', 'Load More Plugins')} (
                      {t('hub.pagination.showing', { shown: paginatedHub.length, total: filteredHub.length })})
                    </Button>
                  </div>
                )}
              </>
            )}
          </div>
        )}
      </PageContainer>

      {/* Modals */}
      <PluginUploadModal
        isOpen={isUploadOpen}
        onClose={() => setIsUploadOpen(false)}
        onSuccess={fetchData}
      />

      <PluginDetailModal
        plugin={selectedPlugin}
        isOpen={isDetailOpen}
        initialTab={detailInitialTab}
        onPluginUpdated={fetchData}
        onClose={() => {
          setIsDetailOpen(false);
          setSelectedPlugin(null);
        }}
      />

      <PluginLogsModal
        plugin={selectedPlugin}
        isOpen={isLogsOpen}
        onClose={() => {
          setIsLogsOpen(false);
          setSelectedPlugin(null);
        }}
      />

      <PluginHubDetailModal
        plugin={inspectHubPlugin}
        isOpen={Boolean(inspectHubPlugin)}
        onClose={() => setInspectHubPlugin(null)}
        onInstall={handleInstallFromHub}
        isInstalling={inspectHubPlugin ? isExecuting(inspectHubPlugin.id) : false}
      />
    </div>
  );
}

import { useState, useEffect, useMemo, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { PageHeader } from '@/components/ui/PageHeader';
import { Button } from '@/components/ui/Button';
import { Card } from '@/components/ui/Card';
import { useToast } from '@/components/ui/Toast';
import { api } from '@/lib/api';
import type { PluginInfo } from '@/lib/types';
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
} from 'lucide-react';
import { PluginUploadModal } from './PluginUploadModal';
import { PluginDetailModal } from './PluginDetailModal';
import { PluginLogsModal } from './PluginLogsModal';
import { getErrorMessage } from '@/lib/errors';

export function PluginsPage() {
  const { t } = useTranslation('plugins');
  const { success, error, info } = useToast();

  const [plugins, setPlugins] = useState<PluginInfo[]>([]);
  const [loading, setLoading] = useState(true);
  const [activeTab, setActiveTab] = useState<'all' | 'channel' | 'connector' | 'tool'>('all');
  const [searchQuery, setSearchQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState<'all' | 'running' | 'stopped' | 'error'>('all');
  const [togglingID, setTogglingID] = useState<string | null>(null);

  // Modals state
  const [isUploadOpen, setIsUploadOpen] = useState(false);
  const [selectedPlugin, setSelectedPlugin] = useState<PluginInfo | null>(null);
  const [detailInitialTab, setDetailInitialTab] = useState<'overview' | 'config' | 'tools' | 'raw'>('overview');
  const [isDetailOpen, setIsDetailOpen] = useState(false);
  const [isLogsOpen, setIsLogsOpen] = useState(false);

  const fetchPlugins = useCallback(async () => {
    try {
      setLoading(true);
      const res = await api.listPlugins();
      setPlugins(res.plugins || []);
    } catch (err) {
      error('Failed to load plugins', getErrorMessage(err));
    } finally {
      setLoading(false);
    }
  }, [error]);

  useEffect(() => {
    fetchPlugins();
  }, [fetchPlugins]);

  // Toggle active/inactive
  const handleToggle = async (plugin: PluginInfo) => {
    const id = plugin.manifest.id;
    const shouldEnable = plugin.status !== 'running';
    setTogglingID(id);
    try {
      if (shouldEnable) {
        await api.enablePlugin(id);
        success(t('actions.enable'), `Plugin ${plugin.manifest.name || id} is now running.`);
      } else {
        await api.disablePlugin(id);
        info(t('actions.disable'), `Plugin ${plugin.manifest.name || id} has been stopped.`);
      }
      await fetchPlugins();
    } catch (err) {
      error('Toggle Error', getErrorMessage(err));
    } finally {
      setTogglingID(null);
    }
  };

  // Uninstall plugin
  const handleUninstall = async (plugin: PluginInfo) => {
    const name = plugin.manifest.name || plugin.manifest.id;
    if (!window.confirm(t('actions.confirmUninstall', { name }))) {
      return;
    }
    try {
      await api.uninstallPlugin(plugin.manifest.id);
      success(t('actions.uninstall'), `Plugin ${name} was uninstalled.`);
      if (selectedPlugin?.manifest.id === plugin.manifest.id) {
        setIsDetailOpen(false);
        setIsLogsOpen(false);
      }
      await fetchPlugins();
    } catch (err) {
      error('Uninstall Error', getErrorMessage(err));
    }
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

  // Filtered list
  const filteredPlugins = useMemo(() => {
    return plugins.filter((p) => {
      // Tab filter
      if (activeTab !== 'all') {
        const caps = p.manifest.capabilities || [];
        if (!caps.includes(activeTab)) return false;
      }
      // Status filter
      if (statusFilter !== 'all') {
        if (statusFilter === 'running' && p.status !== 'running') return false;
        if (statusFilter === 'stopped' && p.status !== 'stopped' && p.status !== 'disabled') return false;
        if (statusFilter === 'error' && p.status !== 'error') return false;
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
  }, [plugins, activeTab, statusFilter, searchQuery]);

  return (
    <div className="relative min-h-[calc(100vh-64px)] pb-16">
      <PageContainer maxWidth="wide">
        {/* Header */}
        <PageHeader
          eyebrow={t('eyebrow', 'EXTENSIONS & RUNTIMES')}
          title={t('title', 'WASM Plugins')}
          description={t('subtitle', 'Polyglot sandboxed WebAssembly extensions running inside Wazero JIT runtime with hardware vault brokering and domain egress firewall.')}
          actions={
            <div className="flex flex-wrap items-center gap-3">
              <Button variant="ghost" size="sm" icon={<RefreshCw className={loading ? 'animate-spin' : ''} />} onClick={fetchPlugins}>
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
          <Card className="p-5 border border-onyx/10 dark:border-white/10 bg-soft-meadow/80 dark:bg-soft-meadow/50 rounded-[22px] shadow-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-caption font-semibold uppercase tracking-wider text-slate">{t('stats.total', 'Total Installed')}</p>
                <h3 className="font-serif text-heading font-bold text-deep-ink mt-1">{stats.total}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-deep-ink/5 dark:bg-white/10 text-deep-ink flex items-center justify-center">
                <Boxes className="w-6 h-6" />
              </div>
            </div>
            <p className="text-caption text-slate mt-2 flex items-center gap-1.5">
              <span>{stats.tools} Tool modules active</span>
            </p>
          </Card>

          <Card className="p-5 border border-onyx/10 dark:border-white/10 bg-soft-meadow/80 dark:bg-soft-meadow/50 rounded-[22px] shadow-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-caption font-semibold uppercase tracking-wider text-slate">{t('stats.active', 'Active & Running')}</p>
                <h3 className="font-serif text-heading font-bold text-emerald-600 dark:text-emerald-400 mt-1">{stats.active}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center">
                <Activity className="w-6 h-6" />
              </div>
            </div>
            <p className="text-caption text-slate mt-2 flex items-center gap-1.5">
              <span className="w-2 h-2 rounded-full bg-emerald-500" />
              <span>JIT hot-reloaded memory safe</span>
            </p>
          </Card>

          <Card className="p-5 border border-onyx/10 dark:border-white/10 bg-soft-meadow/80 dark:bg-soft-meadow/50 rounded-[22px] shadow-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-caption font-semibold uppercase tracking-wider text-slate">{t('stats.channels', 'Chat Channels')}</p>
                <h3 className="font-serif text-heading font-bold text-purple-600 dark:text-purple-400 mt-1">{stats.channels}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-purple-500/10 text-purple-600 dark:text-purple-400 flex items-center justify-center">
                <Radio className="w-6 h-6" />
              </div>
            </div>
            <p className="text-caption text-slate mt-2 flex items-center gap-1.5">
              <span>Dynamic messaging adapters</span>
            </p>
          </Card>

          <Card className="p-5 border border-onyx/10 dark:border-white/10 bg-soft-meadow/80 dark:bg-soft-meadow/50 rounded-[22px] shadow-xs">
            <div className="flex items-center justify-between">
              <div>
                <p className="text-caption font-semibold uppercase tracking-wider text-slate">{t('stats.connectors', 'SaaS Connectors')}</p>
                <h3 className="font-serif text-heading font-bold text-blue-600 dark:text-blue-400 mt-1">{stats.connectors}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-blue-500/10 text-blue-600 dark:text-blue-400 flex items-center justify-center">
                <Network className="w-6 h-6" />
              </div>
            </div>
            <p className="text-caption text-slate mt-2 flex items-center gap-1.5">
              <span>Vault token brokered</span>
            </p>
          </Card>
        </div>

        {/* Floating Category Navigation & Filter Row */}
        <div className="flex flex-col lg:flex-row items-stretch lg:items-center justify-between gap-4 mb-6">
          {/* Category Tabs */}
          <div className="flex items-center gap-1 bg-canvas/80 backdrop-blur-sm p-1 rounded-full border border-onyx/10 dark:border-white/10 shadow-xs self-start overflow-x-auto max-w-full">
            <button
              onClick={() => setActiveTab('all')}
              className={`px-4 py-2 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-2 ${activeTab === 'all'
                  ? 'bg-deep-ink text-white shadow-xs dark:bg-hi-yellow dark:text-deep-ink'
                  : 'text-deep-ink hover:text-slate'
                }`}
            >
              <Boxes className="w-3.5 h-3.5" />
              <span>{t('tabs.all', 'All Plugins')}</span>
              <span className={`px-1.5 py-0.2 text-[11px] rounded-full ${activeTab === 'all' ? 'bg-white/20 dark:bg-black/20' : 'bg-onyx/10 dark:bg-white/10'}`}>
                {stats.total}
              </span>
            </button>

            <button
              onClick={() => setActiveTab('channel')}
              className={`px-4 py-2 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-2 ${activeTab === 'channel'
                  ? 'bg-purple-600 text-white shadow-xs dark:bg-purple-500 dark:text-white'
                  : 'text-deep-ink hover:text-slate'
                }`}
            >
              <Radio className="w-3.5 h-3.5" />
              <span>{t('tabs.channel', 'Chat Channels')}</span>
              <span className={`px-1.5 py-0.2 text-[11px] rounded-full ${activeTab === 'channel' ? 'bg-white/20' : 'bg-onyx/10 dark:bg-white/10'}`}>
                {stats.channels}
              </span>
            </button>

            <button
              onClick={() => setActiveTab('connector')}
              className={`px-4 py-2 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-2 ${activeTab === 'connector'
                  ? 'bg-blue-600 text-white shadow-xs dark:bg-blue-500 dark:text-white'
                  : 'text-deep-ink hover:text-slate'
                }`}
            >
              <Network className="w-3.5 h-3.5" />
              <span>{t('tabs.connector', 'SaaS Connectors')}</span>
              <span className={`px-1.5 py-0.2 text-[11px] rounded-full ${activeTab === 'connector' ? 'bg-white/20' : 'bg-onyx/10 dark:bg-white/10'}`}>
                {stats.connectors}
              </span>
            </button>

            <button
              onClick={() => setActiveTab('tool')}
              className={`px-4 py-2 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-2 ${activeTab === 'tool'
                  ? 'bg-emerald-600 text-white shadow-xs dark:bg-emerald-500 dark:text-white'
                  : 'text-deep-ink hover:text-slate'
                }`}
            >
              <Wrench className="w-3.5 h-3.5" />
              <span>{t('tabs.tool', 'Tool Extensions')}</span>
              <span className={`px-1.5 py-0.2 text-[11px] rounded-full ${activeTab === 'tool' ? 'bg-white/20' : 'bg-onyx/10 dark:bg-white/10'}`}>
                {stats.tools}
              </span>
            </button>
          </div>

          {/* Search and Status Dropdown */}
          <div className="flex items-center gap-3">
            <div className="relative flex-1 sm:w-80">
              <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder={t('search.placeholder', 'Search plugins by name, ID, capability, domain, author...')}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-soft-meadow dark:bg-soft-meadow/50 text-deep-ink pl-10 pr-9 py-2 rounded-full border border-onyx/10 dark:border-white/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink dark:focus:ring-hi-yellow transition-all"
              />
              {searchQuery && (
                <button
                  onClick={() => setSearchQuery('')}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink"
                >
                  <X className="w-3.5 h-3.5" />
                </button>
              )}
            </div>

            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as any)}
              className="bg-soft-meadow dark:bg-soft-meadow/50 text-deep-ink px-3.5 py-2 rounded-full border border-onyx/10 dark:border-white/10 text-caption font-semibold focus:outline-none cursor-pointer"
            >
              <option value="all">{t('search.statusFilter', 'All Statuses')}</option>
              <option value="running">🟢 {t('status.running', 'Running')}</option>
              <option value="stopped">⚪ {t('status.stopped', 'Stopped')}</option>
              <option value="error">🔴 {t('status.error', 'Error')}</option>
            </select>
          </div>
        </div>

        {/* Installed Plugins Grid or Empty State */}
        {loading ? (
          <div className="py-20 text-center text-slate font-sans flex flex-col items-center justify-center gap-3">
            <RefreshCw className="w-8 h-8 animate-spin text-deep-ink dark:text-hi-yellow" />
            <p className="text-body-sm font-semibold">Loading WASM Plugins...</p>
          </div>
        ) : filteredPlugins.length > 0 ? (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6 mb-12">
            {filteredPlugins.map((plugin) => {
              const manifest = plugin.manifest;
              const isRunning = plugin.status === 'running';
              const isToggling = togglingID === manifest.id;
              const capabilities = manifest.capabilities || ['tool'];
              const permissions = manifest.permissions || {};
              const tools = manifest.tools || [];

              return (
                <Card
                  key={manifest.id}
                  className="relative flex flex-col justify-between p-6 rounded-[24px] border border-onyx/10 dark:border-white/10 bg-soft-meadow/90 dark:bg-soft-meadow/40 hover:shadow-md transition-all group"
                >
                  <div>
                    {/* Top Header with Icon & Active Toggle Switch */}
                    <div className="flex items-start justify-between gap-3 mb-3">
                      <div className="flex items-center gap-3">
                        <div
                          className={`w-12 h-12 rounded-2xl flex items-center justify-center shrink-0 ${capabilities.includes('channel')
                              ? 'bg-purple-500/15 text-purple-600 dark:text-purple-400'
                              : capabilities.includes('connector')
                                ? 'bg-blue-500/15 text-blue-600 dark:text-blue-400'
                                : 'bg-emerald-500/15 text-emerald-600 dark:text-emerald-400'
                            }`}
                        >
                          {capabilities.includes('channel') ? (
                            <Radio className="w-6 h-6" />
                          ) : capabilities.includes('connector') ? (
                            <Network className="w-6 h-6" />
                          ) : (
                            <Wrench className="w-6 h-6" />
                          )}
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
                        className={`relative inline-flex h-6 w-11 shrink-0 cursor-pointer rounded-full border-2 border-transparent transition-colors duration-200 ease-in-out focus:outline-none ${isRunning ? 'bg-emerald-500' : 'bg-onyx/20 dark:bg-white/20'
                          } ${isToggling ? 'opacity-50 cursor-not-allowed' : ''}`}
                      >
                        <span
                          className={`pointer-events-none inline-block h-5 w-5 transform rounded-full bg-white shadow-lg ring-0 transition duration-200 ease-in-out ${isRunning ? 'translate-x-5' : 'translate-x-0'
                            }`}
                        />
                      </button>
                    </div>

                    {/* Description */}
                    <p className="text-caption text-slate line-clamp-2 mb-4 leading-relaxed">
                      {manifest.description || 'Polyglot WebAssembly extension executing inside linear sandbox runtime.'}
                    </p>

                    {/* Capability Tags */}
                    <div className="flex flex-wrap gap-1.5 mb-4">
                      {capabilities.map((cap) => (
                        <span
                          key={cap}
                          className={`px-2.5 py-1 rounded-md text-[11px] font-semibold uppercase tracking-wider ${cap === 'channel'
                              ? 'bg-purple-500/15 text-purple-700 dark:text-purple-300'
                              : cap === 'connector'
                                ? 'bg-blue-500/15 text-blue-700 dark:text-blue-300'
                                : 'bg-emerald-500/15 text-emerald-700 dark:text-emerald-300'
                            }`}
                        >
                          {t(`capabilities.${cap}`, cap)}
                        </span>
                      ))}

                      {tools.length > 0 && (
                        <span className="px-2.5 py-1 rounded-md bg-amber-500/15 text-amber-800 dark:text-amber-300 text-[11px] font-semibold flex items-center gap-1">
                          <Bot className="w-3 h-3" />
                          <span>{tools.length} Tools</span>
                        </span>
                      )}
                    </div>

                    {/* Permissions / Sandbox Scope Summary */}
                    <div className="p-3 rounded-2xl bg-canvas/60 dark:bg-canvas/40 border border-onyx/5 dark:border-white/5 space-y-1.5 text-caption font-mono mb-4">
                      <div className="flex items-center justify-between text-slate">
                        <span className="flex items-center gap-1.5">
                          <Globe className="w-3.5 h-3.5 text-slate" /> Egress:
                        </span>
                        <span className="truncate max-w-[170px] text-deep-ink">
                          {permissions.net_outbound?.length ? permissions.net_outbound.join(', ') : 'Blocked (None)'}
                        </span>
                      </div>

                      <div className="flex items-center justify-between text-slate">
                        <span className="flex items-center gap-1.5">
                          <Key className="w-3.5 h-3.5 text-slate" /> Vault:
                        </span>
                        <span className="truncate max-w-[170px] text-deep-ink">
                          {permissions.secrets?.length ? permissions.secrets.join(', ') : 'None'}
                        </span>
                      </div>

                      {permissions.storage && (
                        <div className="flex items-center justify-between text-slate">
                          <span className="flex items-center gap-1.5">
                            <Database className="w-3.5 h-3.5 text-slate" /> Storage:
                          </span>
                          <span className="text-emerald-600 dark:text-emerald-400 font-semibold">Isolated SQLite KV</span>
                        </div>
                      )}
                    </div>
                  </div>

                  {/* Actions Footer */}
                  <div className="flex items-center justify-between pt-3 border-t border-onyx/10 dark:border-white/10">
                    <div className="flex items-center gap-1.5 flex-wrap">
                      <button
                        type="button"
                        onClick={() => {
                          setSelectedPlugin(plugin);
                          setDetailInitialTab('config');
                          setIsDetailOpen(true);
                        }}
                        className="px-3 py-1.5 rounded-full bg-deep-ink dark:bg-hi-yellow text-white dark:text-deep-ink text-caption font-semibold transition-all flex items-center gap-1.5 cursor-pointer shadow-2xs hover:opacity-90"
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
                        className="px-3 py-1.5 rounded-full bg-soft-meadow hover:bg-onyx/10 dark:hover:bg-white/10 text-caption font-semibold text-deep-ink transition-all flex items-center gap-1.5 cursor-pointer"
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
                        className="px-3 py-1.5 rounded-full bg-soft-meadow hover:bg-onyx/10 dark:hover:bg-white/10 text-caption font-semibold text-deep-ink transition-all flex items-center gap-1.5 cursor-pointer"
                      >
                        <Terminal className="w-3.5 h-3.5" />
                        <span>{t('actions.viewLogs', 'Logs')}</span>
                      </button>
                    </div>

                    <button
                      type="button"
                      onClick={() => handleUninstall(plugin)}
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
          /* Empty Search or Zero Installed State */
          <div className="p-8 sm:p-12 mb-12 rounded-[28px] border border-onyx/10 dark:border-white/10 bg-soft-meadow/60 dark:bg-soft-meadow/30 text-center max-w-xl mx-auto shadow-xs">
            <div className="w-16 h-16 rounded-full bg-deep-ink/5 dark:bg-white/10 text-deep-ink dark:text-hi-yellow mx-auto mb-4 flex items-center justify-center">
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
                  {t('empty.clearFilters', 'Clear Filters')}
                </Button>
              ) : (
                <Button variant="primary" size="sm" icon={<Plus className="w-4 h-4" />} onClick={() => setIsUploadOpen(true)}>
                  {t('upload', 'Upload Plugin')}
                </Button>
              )}
            </div>
          </div>
        )}
      </PageContainer>

      {/* Modals */}
      <PluginUploadModal
        isOpen={isUploadOpen}
        onClose={() => setIsUploadOpen(false)}
        onSuccess={fetchPlugins}
      />

      <PluginDetailModal
        plugin={selectedPlugin}
        isOpen={isDetailOpen}
        initialTab={detailInitialTab}
        onPluginUpdated={fetchPlugins}
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
    </div>
  );
}

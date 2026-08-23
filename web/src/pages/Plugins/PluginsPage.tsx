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
  Sparkles,
  ChevronRight,
  Bot,
  Zap,
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
  const [isDetailOpen, setIsDetailOpen] = useState(false);
  const [isLogsOpen, setIsLogsOpen] = useState(false);

  // Starter template prefill state
  const [templateManifest, setTemplateManifest] = useState<string | undefined>(undefined);
  const [templateID, setTemplateID] = useState<string | undefined>(undefined);

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

  // Open Upload Modal with specific template
  const handleOpenTemplate = (id: string, manifestObj: object) => {
    setTemplateID(id);
    setTemplateManifest(JSON.stringify(manifestObj, null, 2));
    setIsUploadOpen(true);
  };

  // Open blank Upload Modal
  const handleOpenNewUpload = () => {
    setTemplateID(undefined);
    setTemplateManifest(undefined);
    setIsUploadOpen(true);
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

  // Starter Scaffolding Templates Catalog
  const starterTemplates = useMemo(
    () => [
      {
        id: 'telegram_bot_channel',
        title: t('starters.telegramTitle', 'Telegram Bot Channel'),
        description: t('starters.telegramDesc', 'Dual long-polling gateway with 6-digit pairing PIN verification and streaming support.'),
        category: 'channel',
        language: 'Rust / TinyGo',
        icon: Radio,
        color: 'text-purple-600 dark:text-purple-400 bg-purple-500/10 border-purple-500/20',
        manifest: {
          id: 'telegram_bot_channel',
          name: 'Telegram Bot Gateway',
          version: '1.0.0',
          capabilities: ['channel'],
          permissions: {
            net_outbound: ['api.telegram.org'],
            secrets: ['telegram_bot_token'],
            bus_events: ['channel:message:inbound', 'channel:message:outbound'],
          },
          config: {
            channel_name: 'telegram',
          },
        },
      },
      {
        id: 'discord_gateway_channel',
        title: t('starters.discordTitle', 'Discord Gateway Channel'),
        description: t('starters.discordDesc', 'Gateway WebSocket adapter with message listener, @mentions routing, and embed responses.'),
        category: 'channel',
        language: 'Rust / Zig',
        icon: Radio,
        color: 'text-indigo-600 dark:text-indigo-400 bg-indigo-500/10 border-indigo-500/20',
        manifest: {
          id: 'discord_gateway_channel',
          name: 'Discord Gateway Adapter',
          version: '1.0.0',
          capabilities: ['channel'],
          permissions: {
            net_outbound: ['discord.com', 'gateway.discord.gg'],
            secrets: ['discord_bot_token'],
            bus_events: ['channel:message:inbound'],
          },
          config: {
            channel_name: 'discord',
          },
        },
      },
      {
        id: 'github_saas_connector',
        title: t('starters.githubTitle', 'GitHub SaaS Connector'),
        description: t('starters.githubDesc', 'Inspect repository issues, pull requests, and review code with scoped token brokering.'),
        category: 'connector',
        language: 'AssemblyScript',
        icon: Network,
        color: 'text-blue-600 dark:text-blue-400 bg-blue-500/10 border-blue-500/20',
        manifest: {
          id: 'github_saas_connector',
          name: 'GitHub SaaS Connector',
          version: '1.0.0',
          capabilities: ['connector', 'tool'],
          permissions: {
            net_outbound: ['api.github.com'],
            secrets: ['github_access_token'],
            storage: true,
          },
          tools: [
            {
              name: 'github_search_issues',
              description: 'Search issues and pull requests in repository',
              parameters: {
                type: 'object',
                properties: { repo: { type: 'string' }, query: { type: 'string' } },
                required: ['repo', 'query'],
              },
            },
          ],
        },
      },
      {
        id: 'database_sql_tool',
        title: t('starters.databaseTitle', 'Postgres & SQLite Tool'),
        description: t('starters.databaseDesc', 'Execute sandboxed SQL queries with persistent scoped key-value storage.'),
        category: 'tool',
        language: 'Go / Rust',
        icon: Database,
        color: 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border-emerald-500/20',
        manifest: {
          id: 'database_sql_tool',
          name: 'Database SQL Query Engine',
          version: '1.0.0',
          capabilities: ['tool'],
          permissions: {
            storage: true,
            secrets: ['database_dsn'],
          },
          tools: [
            {
              name: 'sql_query_readonly',
              description: 'Execute safe SELECT queries against database',
              parameters: {
                type: 'object',
                properties: { query: { type: 'string' } },
                required: ['query'],
              },
            },
          ],
        },
      },
      {
        id: 'web_scraper_crawler',
        title: t('starters.scraperTitle', 'Web Scraper & Crawler'),
        description: t('starters.scraperDesc', 'Outbound HTTP fetcher with domain egress firewall compliance and HTML parsing.'),
        category: 'tool',
        language: 'Rust',
        icon: Globe,
        color: 'text-amber-600 dark:text-amber-400 bg-amber-500/10 border-amber-500/20',
        manifest: {
          id: 'web_scraper_crawler',
          name: 'Sandboxed Web Scraper',
          version: '1.0.0',
          capabilities: ['tool'],
          permissions: {
            net_outbound: ['*'],
            storage: true,
          },
          tools: [
            {
              name: 'scrape_web_content',
              description: 'Extract and clean markdown from given URL',
              parameters: {
                type: 'object',
                properties: { url: { type: 'string' } },
                required: ['url'],
              },
            },
          ],
        },
      },
      {
        id: 'crypto_market_ticker',
        title: t('starters.cryptoTitle', 'Live Market Ticker Tool'),
        description: t('starters.cryptoDesc', 'Fetch real-time cryptocurrency and stock pricing with automated interval polling.'),
        category: 'tool',
        language: 'TinyGo',
        icon: Zap,
        color: 'text-cyan-600 dark:text-cyan-400 bg-cyan-500/10 border-cyan-500/20',
        manifest: {
          id: 'crypto_market_ticker',
          name: 'Live Market Data Tool',
          version: '1.0.0',
          capabilities: ['tool'],
          permissions: {
            net_outbound: ['api.coingecko.com', 'api.binance.com'],
            storage: true,
          },
          tools: [
            {
              name: 'get_crypto_price',
              description: 'Get current price and 24h change for crypto token',
              parameters: {
                type: 'object',
                properties: { symbol: { type: 'string' } },
                required: ['symbol'],
              },
            },
          ],
        },
      },
    ],
    [t]
  );

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
              <div className="hidden sm:flex items-center gap-2 px-3 py-1.5 rounded-full bg-emerald-500/10 border border-emerald-500/20 text-emerald-700 dark:text-emerald-300 text-caption font-semibold">
                <span className="w-2 h-2 rounded-full bg-emerald-500 animate-pulse" />
                <span>{t('runtimeBadge', 'Wazero JIT Runtime • Fail-Closed Sandbox')}</span>
              </div>
              <Button variant="ghost" size="sm" icon={<RefreshCw className={loading ? 'animate-spin' : ''} />} onClick={fetchPlugins}>
                {t('actions.refresh', 'Refresh')}
              </Button>
              <Button
                variant="primary"
                size="sm"
                icon={<Plus className="w-4 h-4" />}
                onClick={handleOpenNewUpload}
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
                <h3 className="font-serif text-heading font-bold text-deep-ink dark:text-cream mt-1">{stats.total}</h3>
              </div>
              <div className="w-12 h-12 rounded-2xl bg-deep-ink/5 dark:bg-white/10 text-deep-ink dark:text-cream flex items-center justify-center">
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
              className={`px-4 py-2 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-2 ${
                activeTab === 'all'
                  ? 'bg-deep-ink text-white shadow-xs dark:bg-hi-yellow dark:text-deep-ink'
                  : 'text-deep-ink hover:text-slate dark:text-cream dark:hover:text-slate'
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
              className={`px-4 py-2 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-2 ${
                activeTab === 'channel'
                  ? 'bg-deep-ink text-white shadow-xs dark:bg-hi-yellow dark:text-deep-ink'
                  : 'text-deep-ink hover:text-slate dark:text-cream dark:hover:text-slate'
              }`}
            >
              <Radio className="w-3.5 h-3.5" />
              <span>{t('tabs.channel', 'Chat Channels')}</span>
              <span className={`px-1.5 py-0.2 text-[11px] rounded-full ${activeTab === 'channel' ? 'bg-white/20 dark:bg-black/20' : 'bg-onyx/10 dark:bg-white/10'}`}>
                {stats.channels}
              </span>
            </button>

            <button
              onClick={() => setActiveTab('connector')}
              className={`px-4 py-2 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-2 ${
                activeTab === 'connector'
                  ? 'bg-deep-ink text-white shadow-xs dark:bg-hi-yellow dark:text-deep-ink'
                  : 'text-deep-ink hover:text-slate dark:text-cream dark:hover:text-slate'
              }`}
            >
              <Network className="w-3.5 h-3.5" />
              <span>{t('tabs.connector', 'SaaS Connectors')}</span>
              <span className={`px-1.5 py-0.2 text-[11px] rounded-full ${activeTab === 'connector' ? 'bg-white/20 dark:bg-black/20' : 'bg-onyx/10 dark:bg-white/10'}`}>
                {stats.connectors}
              </span>
            </button>

            <button
              onClick={() => setActiveTab('tool')}
              className={`px-4 py-2 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer flex items-center gap-2 ${
                activeTab === 'tool'
                  ? 'bg-deep-ink text-white shadow-xs dark:bg-hi-yellow dark:text-deep-ink'
                  : 'text-deep-ink hover:text-slate dark:text-cream dark:hover:text-slate'
              }`}
            >
              <Wrench className="w-3.5 h-3.5" />
              <span>{t('tabs.tool', 'Tool Extensions')}</span>
              <span className={`px-1.5 py-0.2 text-[11px] rounded-full ${activeTab === 'tool' ? 'bg-white/20 dark:bg-black/20' : 'bg-onyx/10 dark:bg-white/10'}`}>
                {stats.tools}
              </span>
            </button>
          </div>

          {/* Search & Status Filter */}
          <div className="flex items-center gap-3">
            <div className="relative flex-1 sm:w-80">
              <Search className="w-4 h-4 text-slate absolute left-3.5 top-1/2 -translate-y-1/2" />
              <input
                type="text"
                placeholder={t('search.placeholder', 'Search plugins by name, ID, capability, domain, author...')}
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="w-full bg-soft-meadow dark:bg-soft-meadow/50 text-deep-ink dark:text-cream pl-10 pr-9 py-2 rounded-full border border-onyx/10 dark:border-white/10 text-body-sm font-sans focus:outline-none focus:ring-2 focus:ring-deep-ink dark:focus:ring-hi-yellow transition-all"
              />
              {searchQuery && (
                <button
                  onClick={() => setSearchQuery('')}
                  className="absolute right-3 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink dark:hover:text-cream"
                >
                  <X className="w-3.5 h-3.5" />
                </button>
              )}
            </div>

            <select
              value={statusFilter}
              onChange={(e) => setStatusFilter(e.target.value as any)}
              className="bg-soft-meadow dark:bg-soft-meadow/50 text-deep-ink dark:text-cream px-3.5 py-2 rounded-full border border-onyx/10 dark:border-white/10 text-caption font-semibold focus:outline-none cursor-pointer"
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
                          className={`w-12 h-12 rounded-2xl flex items-center justify-center shrink-0 ${
                            capabilities.includes('channel')
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
                            <h3 className="font-serif text-subheading font-bold text-deep-ink dark:text-cream leading-tight">
                              {manifest.name || manifest.id}
                            </h3>
                            <span className="px-2 py-0.5 rounded-full bg-deep-ink/5 dark:bg-white/10 text-[11px] font-mono font-medium text-slate">
                              v{manifest.version || '1.0.0'}
                            </span>
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
                          isRunning ? 'bg-emerald-500' : 'bg-onyx/20 dark:bg-white/20'
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
                      {manifest.description || 'Polyglot WebAssembly extension executing inside linear sandbox runtime.'}
                    </p>

                    {/* Capability Tags */}
                    <div className="flex flex-wrap gap-1.5 mb-4">
                      {capabilities.map((cap) => (
                        <span
                          key={cap}
                          className={`px-2.5 py-1 rounded-md text-[11px] font-semibold uppercase tracking-wider ${
                            cap === 'channel'
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
                        <span className="truncate max-w-[170px] text-deep-ink dark:text-cream">
                          {permissions.net_outbound?.length ? permissions.net_outbound.join(', ') : 'Blocked (None)'}
                        </span>
                      </div>

                      <div className="flex items-center justify-between text-slate">
                        <span className="flex items-center gap-1.5">
                          <Key className="w-3.5 h-3.5 text-slate" /> Vault:
                        </span>
                        <span className="truncate max-w-[170px] text-deep-ink dark:text-cream">
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
                    <div className="flex items-center gap-2">
                      <button
                        type="button"
                        onClick={() => {
                          setSelectedPlugin(plugin);
                          setIsDetailOpen(true);
                        }}
                        className="px-3 py-1.5 rounded-full bg-soft-meadow hover:bg-onyx/10 dark:hover:bg-white/10 text-caption font-semibold text-deep-ink dark:text-cream transition-all flex items-center gap-1.5 cursor-pointer"
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
                        className="px-3 py-1.5 rounded-full bg-soft-meadow hover:bg-onyx/10 dark:hover:bg-white/10 text-caption font-semibold text-deep-ink dark:text-cream transition-all flex items-center gap-1.5 cursor-pointer"
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
            <h3 className="font-serif text-heading-sm font-bold text-deep-ink dark:text-cream mb-2">
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
                <Button variant="primary" size="sm" icon={<Plus className="w-4 h-4" />} onClick={handleOpenNewUpload}>
                  {t('upload', 'Upload Plugin')}
                </Button>
              )}
            </div>
          </div>
        )}

        {/* Starter Scaffolding & Quick Template Gallery */}
        <div className="mt-8 pt-8 border-t border-onyx/10 dark:border-white/10">
          <div className="mb-6">
            <div className="flex items-center gap-2">
              <Sparkles className="w-5 h-5 text-deep-ink dark:text-hi-yellow" />
              <h2 className="font-serif text-heading-sm font-bold text-deep-ink dark:text-cream">
                {t('starters.title', 'Starter Plugin Templates & Scaffolding')}
              </h2>
            </div>
            <p className="text-body-sm text-slate mt-1">
              {t('starters.subtitle', 'Quickly bootstrap custom polyglot extensions in Rust, TinyGo, Zig, or AssemblyScript compiled to WebAssembly.')}
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-5">
            {starterTemplates.map((item) => {
              const Icon = item.icon;
              return (
                <div
                  key={item.id}
                  className="flex flex-col justify-between p-5 rounded-[22px] border border-onyx/10 dark:border-white/10 bg-soft-meadow/50 dark:bg-soft-meadow/20 hover:border-onyx/30 dark:hover:border-white/20 transition-all group"
                >
                  <div>
                    <div className="flex items-start justify-between gap-3 mb-3">
                      <div className={`w-10 h-10 rounded-xl flex items-center justify-center border ${item.color}`}>
                        <Icon className="w-5 h-5" />
                      </div>
                      <span className="px-2.5 py-0.5 rounded-full bg-deep-ink/5 dark:bg-white/10 text-[11px] font-mono font-medium text-slate">
                        {item.language}
                      </span>
                    </div>

                    <h3 className="font-serif text-body font-bold text-deep-ink dark:text-cream mb-1">{item.title}</h3>
                    <p className="text-caption text-slate leading-relaxed mb-4">{item.description}</p>
                  </div>

                  <button
                    type="button"
                    onClick={() => handleOpenTemplate(item.id, item.manifest)}
                    className="w-full py-2 px-3 rounded-full bg-deep-ink/5 hover:bg-deep-ink hover:text-white dark:bg-white/10 dark:hover:bg-hi-yellow dark:hover:text-deep-ink text-deep-ink dark:text-cream text-caption font-semibold transition-all flex items-center justify-center gap-1.5 cursor-pointer group-hover:shadow-xs"
                  >
                    <span>{t('actions.useTemplate', 'Use Template')}</span>
                    <ChevronRight className="w-3.5 h-3.5" />
                  </button>
                </div>
              );
            })}
          </div>
        </div>
      </PageContainer>

      {/* Modals */}
      <PluginUploadModal
        isOpen={isUploadOpen}
        onClose={() => setIsUploadOpen(false)}
        onSuccess={fetchPlugins}
        initialID={templateID}
        initialManifest={templateManifest}
      />

      <PluginDetailModal
        plugin={selectedPlugin}
        isOpen={isDetailOpen}
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

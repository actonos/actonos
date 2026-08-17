import { useState, useEffect, useMemo } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Button } from '@/components/ui/Button';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import {
  RefreshCw,
  CheckCircle2,
  Shield,
  Zap,
  Layers,
  Sparkles,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { ConnectorInfo, ConnectorCategory } from '@/lib/types';
import { ConnectorCard } from './components/ConnectorCard';
import { ConnectorModal } from './components/ConnectorModal';
import { ConnectorFilterBar } from './components/ConnectorFilterBar';

// Catalog of default + upcoming connectors for easy modular expansion
const EXTENDED_CONNECTORS_CATALOG: Partial<ConnectorInfo>[] = [
  {
    id: 'linear',
    name: 'Linear',
    category: 'Development',
    icon: 'layers',
    risk_level: 'Low',
    description: 'Streamline software projects, issues, sprint cycles, and automated roadmaps.',
    scopes: ['read', 'write', 'issues:create'],
  },
  {
    id: 'supabase',
    name: 'Supabase / Postgres',
    category: 'Databases & Storage',
    icon: 'database',
    risk_level: 'High',
    description: 'Query database tables, vector embeddings, and storage buckets directly from agents.',
    scopes: ['storage:read', 'database:query'],
  },
];

export function ConnectorsPage() {
  const { t } = useTranslation('connectors');
  const { success, error, info } = useToast();

  const [integrations, setIntegrations] = useState<ConnectorInfo[]>([]);
  const [loading, setLoading] = useState(true);

  // Filters state
  const [selectedCategory, setSelectedCategory] = useState<ConnectorCategory>('all');
  const [statusFilter, setStatusFilter] = useState<'all' | 'connected' | 'disconnected'>('all');
  const [searchQuery, setSearchQuery] = useState('');

  // Connection Modal state
  const [activeConnector, setActiveConnector] = useState<ConnectorInfo | null>(null);
  const [connecting, setConnecting] = useState(false);

  // Test state
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<
    Record<string, { success: boolean; latency: number; msg: string }>
  >({});

  // Disconnect Modal
  const [disconnectingConnector, setDisconnectingConnector] = useState<ConnectorInfo | null>(null);

  const loadData = async () => {
    try {
      setLoading(true);
      const res = await api.listIntegrations().catch(() => ({ integrations: [], count: 0 }));
      setIntegrations(res.integrations || []);
    } catch (err: any) {
      error('Failed to load connectors', err.message);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadData();

    // Check for OAuth redirect callback query params
    const urlParams = new URLSearchParams(window.location.search);
    const connected = urlParams.get('connected');
    const status = urlParams.get('status');
    const err = urlParams.get('error');
    const errDesc = urlParams.get('error_description') || urlParams.get('details');

    if (connected && status === 'success') {
      success('Connected Successfully', `Connected ${connected} via OAuth 2.1 PKCE.`);
      window.history.replaceState({}, document.title, window.location.pathname);
      loadData();
    } else if (err) {
      error('Authentication Error', errDesc || err);
      window.history.replaceState({}, document.title, window.location.pathname);
    }
  }, []);

  const handleStartOAuth = async (
    connector: ConnectorInfo,
    customClientID?: string,
    customClientSecret?: string
  ) => {
    setConnecting(true);
    try {
      if (customClientID || customClientSecret) {
        await api.saveConnectorConfig(connector.id, customClientID || '', customClientSecret || '');
      }

      const res = await api.getAuthURL(connector.id, customClientID, customClientSecret);
      if (res.auth_url) {
        info('Redirecting to Provider', `Redirecting to ${connector.name} login...`);
        window.location.href = res.auth_url;
      }
    } catch (err: any) {
      error('OAuth Initialization Failed', err.message);
      setConnecting(false);
    }
  };

  const handleConnectWithToken = async (connector: ConnectorInfo, token: string) => {
    setConnecting(true);
    try {
      const res = await api.saveDirectToken(connector.id, token);
      success(
        'Token Verified & Connected',
        `Connected to ${connector.name} as ${res.identity?.AccountName || 'authorized user'}.`
      );
      setActiveConnector(null);
      loadData();
    } catch (err: any) {
      error('Token Verification Failed', err.message);
    } finally {
      setConnecting(false);
    }
  };

  const handleTestConnection = async (connectorId: string) => {
    setTestingId(connectorId);
    try {
      const res = await api.testConnector(connectorId);
      setTestResults((prev) => ({
        ...prev,
        [connectorId]: {
          success: true,
          latency: res.latency_ms,
          msg: `Valid • ${res.latency_ms}ms (${res.identity?.AccountName || 'Active'})`,
        },
      }));
      success('Connection Valid', `${res.provider} is verified (${res.latency_ms}ms).`);
    } catch (err: any) {
      setTestResults((prev) => ({
        ...prev,
        [connectorId]: {
          success: false,
          latency: 0,
          msg: `Failed: ${err.message}`,
        },
      }));
      error('Connection Test Failed', err.message);
    } finally {
      setTestingId(null);
    }
  };

  const handleConfirmDisconnect = async () => {
    if (!disconnectingConnector) return;
    try {
      await api.disconnectConnector(disconnectingConnector.id);
      success('Disconnected', `Disconnected ${disconnectingConnector.name}.`);
      setDisconnectingConnector(null);
      loadData();
    } catch (err: any) {
      error('Disconnect failed', err.message);
    }
  };

  // Combine backend integrations with catalog for complete view
  const allConnectorsList = useMemo(() => {
    const existingIds = new Set(integrations.map((i) => i.id));
    const upcoming: (ConnectorInfo & { isComingSoon?: boolean })[] = EXTENDED_CONNECTORS_CATALOG.filter(
      (c) => !existingIds.has(c.id!)
    ).map((c) => ({
      id: c.id!,
      name: c.name!,
      category: c.category || 'Productivity',
      icon: c.icon || 'layers',
      risk_level: c.risk_level || 'Low',
      description: c.description || '',
      connected: false,
      scopes: c.scopes || [],
      isComingSoon: true,
    }));

    return [...integrations, ...upcoming];
  }, [integrations]);

  // Category counts calculation
  const categoryCounts = useMemo(() => {
    const counts: Record<ConnectorCategory, number> = {
      all: allConnectorsList.length,
      productivity: 0,
      development: 0,
      knowledge: 0,
      messaging: 0,
      database: 0,
    };

    allConnectorsList.forEach((c) => {
      const cat = c.category.toLowerCase();
      if (cat.includes('prod')) counts.productivity++;
      else if (cat.includes('dev')) counts.development++;
      else if (cat.includes('know') || cat.includes('ai')) counts.knowledge++;
      else if (cat.includes('mess')) counts.messaging++;
      else if (cat.includes('data') || cat.includes('stor')) counts.database++;
    });

    return counts;
  }, [allConnectorsList]);

  // Filtered connectors
  const filteredConnectors = useMemo(() => {
    return allConnectorsList.filter((item) => {
      // Category filter
      if (selectedCategory !== 'all') {
        const cat = item.category.toLowerCase();
        if (selectedCategory === 'productivity' && !cat.includes('prod')) return false;
        if (selectedCategory === 'development' && !cat.includes('dev')) return false;
        if (selectedCategory === 'knowledge' && !cat.includes('know') && !cat.includes('ai'))
          return false;
        if (selectedCategory === 'messaging' && !cat.includes('mess')) return false;
        if (selectedCategory === 'database' && !cat.includes('data') && !cat.includes('stor'))
          return false;
      }

      // Status filter
      if (statusFilter === 'connected' && !item.connected) return false;
      if (statusFilter === 'disconnected' && item.connected) return false;

      // Search query
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase();
        const matchesName = item.name.toLowerCase().includes(q);
        const matchesDesc = item.description.toLowerCase().includes(q);
        const matchesCategory = item.category.toLowerCase().includes(q);
        const matchesScope = (item.scopes || []).some((s) => s.toLowerCase().includes(q));
        if (!matchesName && !matchesDesc && !matchesCategory && !matchesScope) return false;
      }

      return true;
    });
  }, [allConnectorsList, selectedCategory, statusFilter, searchQuery]);

  const connectedCount = integrations.filter((i) => i.connected).length;

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Page Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Integrations & SaaS')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight flex items-center gap-3">
              <span>{t('title', 'Service Connectors')}</span>
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'Connect external services like Google, Notion, GitHub, and Slack with OAuth 2.1 or Personal Access Tokens.'
              )}
            </p>
          </div>

          <div className="flex items-center gap-2.5 shrink-0 self-start sm:self-center">
            <Button
              variant="ghost"
              size="sm"
              icon={<RefreshCw className={`w-3.5 h-3.5 ${loading ? 'animate-spin' : ''}`} />}
              onClick={loadData}
            >
              Refresh
            </Button>
          </div>
        </div>

        {/* Quick Stats Strip */}
        <div className="grid grid-cols-2 md:grid-cols-4 gap-4 mb-8">
          <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex items-center gap-3 shadow-xs">
            <div className="w-10 h-10 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink">
              <CheckCircle2 className="w-4 h-4 text-emerald-600" />
            </div>
            <div>
              <span className="text-caption text-slate block">{t('stats.active', 'Connected Services')}</span>
              <span className="text-heading-sm font-serif font-bold text-deep-ink">
                {connectedCount} / {integrations.length}
              </span>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex items-center gap-3 shadow-xs">
            <div className="w-10 h-10 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink">
              <Shield className="w-4 h-4 text-emerald-600" />
            </div>
            <div>
              <span className="text-caption text-slate block">{t('stats.vault', 'Hardware Vault')}</span>
              <span className="text-body-sm font-semibold text-deep-ink flex items-center gap-1 mt-0.5">
                <span className="w-2 h-2 rounded-full bg-emerald-500"></span>
                <span>AES-256-GCM</span>
              </span>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex items-center gap-3 shadow-xs">
            <div className="w-10 h-10 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink">
              <Zap className="w-4 h-4 text-deep-ink" />
            </div>
            <div>
              <span className="text-caption text-slate block">{t('stats.auth', 'OAuth Protocol')}</span>
              <span className="text-body-sm font-semibold text-deep-ink block mt-0.5 font-mono">
                OAuth 2.1 PKCE
              </span>
            </div>
          </div>

          <div className="p-4 rounded-2xl bg-canvas/90 border border-onyx/10 flex items-center gap-3 shadow-xs">
            <div className="w-10 h-10 rounded-full bg-soft-meadow border border-onyx/10 flex items-center justify-center text-deep-ink">
              <Layers className="w-4 h-4 text-deep-ink" />
            </div>
            <div>
              <span className="text-caption text-slate block">{t('stats.catalog', 'Available Services')}</span>
              <span className="text-heading-sm font-serif font-bold text-deep-ink">
                {allConnectorsList.length}
              </span>
            </div>
          </div>
        </div>

        {/* Filter and Search Bar */}
        <ConnectorFilterBar
          selectedCategory={selectedCategory}
          onSelectCategory={setSelectedCategory}
          statusFilter={statusFilter}
          onSelectStatusFilter={setStatusFilter}
          searchQuery={searchQuery}
          onSearchChange={setSearchQuery}
          categoryCounts={categoryCounts}
        />

        {/* Connectors Grid */}
        {loading ? (
          <div className="py-24 text-center text-slate font-sans">
            <RefreshCw className="w-6 h-6 animate-spin mx-auto mb-2 text-slate" />
            <p>Loading connectors...</p>
          </div>
        ) : filteredConnectors.length === 0 ? (
          <div className="py-16 text-center bg-soft-meadow/40 rounded-3xl border border-onyx/10 mb-12">
            <Sparkles className="w-8 h-8 text-slate mx-auto mb-2 opacity-50" />
            <p className="text-body-sm font-semibold text-deep-ink">No connectors match your filter</p>
            <p className="text-caption text-slate mt-1">Try resetting the category filter or searching for another keyword.</p>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => {
                setSelectedCategory('all');
                setStatusFilter('all');
                setSearchQuery('');
              }}
              className="mt-3"
            >
              Reset Filters
            </Button>
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-2 gap-6 mb-12">
            {filteredConnectors.map((item) => (
              <ConnectorCard
                key={item.id}
                connector={item}
                isTesting={testingId === item.id}
                testResult={testResults[item.id]}
                onConnect={(c) => setActiveConnector(c)}
                onTest={handleTestConnection}
                onDisconnect={(c) => setDisconnectingConnector(c)}
                isComingSoon={(item as any).isComingSoon}
              />
            ))}
          </div>
        )}
      </PageContainer>

      {/* Universal Connection Modal */}
      <ConnectorModal
        isOpen={!!activeConnector}
        onClose={() => setActiveConnector(null)}
        connector={activeConnector}
        onStartOAuth={handleStartOAuth}
        onConnectWithToken={handleConnectWithToken}
        connecting={connecting}
      />

      {/* Disconnect Confirmation Modal */}
      <ConfirmModal
        isOpen={!!disconnectingConnector}
        onClose={() => setDisconnectingConnector(null)}
        onConfirm={handleConfirmDisconnect}
        title={`Disconnect ${disconnectingConnector?.name || 'Connector'}`}
        description={`Are you sure you want to disconnect ${disconnectingConnector?.name}? Stored tokens and access permissions will be removed from the encrypted vault.`}
        confirmLabel="Disconnect"
        variant="danger"
      />
    </div>
  );
}

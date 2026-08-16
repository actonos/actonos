import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { PageContainer } from '@/components/layout/PageContainer';
import { BlobBackdrop } from '@/components/ui/BlobBackdrop';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import { Modal } from '@/components/ui/Modal';
import { ConfirmModal } from '@/components/ui/ConfirmModal';
import { useToast } from '@/components/ui/Toast';
import {
  Mail,
  BookOpen,
  Github,
  MessageCircle,
  ExternalLink,
  CheckCircle2,
  XCircle,
  RefreshCw,
  Key,
  Shield,
  Zap,
  ArrowRight,
  Info,
  LogOut,
} from 'lucide-react';
import { api } from '@/lib/api';
import type { ConnectorInfo } from '@/lib/types';

interface ProviderHelpGuide {
  tokenName: string;
  tokenPlaceholder: string;
  tokenHelpUrl: string;
  tokenHelpText: string;
}

const PROVIDER_GUIDES: Record<string, ProviderHelpGuide> = {
  github: {
    tokenName: 'GitHub Personal Access Token (classic or fine-grained)',
    tokenPlaceholder: 'ghp_xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://github.com/settings/tokens',
    tokenHelpText: 'Generate a token with repo, read:user, and user:email scopes.',
  },
  notion: {
    tokenName: 'Notion Internal Integration Secret',
    tokenPlaceholder: 'secret_xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://www.notion.so/my-integrations',
    tokenHelpText: 'Create an internal integration and paste the "Internal Integration Secret".',
  },
  slack: {
    tokenName: 'Slack Bot User OAuth Token',
    tokenPlaceholder: 'xoxb-xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://api.slack.com/apps',
    tokenHelpText: 'Create a Slack App with chat:write, channels:read, users:read permissions.',
  },
  google_workspace: {
    tokenName: 'Google OAuth Access Token / Service Account Key',
    tokenPlaceholder: 'ya29.xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://console.cloud.google.com/apis/credentials',
    tokenHelpText: 'Use OAuth 2.1 or provide an authorized Google Access Token.',
  },
};

export function ConnectorsPage() {
  const { t } = useTranslation('connectors');
  const { success, error, info } = useToast();
  const [integrations, setIntegrations] = useState<ConnectorInfo[]>([]);
  const [loading, setLoading] = useState(true);

  // Connection Modal state
  const [activeConnector, setActiveConnector] = useState<ConnectorInfo | null>(null);
  const [authMode, setAuthMode] = useState<'oauth' | 'token'>('oauth');
  const [directToken, setDirectToken] = useState('');
  const [customClientID, setCustomClientID] = useState('');
  const [customClientSecret, setCustomClientSecret] = useState('');
  const [showAdvancedOAuth, setShowAdvancedOAuth] = useState(false);
  const [connecting, setConnecting] = useState(false);

  // Test state
  const [testingId, setTestingId] = useState<string | null>(null);
  const [testResults, setTestResults] = useState<Record<string, { success: boolean; latency: number; msg: string }>>({});

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

  const handleOpenConnectModal = (connector: ConnectorInfo) => {
    setActiveConnector(connector);
    setAuthMode('oauth');
    setDirectToken('');
    setCustomClientID(connector.client_id || '');
    setCustomClientSecret(connector.client_secret || '');
    setShowAdvancedOAuth(false);
  };

  const handleStartOAuth = async () => {
    if (!activeConnector) return;
    setConnecting(true);
    try {
      // If custom credentials were provided, save them first
      if (customClientID || customClientSecret) {
        await api.saveConnectorConfig(activeConnector.id, customClientID, customClientSecret);
      }

      const res = await api.getAuthURL(activeConnector.id, customClientID, customClientSecret);
      if (res.auth_url) {
        info('Redirecting to OAuth Provider', `Redirecting to ${activeConnector.name} for authorization...`);
        window.location.href = res.auth_url;
      }
    } catch (err: any) {
      error('OAuth Initialization Failed', err.message);
      setConnecting(false);
    }
  };

  const handleConnectWithToken = async () => {
    if (!activeConnector || !directToken.trim()) return;
    setConnecting(true);
    try {
      const res = await api.saveDirectToken(activeConnector.id, directToken.trim());
      success('Token Verified & Connected', `Connected to ${activeConnector.name} as ${res.identity?.AccountName || 'authorized user'}.`);
      setActiveConnector(null);
      setDirectToken('');
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

  const getIcon = (iconName: string) => {
    switch (iconName) {
      case 'mail':
        return <Mail className="w-5 h-5 text-deep-ink" />;
      case 'book-open':
        return <BookOpen className="w-5 h-5 text-deep-ink" />;
      case 'github':
        return <Github className="w-5 h-5 text-deep-ink" />;
      case 'message-circle':
        return <MessageCircle className="w-5 h-5 text-deep-ink" />;
      default:
        return <ExternalLink className="w-5 h-5 text-deep-ink" />;
    }
  };

  const connectedCount = integrations.filter((i) => i.connected).length;

  return (
    <div className="relative min-h-[calc(100vh-64px)]">
      <BlobBackdrop />

      <PageContainer>
        {/* Header */}
        <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-8">
          <div className="flex-1">
            <span className="text-caption uppercase tracking-wider text-slate font-semibold block mb-1">
              {t('eyebrow', 'Connectors')}
            </span>
            <h1 className="font-serif text-heading-lg text-deep-ink tracking-tight flex items-center gap-3">
              <span>{t('title', 'Service Connectors')}</span>
              <Badge variant="neutral" className="text-caption font-mono">
                {connectedCount}/{integrations.length} {t('status.connected', 'Connected')}
              </Badge>
            </h1>
            <p className="font-sans text-body text-slate mt-1 max-w-2xl">
              {t(
                'subtitle',
                'Connect external services like Google, Notion, and GitHub with OAuth 2.1 or Personal Access Tokens.'
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

        {/* Connectors Grid */}
        {loading ? (
          <div className="py-20 text-center text-slate font-sans">Loading connectors...</div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-6">
            {integrations.map((item) => {
              const testInfo = testResults[item.id];
              const isTesting = testingId === item.id;

              return (
                <Card
                  key={item.id}
                  className={`flex flex-col justify-between border p-6 transition-all ${
                    item.connected
                      ? 'border-emerald-500/30 bg-canvas/95 shadow-sm'
                      : 'border-onyx/10 bg-canvas/80'
                  }`}
                >
                  <div>
                    {/* Top Row: Icon, Badges */}
                    <div className="flex items-center justify-between mb-3">
                      <div className="w-10 h-10 rounded-full bg-soft-meadow flex items-center justify-center border border-onyx/10 shadow-xs">
                        {getIcon(item.icon)}
                      </div>
                      <div className="flex items-center gap-2">
                        <Badge variant={item.risk_level === 'High' ? 'accent' : 'neutral'}>
                          {t(`risk.${item.risk_level.toLowerCase()}`, `${item.risk_level} Risk`)}
                        </Badge>
                        <Badge variant={item.connected ? 'active' : 'stopped'}>
                          {item.connected ? t('status.connected', 'Connected') : t('status.disconnected', 'Disconnected')}
                        </Badge>
                      </div>
                    </div>

                    {/* Title & Description */}
                    <h3 className="font-serif text-heading-sm text-deep-ink mb-1">
                      {item.name}
                    </h3>
                    <p className="font-sans text-body-sm text-slate mb-4">
                      {item.description}
                    </p>

                    {/* Connected Account Details Card */}
                    {item.connected && (
                      <div className="p-3.5 rounded-2xl bg-soft-meadow border border-onyx/10 mb-4 space-y-1.5 text-caption font-sans">
                        <div className="flex items-center justify-between">
                          <span className="text-slate">Account:</span>
                          <span className="font-semibold text-deep-ink font-mono truncate max-w-[200px]">
                            {item.account_name || item.account_email || 'Authenticated User'}
                          </span>
                        </div>
                        {item.account_email && item.account_name !== item.account_email && (
                          <div className="flex items-center justify-between">
                            <span className="text-slate">Email:</span>
                            <span className="font-mono text-deep-ink">{item.account_email}</span>
                          </div>
                        )}
                        <div className="flex items-center justify-between">
                          <span className="text-slate">Method:</span>
                          <span className="font-mono uppercase text-[10px] bg-canvas px-2 py-0.5 rounded border border-onyx/5">
                            {item.auth_type === 'oauth' ? 'OAuth 2.1 PKCE' : 'Personal Token'}
                          </span>
                        </div>
                        {testInfo && (
                          <div
                            className={`flex items-center gap-1.5 pt-1 text-[11px] font-mono ${
                              testInfo.success ? 'text-emerald-700 font-semibold' : 'text-red-600'
                            }`}
                          >
                            {testInfo.success ? <CheckCircle2 className="w-3.5 h-3.5" /> : <XCircle className="w-3.5 h-3.5" />}
                            <span>{testInfo.msg}</span>
                          </div>
                        )}
                      </div>
                    )}
                  </div>

                  {/* Actions Footer */}
                  <div className="pt-4 border-t border-onyx/10 flex items-center justify-between">
                    <span className="text-caption text-slate font-mono">
                      {item.category}
                    </span>

                    <div className="flex items-center gap-2">
                      {item.connected ? (
                        <>
                          <Button
                            variant="ghost"
                            size="sm"
                            icon={<RefreshCw className={`w-3.5 h-3.5 ${isTesting ? 'animate-spin' : ''}`} />}
                            onClick={() => handleTestConnection(item.id)}
                            disabled={isTesting}
                          >
                            {isTesting ? 'Testing...' : 'Test'}
                          </Button>
                          <Button
                            variant="danger"
                            size="sm"
                            icon={<LogOut className="w-3.5 h-3.5" />}
                            onClick={() => setDisconnectingConnector(item)}
                          >
                            {t('actions.disconnect', 'Disconnect')}
                          </Button>
                        </>
                      ) : (
                        <Button
                          variant="primary"
                          size="sm"
                          icon={<Zap className="w-3.5 h-3.5" />}
                          onClick={() => handleOpenConnectModal(item)}
                        >
                          {t('actions.connect', 'Connect')}
                        </Button>
                      )}
                    </div>
                  </div>
                </Card>
              );
            })}
          </div>
        )}
      </PageContainer>

      {/* Connection Modal (OAuth + Direct Token) */}
      <Modal
        isOpen={!!activeConnector}
        onClose={() => {
          if (!connecting) setActiveConnector(null);
        }}
        title={`Connect to ${activeConnector?.name || 'Service'}`}
      >
        {activeConnector && (
          <div className="space-y-5">
            {/* Auth Mode Tabs */}
            <div className="flex items-center gap-1.5 bg-soft-meadow p-1 rounded-full border border-onyx/10">
              <button
                type="button"
                onClick={() => setAuthMode('oauth')}
                className={`flex-1 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer ${
                  authMode === 'oauth'
                    ? 'bg-deep-ink text-white shadow-xs'
                    : 'text-deep-ink hover:text-slate'
                }`}
              >
                🔐 OAuth 2.1 (Browser Login)
              </button>
              <button
                type="button"
                onClick={() => setAuthMode('token')}
                className={`flex-1 py-1.5 rounded-full text-caption font-sans font-semibold transition-all cursor-pointer ${
                  authMode === 'token'
                    ? 'bg-deep-ink text-white shadow-xs'
                    : 'text-deep-ink hover:text-slate'
                }`}
              >
                🔑 Direct Token / PAT
              </button>
            </div>

            {/* TAB 1: OAuth 2.1 Flow */}
            {authMode === 'oauth' && (
              <div className="space-y-4">
                <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 text-body-sm text-deep-ink">
                  <div className="flex items-center gap-2 mb-1.5 font-semibold">
                    <Shield className="w-4 h-4 text-emerald-600" />
                    <span>Seamless Browser Authorization</span>
                  </div>
                  <p className="text-caption text-slate leading-relaxed">
                    Clicking continue will redirect you to {activeConnector.name}'s secure login screen. Upon approval, ActonOS will automatically exchange and store your access tokens in the encrypted vault.
                  </p>
                </div>

                {/* Optional Custom OAuth Client Credentials */}
                <div className="pt-1">
                  <button
                    type="button"
                    onClick={() => setShowAdvancedOAuth(!showAdvancedOAuth)}
                    className="text-[11px] font-semibold uppercase tracking-wider text-slate hover:text-deep-ink transition-colors flex items-center gap-1 cursor-pointer"
                  >
                    <span>{showAdvancedOAuth ? '▾ Hide Custom Client Credentials' : '▸ Custom Client Credentials (Optional)'}</span>
                  </button>

                  {showAdvancedOAuth && (
                    <div className="mt-3 p-4 rounded-2xl bg-canvas border border-onyx/10 space-y-3">
                      <div>
                        <label className="text-caption font-semibold text-deep-ink block mb-1">
                          OAuth Client ID
                        </label>
                        <Input
                          placeholder="Your custom OAuth Client ID..."
                          value={customClientID}
                          onChange={(e) => setCustomClientID(e.target.value)}
                        />
                      </div>
                      <div>
                        <label className="text-caption font-semibold text-deep-ink block mb-1">
                          OAuth Client Secret
                        </label>
                        <Input
                          type="password"
                          placeholder="Your custom OAuth Client Secret..."
                          value={customClientSecret}
                          onChange={(e) => setCustomClientSecret(e.target.value)}
                        />
                      </div>
                    </div>
                  )}
                </div>

                <Button
                  variant="primary"
                  size="md"
                  icon={<ArrowRight className="w-4 h-4" />}
                  onClick={handleStartOAuth}
                  disabled={connecting}
                  className="w-full justify-center py-2.5"
                >
                  {connecting ? 'Redirecting...' : `Continue with ${activeConnector.name}`}
                </Button>
              </div>
            )}

            {/* TAB 2: Personal Access Token (PAT) */}
            {authMode === 'token' && (
              <div className="space-y-4">
                {(() => {
                  const guide = PROVIDER_GUIDES[activeConnector.id] || {
                    tokenName: 'API Token / Secret Key',
                    tokenPlaceholder: 'Enter token...',
                    tokenHelpUrl: '',
                    tokenHelpText: 'Provide an authorized bearer token.',
                  };

                  return (
                    <div className="space-y-3">
                      <div>
                        <label className="text-caption font-semibold text-deep-ink block mb-1">
                          {guide.tokenName}
                        </label>
                        <Input
                          type="password"
                          placeholder={guide.tokenPlaceholder}
                          value={directToken}
                          onChange={(e) => setDirectToken(e.target.value)}
                          required
                        />
                      </div>

                      {guide.tokenHelpUrl && (
                        <div className="p-3 rounded-xl bg-soft-meadow border border-onyx/5 flex items-start gap-2 text-caption text-slate">
                          <Info className="w-4 h-4 text-deep-ink shrink-0 mt-0.5" />
                          <div>
                            <p>{guide.tokenHelpText}</p>
                            <a
                              href={guide.tokenHelpUrl}
                              target="_blank"
                              rel="noreferrer"
                              className="text-deep-ink font-semibold underline hover:text-slate inline-flex items-center gap-1 mt-1"
                            >
                              <span>Generate token on {activeConnector.name}</span>
                              <ExternalLink className="w-3 h-3" />
                            </a>
                          </div>
                        </div>
                      )}

                      <Button
                        variant="primary"
                        size="md"
                        icon={<Key className="w-4 h-4" />}
                        onClick={handleConnectWithToken}
                        disabled={connecting || !directToken.trim()}
                        className="w-full justify-center py-2.5"
                      >
                        {connecting ? 'Verifying Token...' : 'Verify & Connect'}
                      </Button>
                    </div>
                  );
                })()}
              </div>
            )}
          </div>
        )}
      </Modal>

      {/* Disconnect Confirmation Modal */}
      <ConfirmModal
        isOpen={!!disconnectingConnector}
        onClose={() => setDisconnectingConnector(null)}
        onConfirm={handleConfirmDisconnect}
        title={`Disconnect ${disconnectingConnector?.name || 'Connector'}`}
        description={`Are you sure you want to disconnect ${disconnectingConnector?.name}? Stored tokens and access permissions will be removed from the kernel.`}
        confirmLabel="Disconnect"
        variant="danger"
      />
    </div>
  );
}

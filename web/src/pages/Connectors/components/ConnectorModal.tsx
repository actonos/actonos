import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import {
  ArrowRight,
  Info,
  ExternalLink,
  Lock,
  Copy,
  Check,
  Eye,
  EyeOff,
} from 'lucide-react';
import type { ConnectorInfo } from '@/lib/types';

interface ProviderHelpGuide {
  tokenName: string;
  tokenPlaceholder: string;
  tokenHelpUrl: string;
  tokenHelpText: string;
  oauthDevUrl?: string;
  oauthDevName?: string;
}

const PROVIDER_GUIDES: Record<string, ProviderHelpGuide> = {
  github: {
    tokenName: 'GitHub Personal Access Token (classic or fine-grained)',
    tokenPlaceholder: 'ghp_xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://github.com/settings/tokens',
    tokenHelpText: 'Generate a token with repo, read:user, and user:email scopes.',
    oauthDevUrl: 'https://github.com/settings/developers',
    oauthDevName: 'GitHub Developer Settings > OAuth Apps',
  },
  notion: {
    tokenName: 'Notion Internal Integration Secret',
    tokenPlaceholder: 'secret_xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://www.notion.so/my-integrations',
    tokenHelpText: 'Create an internal integration and paste the "Internal Integration Secret".',
    oauthDevUrl: 'https://www.notion.so/my-integrations',
    oauthDevName: 'Notion Developers > My Integrations',
  },
  slack: {
    tokenName: 'Slack Bot User OAuth Token',
    tokenPlaceholder: 'xoxb-xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://api.slack.com/apps',
    tokenHelpText: 'Create a Slack App with chat:write, channels:read, users:read permissions.',
    oauthDevUrl: 'https://api.slack.com/apps',
    oauthDevName: 'Slack API > Your Apps',
  },
  google_workspace: {
    tokenName: 'Google OAuth Access Token / Service Account Key',
    tokenPlaceholder: 'ya29.xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://console.cloud.google.com/apis/credentials',
    tokenHelpText: 'Use OAuth 2.1 or provide an authorized Google Access Token.',
    oauthDevUrl: 'https://console.cloud.google.com/apis/credentials',
    oauthDevName: 'Google Cloud Console > Credentials',
  },
  linear: {
    tokenName: 'Linear Personal API Key',
    tokenPlaceholder: 'lin_api_xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://linear.app/settings/api',
    tokenHelpText: 'Create a personal API key from Account Settings > API.',
    oauthDevUrl: 'https://linear.app/settings/api',
    oauthDevName: 'Linear Settings > API',
  },
};

interface ConnectorModalProps {
  isOpen: boolean;
  onClose: () => void;
  connector: ConnectorInfo | null;
  onStartOAuth: (connector: ConnectorInfo, customClientID?: string, customClientSecret?: string) => Promise<void>;
  onConnectWithToken: (connector: ConnectorInfo, token: string) => Promise<void>;
  connecting: boolean;
}

export function ConnectorModal({
  isOpen,
  onClose,
  connector,
  onStartOAuth,
  onConnectWithToken,
  connecting,
}: ConnectorModalProps) {
  const { t } = useTranslation('connectors');
  const [authMode, setAuthMode] = useState<'oauth' | 'token'>('oauth');
  const [directToken, setDirectToken] = useState('');
  const [customClientID, setCustomClientID] = useState('');
  const [customClientSecret, setCustomClientSecret] = useState('');
  const [showSecret, setShowSecret] = useState(false);
  const [copiedRedirect, setCopiedRedirect] = useState(false);

  useEffect(() => {
    if (connector) {
      setAuthMode('oauth');
      setDirectToken('');
      setCustomClientID(connector.client_id || '');
      setCustomClientSecret(connector.client_secret || '');
      setShowSecret(false);
      setCopiedRedirect(false);
    }
  }, [connector, isOpen]);

  if (!connector) return null;

  const guide = PROVIDER_GUIDES[connector.id] || {
    tokenName: 'API Token / Bearer Key',
    tokenPlaceholder: 'Enter secret token...',
    tokenHelpUrl: '',
    tokenHelpText: 'Provide an authorized secret token with sufficient permissions.',
  };

  const redirectUri = `${window.location.origin}/api/integrations/oauth/callback`;

  const handleCopyRedirectUri = () => {
    navigator.clipboard.writeText(redirectUri);
    setCopiedRedirect(true);
    setTimeout(() => setCopiedRedirect(false), 2000);
  };

  const handleOAuthSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!customClientID.trim() || !customClientSecret.trim()) return;
    onStartOAuth(connector, customClientID.trim(), customClientSecret.trim());
  };

  const handleTokenSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!directToken.trim()) return;
    onConnectWithToken(connector, directToken.trim());
  };

  const isOAuthValid = customClientID.trim().length > 0 && customClientSecret.trim().length > 0;

  return (
    <Modal
      isOpen={isOpen}
      onClose={() => {
        if (!connecting) onClose();
      }}
      title={t('ui.connectTo', { connector: connector.name })}
    >
      <div className="space-y-5">
        {/* Auth Mode Switcher */}
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
            {t('ui.oauthBrowser')}
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
            {t('ui.directToken')}
          </button>
        </div>

        {/* TAB 1: OAuth 2.1 PKCE Flow with Required Custom Credentials */}
        {authMode === 'oauth' && (
          <form onSubmit={handleOAuthSubmit} className="space-y-4">
            {/* Open Source Notice Banner */}
            <div className="p-3.5 rounded-2xl bg-hi-yellow/10 border border-hi-yellow/25 text-body-sm text-deep-ink flex items-start gap-2.5">
              <Info className="w-4 h-4 text-deep-ink shrink-0 mt-0.5" />
              <p className="text-[12px] leading-relaxed text-deep-ink/90">
                {t('ui.openSourceNotice')}
              </p>
            </div>

            {/* Developer Guidance & Redirect URI */}
            <div className="p-4 rounded-2xl bg-canvas border border-onyx/10 space-y-3">
              {/* Redirect URI Box */}
              <div>
                <label className="text-[11px] font-semibold text-slate uppercase tracking-wider block mb-1">
                  {t('ui.redirectUri')}
                </label>
                <div className="flex items-center gap-2 bg-soft-meadow/80 border border-onyx/10 rounded-xl px-3 py-2">
                  <span className="font-mono text-xs text-deep-ink truncate flex-1 select-all">
                    {redirectUri}
                  </span>
                  <button
                    type="button"
                    onClick={handleCopyRedirectUri}
                    className="p-1 rounded-lg hover:bg-onyx/10 text-slate hover:text-deep-ink transition-colors shrink-0 cursor-pointer"
                    title={t('ui.copyRedirectUri')}
                  >
                    {copiedRedirect ? (
                      <Check className="w-3.5 h-3.5 text-emerald-600" />
                    ) : (
                      <Copy className="w-3.5 h-3.5" />
                    )}
                  </button>
                </div>
              </div>

              {/* Developer Console Link */}
              {guide.oauthDevUrl && (
                <div>
                  <a
                    href={guide.oauthDevUrl}
                    target="_blank"
                    rel="noreferrer"
                    className="inline-flex items-center gap-1.5 text-xs font-semibold text-deep-ink underline hover:text-slate transition-colors"
                  >
                    <span>{t('ui.createOAuthAppOn', { connector: connector.name })}</span>
                    <ExternalLink className="w-3.5 h-3.5 shrink-0" />
                  </a>
                </div>
              )}

              {/* Required Scopes */}
              {connector.scopes && connector.scopes.length > 0 && (
                <div>
                  <label className="text-[10px] font-semibold text-slate uppercase tracking-wider block mb-1">
                    {t('ui.requiredScopes')}
                  </label>
                  <div className="flex flex-wrap gap-1.5">
                    {connector.scopes.map((scope) => (
                      <span
                        key={scope}
                        className="px-2 py-0.5 rounded-full bg-soft-meadow border border-onyx/10 text-[10px] font-mono text-deep-ink/80"
                      >
                        {scope}
                      </span>
                    ))}
                  </div>
                </div>
              )}
            </div>

            {/* Mandatory OAuth Client ID and Secret */}
            <div className="space-y-3">
              <div>
                <label className="text-caption font-semibold text-deep-ink block mb-1">
                  {t('ui.clientId')} <span className="text-red-500">*</span>
                </label>
                <Input
                  placeholder={t('ui.clientIdPlaceholder')}
                  value={customClientID}
                  onChange={(e) => setCustomClientID(e.target.value)}
                  required
                />
              </div>

              <div>
                <label className="text-caption font-semibold text-deep-ink block mb-1">
                  {t('ui.clientSecret')} <span className="text-red-500">*</span>
                </label>
                <div className="relative">
                  <Input
                    type={showSecret ? 'text' : 'password'}
                    placeholder={t('ui.clientSecretPlaceholder')}
                    value={customClientSecret}
                    onChange={(e) => setCustomClientSecret(e.target.value)}
                    required
                    className="pr-9"
                  />
                  <button
                    type="button"
                    onClick={() => setShowSecret(!showSecret)}
                    className="absolute right-2.5 top-1/2 -translate-y-1/2 text-slate hover:text-deep-ink transition-colors cursor-pointer"
                  >
                    {showSecret ? <EyeOff className="w-4 h-4" /> : <Eye className="w-4 h-4" />}
                  </button>
                </div>
              </div>
            </div>

            <Button
              type="submit"
              variant="primary"
              size="md"
              icon={<ArrowRight className="w-4 h-4" />}
              disabled={connecting || !isOAuthValid}
              className="w-full justify-center py-2.5"
            >
              {connecting ? 'Redirecting to Provider...' : `Authorize & Connect with ${connector.name}`}
            </Button>
          </form>
        )}

        {/* TAB 2: Personal Access Token (PAT) */}
        {authMode === 'token' && (
          <form onSubmit={handleTokenSubmit} className="space-y-4">
            <div>
              <label className="text-caption font-semibold text-deep-ink block mb-1">
                {guide.tokenName} <span className="text-red-500">*</span>
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
                    className="text-deep-ink font-semibold underline hover:text-slate inline-flex items-center gap-1 mt-1 font-medium"
                  >
                    <span>{t('ui.generateTokenOn', { connector: connector.name })}</span>
                    <ExternalLink className="w-3 h-3" />
                  </a>
                </div>
              </div>
            )}

            <Button
              type="submit"
              variant="primary"
              size="md"
              icon={<Lock className="w-4 h-4" />}
              disabled={connecting || !directToken.trim()}
              className="w-full justify-center py-2.5"
            >
              {connecting ? 'Verifying & Encrypting Token...' : 'Verify & Store in Vault'}
            </Button>
          </form>
        )}
      </div>
    </Modal>
  );
}

import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Input } from '@/components/ui/Input';
import {
  Shield,
  ArrowRight,
  Info,
  ExternalLink,
  Lock,
} from 'lucide-react';
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
  linear: {
    tokenName: 'Linear Personal API Key',
    tokenPlaceholder: 'lin_api_xxxxxxxxxxxxxxxxxxxx',
    tokenHelpUrl: 'https://linear.app/settings/api',
    tokenHelpText: 'Create a personal API key from Account Settings > API.',
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
  const [showAdvancedOAuth, setShowAdvancedOAuth] = useState(false);

  useEffect(() => {
    if (connector) {
      setAuthMode('oauth');
      setDirectToken('');
      setCustomClientID(connector.client_id || '');
      setCustomClientSecret(connector.client_secret || '');
      setShowAdvancedOAuth(false);
    }
  }, [connector, isOpen]);

  if (!connector) return null;

  const guide = PROVIDER_GUIDES[connector.id] || {
    tokenName: 'API Token / Bearer Key',
    tokenPlaceholder: 'Enter secret token...',
    tokenHelpUrl: '',
    tokenHelpText: 'Provide an authorized secret token with sufficient permissions.',
  };

  const handleOAuthSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    onStartOAuth(connector, customClientID.trim() || undefined, customClientSecret.trim() || undefined);
  };

  const handleTokenSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (!directToken.trim()) return;
    onConnectWithToken(connector, directToken.trim());
  };

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

        {/* TAB 1: OAuth 2.1 PKCE Flow */}
        {authMode === 'oauth' && (
          <form onSubmit={handleOAuthSubmit} className="space-y-4">
            <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 text-body-sm text-deep-ink">
              <div className="flex items-center gap-2 mb-1.5 font-semibold">
                <Shield className="w-4 h-4 text-emerald-600" />
                <span>{t('ui.browserAuthorization')}</span>
              </div>
              <p className="text-caption text-slate leading-relaxed">
                {t('ui.browserAuthorizationHelp', { connector: connector.name })}
              </p>
            </div>

            {/* Custom OAuth Client Credentials Accordion */}
            <div className="pt-1">
              <button
                type="button"
                onClick={() => setShowAdvancedOAuth(!showAdvancedOAuth)}
                className="text-[11px] font-semibold uppercase tracking-wider text-slate hover:text-deep-ink transition-colors flex items-center gap-1 cursor-pointer"
              >
                <span>
                  {showAdvancedOAuth
                    ? '▾ Hide Custom Client Credentials'
                    : '▸ Custom OAuth App Credentials (Optional)'}
                </span>
              </button>

              {showAdvancedOAuth && (
                <div className="mt-3 p-4 rounded-2xl bg-canvas border border-onyx/10 space-y-3">
                  <div>
                    <label className="text-caption font-semibold text-deep-ink block mb-1">
                      {t('ui.clientId')}
                    </label>
                    <Input
                      placeholder={t('ui.clientIdPlaceholder')}
                      value={customClientID}
                      onChange={(e) => setCustomClientID(e.target.value)}
                    />
                  </div>
                  <div>
                    <label className="text-caption font-semibold text-deep-ink block mb-1">
                      {t('ui.clientSecret')}
                    </label>
                    <Input
                      type="password"
                      placeholder={t('ui.clientSecretPlaceholder')}
                      value={customClientSecret}
                      onChange={(e) => setCustomClientSecret(e.target.value)}
                    />
                  </div>
                </div>
              )}
            </div>

            <Button
              type="submit"
              variant="primary"
              size="md"
              icon={<ArrowRight className="w-4 h-4" />}
              disabled={connecting}
              className="w-full justify-center py-2.5"
            >
              {connecting ? 'Redirecting to Provider...' : `Continue with ${connector.name}`}
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

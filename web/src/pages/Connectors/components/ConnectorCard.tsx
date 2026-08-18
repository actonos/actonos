import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Card } from '@/components/ui/Card';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import {
  Mail,
  BookOpen,
  Github,
  MessageCircle,
  CheckCircle2,
  XCircle,
  RefreshCw,
  Zap,
  LogOut,
  Sliders,
  Shield,
  Layers,
  Database,
} from 'lucide-react';
import type { ConnectorInfo } from '@/lib/types';

interface ConnectorCardProps {
  connector: ConnectorInfo;
  isTesting: boolean;
  testResult?: { success: boolean; latency: number; msg: string };
  onConnect: (connector: ConnectorInfo) => void;
  onTest: (connectorId: string) => void;
  onDisconnect: (connector: ConnectorInfo) => void;
  isComingSoon?: boolean;
}

export function ConnectorCard({
  connector,
  isTesting,
  testResult,
  onConnect,
  onTest,
  onDisconnect,
  isComingSoon,
}: ConnectorCardProps) {
  const { t } = useTranslation('connectors');
  const [showAllScopes, setShowAllScopes] = useState(false);

  const getServiceIcon = (iconName: string) => {
    switch (iconName) {
      case 'mail':
        return <Mail className="w-5 h-5 text-deep-ink" />;
      case 'book-open':
        return <BookOpen className="w-5 h-5 text-deep-ink" />;
      case 'github':
        return <Github className="w-5 h-5 text-deep-ink" />;
      case 'message-circle':
        return <MessageCircle className="w-5 h-5 text-deep-ink" />;
      case 'database':
        return <Database className="w-5 h-5 text-deep-ink" />;
      default:
        return <Layers className="w-5 h-5 text-deep-ink" />;
    }
  };

  const scopes = connector.scopes || [];
  const displayedScopes = showAllScopes ? scopes : scopes.slice(0, 3);
  const remainingScopesCount = scopes.length - 3;

  return (
    <Card
      className={`flex flex-col justify-between border p-6 transition-all duration-200 ${
        isComingSoon
          ? 'border-onyx/10 border-dashed bg-canvas/70'
          : connector.connected
          ? 'border-emerald-500/30 bg-canvas/95 shadow-xs'
          : 'border-onyx/10 bg-canvas/85 hover:border-onyx/20'
      }`}
    >
      <div>
        {/* Top bar: Icon, Badges */}
        <div className="flex items-start justify-between gap-3 mb-3">
          <div className="w-11 h-11 rounded-full bg-soft-meadow flex items-center justify-center border border-onyx/10 shadow-xs shrink-0">
            {getServiceIcon(connector.icon)}
          </div>

          <div className="flex items-center gap-1.5 flex-wrap justify-end">
            <Badge
              variant={
                connector.risk_level === 'High'
                  ? 'accent'
                  : connector.risk_level === 'Medium'
                  ? 'neutral'
                  : 'neutral'
              }
              className="text-[10px]"
            >
              {t(`risk.${connector.risk_level.toLowerCase()}`, `${connector.risk_level} Risk`)}
            </Badge>

            {isComingSoon ? (
              <Badge variant="neutral" className="text-[10px]">
                {t('status.comingSoon', 'Coming Soon')}
              </Badge>
            ) : connector.connected ? (
              <Badge variant="active" className="text-[10px]">
                <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 mr-1 animate-pulse"></span>
                <span>{t('status.connected', 'Connected')}</span>
              </Badge>
            ) : (
              <Badge variant="stopped" className="text-[10px]">
                {t('status.disconnected', 'Available')}
              </Badge>
            )}
          </div>
        </div>

        {/* Title, Category & Description */}
        <div className="mb-3">
          <div className="flex items-baseline justify-between gap-2">
            <h3 className="font-serif text-heading-sm text-deep-ink">
              {connector.name}
            </h3>
            <span className="text-[11px] font-mono text-slate uppercase tracking-wider">
              {connector.category}
            </span>
          </div>
          <p className="font-sans text-body-sm text-slate mt-1 line-clamp-2 leading-relaxed">
            {connector.description}
          </p>
        </div>

        {/* Scopes Tag List */}
        {scopes.length > 0 && (
          <div className="mb-4">
            <div className="flex flex-wrap items-center gap-1">
              {displayedScopes.map((scope) => (
                <span
                  key={scope}
                  className="text-[10px] font-mono bg-soft-meadow/80 text-deep-ink px-2 py-0.5 rounded-md border border-onyx/5 max-w-[180px] truncate"
                  title={scope}
                >
                  {scope.split('/').pop() || scope}
                </span>
              ))}
              {remainingScopesCount > 0 && (
                <button
                  type="button"
                  onClick={() => setShowAllScopes(!showAllScopes)}
                  className="text-[10px] font-mono text-slate hover:text-deep-ink font-semibold px-1.5 py-0.5 rounded cursor-pointer"
                >
                  {showAllScopes ? 'Show less' : `+${remainingScopesCount} more`}
                </button>
              )}
            </div>
          </div>
        )}

        {/* Connected Identity Panel */}
        {connector.connected && (
          <div className="p-3.5 rounded-2xl bg-soft-meadow border border-onyx/10 mb-4 space-y-2 text-caption">
            <div className="flex items-center justify-between">
              <span className="text-slate">{t('details.account', 'Account')}:</span>
              <span className="font-semibold text-deep-ink font-mono truncate max-w-[180px]">
                {connector.account_name || connector.account_email || 'Authorized User'}
              </span>
            </div>

            {connector.account_email && connector.account_name !== connector.account_email && (
              <div className="flex items-center justify-between">
                <span className="text-slate">{t('details.email', 'Email')}:</span>
                <span className="font-mono text-deep-ink truncate max-w-[180px]">
                  {connector.account_email}
                </span>
              </div>
            )}

            <div className="flex items-center justify-between">
              <span className="text-slate">{t('details.authMethod', 'Method')}:</span>
              <span className="font-mono uppercase text-[10px] bg-canvas px-2 py-0.5 rounded border border-onyx/5 text-deep-ink">
                {connector.auth_type === 'oauth' ? 'OAuth 2.1 PKCE' : 'Personal Token'}
              </span>
            </div>

            {/* Test Connection Result Display */}
            {testResult && (
              <div
                className={`flex items-center gap-1.5 pt-1 text-[11px] font-mono border-t border-onyx/5 mt-1.5 ${
                  testResult.success ? 'text-emerald-700 font-semibold' : 'text-red-600'
                }`}
              >
                {testResult.success ? (
                  <CheckCircle2 className="w-3.5 h-3.5 shrink-0" />
                ) : (
                  <XCircle className="w-3.5 h-3.5 shrink-0" />
                )}
                <span className="truncate">{testResult.msg}</span>
              </div>
            )}
          </div>
        )}
      </div>

      {/* Footer Actions */}
      <div className="pt-4 border-t border-onyx/10 flex items-center justify-between gap-2 mt-2">
        <span className="text-[11px] font-mono text-slate flex items-center gap-1">
          <Shield className="w-3 h-3 text-emerald-600" />
          <span>{t('ui.encryptedEnclave')}</span>
        </span>

        <div className="flex items-center gap-1.5">
          {isComingSoon ? (
            <Button variant="ghost" size="sm" disabled className="text-[11px]">
              {t('status.comingSoon')}
            </Button>
          ) : connector.connected ? (
            <>
              <Button
                variant="ghost"
                size="sm"
                icon={<RefreshCw className={`w-3.5 h-3.5 ${isTesting ? 'animate-spin' : ''}`} />}
                onClick={() => onTest(connector.id)}
                disabled={isTesting}
                className="text-[11px]"
              >
                {isTesting ? 'Testing...' : t('actions.test', 'Test')}
              </Button>
              <Button
                variant="ghost"
                size="sm"
                icon={<Sliders className="w-3.5 h-3.5" />}
                onClick={() => onConnect(connector)}
                className="text-[11px]"
              >
                {t('actions.reauth', 'Config')}
              </Button>
              <Button
                variant="danger"
                size="sm"
                icon={<LogOut className="w-3.5 h-3.5" />}
                onClick={() => onDisconnect(connector)}
                className="text-[11px]"
              >
                {t('actions.disconnect', 'Disconnect')}
              </Button>
            </>
          ) : (
            <Button
              variant="primary"
              size="sm"
              icon={<Zap className="w-3.5 h-3.5" />}
              onClick={() => onConnect(connector)}
              className="text-[11px]"
            >
              {t('actions.connect', 'Connect')}
            </Button>
          )}
        </div>
      </div>
    </Card>
  );
}

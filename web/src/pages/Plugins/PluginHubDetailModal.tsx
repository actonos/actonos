import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import type { RegistryPlugin } from '@/lib/types';
import {
  Globe,
  Key,
  Database,
  Radio,
  Network,
  Wrench,
  Copy,
  Check,
  ShieldCheck,
  Code2,
  Sliders,
  Download,
  CheckCircle2,
  Star,
  Layers,
  Bot,
} from 'lucide-react';

export interface PluginHubDetailModalProps {
  plugin: RegistryPlugin | null;
  isOpen: boolean;
  onClose: () => void;
  onInstall: (pluginId: string, displayName?: string, downloadUrl?: string) => void;
  isInstalling?: boolean;
}

export function PluginHubDetailModal({
  plugin,
  isOpen,
  onClose,
  onInstall,
  isInstalling = false,
}: PluginHubDetailModalProps) {
  const { t } = useTranslation('plugins');
  const [copied, setCopied] = useState(false);
  const [activeTab, setActiveTab] = useState<'overview' | 'tools' | 'config' | 'raw'>('overview');

  useEffect(() => {
    if (isOpen) {
      setActiveTab('overview');
    }
  }, [isOpen]);

  if (!plugin) return null;

  const permissions = plugin.permissions || {};
  const tools = plugin.tools || [];
  const channels = plugin.channels || [];
  const connectors = plugin.connectors || [];
  const capabilities = plugin.capabilities || [];
  const configSchemaObj = plugin.config_schema as { properties?: Record<string, unknown> } | undefined;
  const hasConfig = Boolean(
    configSchemaObj &&
    configSchemaObj.properties &&
    Object.keys(configSchemaObj.properties).length > 0
  );

  const handleCopyJson = () => {
    navigator.clipboard.writeText(JSON.stringify(plugin, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={plugin.name || plugin.id}
      maxWidth="max-w-3xl"
    >
      <div className="space-y-6">
        {/* Meta Header */}
        <div className="-mt-3 mb-2 flex flex-wrap items-center justify-between gap-2 font-mono text-caption text-slate">
          <div className="flex items-center gap-2 flex-wrap">
            <span className="text-deep-ink font-semibold">{`@${plugin.id}`}</span>
            <span>{`• v${plugin.version || '1.0.0'}`}</span>
            {plugin.author && <span>{`• ${t('hub.byAuthor', { author: plugin.author })}`}</span>}
          </div>
          <div className="flex items-center gap-2">
            {plugin.stars ? (
              <span className="inline-flex items-center gap-1 text-[11px] font-mono font-medium text-amber-900 bg-amber-500/10 px-2 py-0.5 rounded-full border border-amber-500/20">
                <Star className="w-3 h-3 fill-amber-500 text-amber-500" />
                {plugin.stars}
              </span>
            ) : null}
            {plugin.installed ? (
              <Badge variant="active" className="flex items-center gap-1 text-[11px]">
                <CheckCircle2 className="w-3 h-3" />
                {t('hub.installed', 'Installed')}
              </Badge>
            ) : null}
          </div>
        </div>

        {/* Navigation Tabs Pill Control */}
        <div className="flex max-w-full items-center gap-1 overflow-x-auto rounded-full border border-onyx/10 bg-soft-meadow p-1">
          <button
            type="button"
            onClick={() => setActiveTab('overview')}
            className={`shrink-0 rounded-full px-4 py-2 text-caption font-semibold transition-colors flex items-center gap-1.5 focus-visible:outline-none cursor-pointer ${
              activeTab === 'overview'
                ? 'bg-deep-ink text-canvas shadow-xs'
                : 'text-slate hover:bg-canvas hover:text-deep-ink'
            }`}
          >
            <ShieldCheck className="h-3.5 w-3.5" />
            <span>{t('hub.modal.overview', 'Overview & Permissions')}</span>
          </button>

          {(tools.length > 0 || channels.length > 0 || connectors.length > 0) && (
            <button
              type="button"
              onClick={() => setActiveTab('tools')}
              className={`shrink-0 rounded-full px-4 py-2 text-caption font-semibold transition-colors flex items-center gap-1.5 focus-visible:outline-none cursor-pointer ${
                activeTab === 'tools'
                  ? 'bg-deep-ink text-canvas shadow-xs'
                  : 'text-slate hover:bg-canvas hover:text-deep-ink'
              }`}
            >
              <Bot className="h-3.5 w-3.5" />
              <span>
                {t('hub.modal.capabilities', 'Capabilities')} ({tools.length + channels.length + connectors.length})
              </span>
            </button>
          )}

          {hasConfig && (
            <button
              type="button"
              onClick={() => setActiveTab('config')}
              className={`shrink-0 rounded-full px-4 py-2 text-caption font-semibold transition-colors flex items-center gap-1.5 focus-visible:outline-none cursor-pointer ${
                activeTab === 'config'
                  ? 'bg-deep-ink text-canvas shadow-xs'
                  : 'text-slate hover:bg-canvas hover:text-deep-ink'
              }`}
            >
              <Sliders className="h-3.5 w-3.5" />
              <span>{t('hub.modal.configSchema', 'Config Schema')}</span>
            </button>
          )}

          <button
            type="button"
            onClick={() => setActiveTab('raw')}
            className={`shrink-0 rounded-full px-4 py-2 text-caption font-semibold transition-colors flex items-center gap-1.5 focus-visible:outline-none cursor-pointer ${
              activeTab === 'raw'
                ? 'bg-deep-ink text-canvas shadow-xs'
                : 'text-slate hover:bg-canvas hover:text-deep-ink'
            }`}
          >
            <Code2 className="h-3.5 w-3.5" />
            <span>{t('hub.modal.rawManifest', 'Registry JSON')}</span>
          </button>
        </div>

        {/* Tab 1: Overview & Permissions */}
        {activeTab === 'overview' && (
          <div className="space-y-6">
            <div>
              <h4 className="text-body-sm font-semibold text-deep-ink mb-1.5">{t('hub.modal.about', 'About Plugin')}</h4>
              <p className="text-body-sm text-slate leading-relaxed">
                {plugin.description || t('hub.modal.noDesc', 'Official WebAssembly plugin extension for ActonOS.')}
              </p>
            </div>

            {/* Capability Badges */}
            {capabilities.length > 0 && (
              <div>
                <h4 className="text-caption font-semibold uppercase tracking-wider text-slate mb-2">
                  {t('hub.modal.capabilityTypes', 'Declared Capabilities')}
                </h4>
                <div className="flex flex-wrap gap-2">
                  {capabilities.map((cap) => (
                    <span
                      key={cap}
                      className={`inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-caption font-semibold uppercase tracking-wider ${
                        cap === 'channel'
                          ? 'bg-purple-500/15 text-purple-700'
                          : cap === 'connector'
                            ? 'bg-blue-500/15 text-blue-700'
                            : 'bg-emerald-500/15 text-emerald-700'
                      }`}
                    >
                      {cap === 'channel' ? (
                        <Radio className="w-3.5 h-3.5" />
                      ) : cap === 'connector' ? (
                        <Network className="w-3.5 h-3.5" />
                      ) : (
                        <Wrench className="w-3.5 h-3.5" />
                      )}
                      {t(`capabilities.${cap}`, cap)}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Tags */}
            {plugin.tags && plugin.tags.length > 0 && (
              <div>
                <h4 className="text-caption font-semibold uppercase tracking-wider text-slate mb-2">
                  {t('hub.modal.tags', 'Tags')}
                </h4>
                <div className="flex flex-wrap gap-1.5">
                  {plugin.tags.map((tag) => (
                    <span
                      key={tag}
                      className="px-2.5 py-0.5 rounded-full bg-soft-meadow border border-onyx/10 text-caption font-mono text-slate"
                    >
                      #{tag}
                    </span>
                  ))}
                </div>
              </div>
            )}

            {/* Sandboxed Permissions */}
            <div>
              <h4 className="text-caption font-semibold uppercase tracking-wider text-slate mb-3 flex items-center gap-2">
                <ShieldCheck className="w-4 h-4 text-deep-ink" />
                <span>{t('permissions.title', 'Sandboxed Security Permissions')}</span>
              </h4>
              <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
                {/* Outbound Network */}
                <div className="p-4 rounded-[20px] bg-soft-meadow/70 border border-onyx/10 space-y-1.5">
                  <div className="flex items-center gap-2 text-body-sm font-semibold text-deep-ink">
                    <Globe className="w-4 h-4 text-blue-600" />
                    <span>{t('permissions.netOutbound', 'Outbound Domains')}</span>
                  </div>
                  {permissions.net_outbound?.length ? (
                    <div className="flex flex-wrap gap-1 mt-1">
                      {permissions.net_outbound.map((domain) => (
                        <span
                          key={domain}
                          className="px-2 py-0.5 rounded-full bg-canvas border border-onyx/10 text-[11px] font-mono text-deep-ink"
                        >
                          {domain}
                        </span>
                      ))}
                    </div>
                  ) : (
                    <p className="text-caption text-slate">{t('hub.modal.noEgress', 'Network Egress Blocked (None)')}</p>
                  )}
                </div>

                {/* Vault Secrets */}
                <div className="p-4 rounded-[20px] bg-soft-meadow/70 border border-onyx/10 space-y-1.5">
                  <div className="flex items-center gap-2 text-body-sm font-semibold text-deep-ink">
                    <Key className="w-4 h-4 text-amber-600" />
                    <span>{t('permissions.secrets', 'Vault Secrets')}</span>
                  </div>
                  {permissions.secrets?.length ? (
                    <div className="flex flex-wrap gap-1 mt-1">
                      {permissions.secrets.map((secret) => (
                        <span
                          key={secret}
                          className="px-2 py-0.5 rounded-full bg-canvas border border-onyx/10 text-[11px] font-mono text-deep-ink"
                        >
                          {secret}
                        </span>
                      ))}
                    </div>
                  ) : (
                    <p className="text-caption text-slate">{t('hub.modal.noSecrets', 'No Vault Secrets Required')}</p>
                  )}
                </div>

                {/* Storage */}
                <div className="p-4 rounded-[20px] bg-soft-meadow/70 border border-onyx/10 space-y-1.5">
                  <div className="flex items-center gap-2 text-body-sm font-semibold text-deep-ink">
                    <Database className="w-4 h-4 text-emerald-600" />
                    <span>{t('permissions.storage', 'Storage')}</span>
                  </div>
                  <p className="text-caption text-slate">
                    {permissions.storage
                      ? t('hub.modal.storageGranted', 'Enabled')
                      : t('hub.modal.storageNone', 'None')}
                  </p>
                </div>

                {/* Event Bus Topics */}
                <div className="p-4 rounded-[20px] bg-soft-meadow/70 border border-onyx/10 space-y-1.5">
                  <div className="flex items-center gap-2 text-body-sm font-semibold text-deep-ink">
                    <Layers className="w-4 h-4 text-purple-600" />
                    <span>{t('permissions.busEvents', 'Event Bus Topics')}</span>
                  </div>
                  {permissions.bus_events?.length ? (
                    <div className="flex flex-wrap gap-1 mt-1">
                      {permissions.bus_events.map((topic) => (
                        <span
                          key={topic}
                          className="px-2 py-0.5 rounded-full bg-canvas border border-onyx/10 text-[11px] font-mono text-deep-ink"
                        >
                          {topic}
                        </span>
                      ))}
                    </div>
                  ) : (
                    <p className="text-caption text-slate">{t('hub.modal.busNone', 'Standard event bus pipeline')}</p>
                  )}
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Tab 2: Exported Tools / Channels / Connectors */}
        {activeTab === 'tools' && (
          <div className="space-y-4 max-h-[420px] overflow-y-auto pr-1">
            {tools.length > 0 && (
              <div>
                <h4 className="text-caption font-semibold uppercase tracking-wider text-slate mb-2">
                  {t('modals.toolsExported', { count: tools.length })}
                </h4>
                <div className="space-y-3">
                  {tools.map((tool) => (
                    <div
                      key={tool.name}
                      className="p-4 rounded-[18px] bg-soft-meadow/80 border border-onyx/10 space-y-2"
                    >
                      <div className="flex items-center justify-between">
                        <span className="font-mono text-body-sm font-bold text-deep-ink">{tool.name}</span>
                        <span className="px-2 py-0.5 rounded-full bg-canvas text-[11px] font-mono text-slate border border-onyx/10">
                          {t('capabilities.tool', 'Tool Extension')}
                        </span>
                      </div>
                      <p className="text-caption text-slate">{tool.description}</p>
                      {tool.parameters && Object.keys(tool.parameters).length > 0 && (
                        <details className="mt-2 text-[11px] font-mono text-slate">
                          <summary className="cursor-pointer font-sans font-semibold text-deep-ink hover:underline">
                            {t('hub.modal.viewSchema', 'View Parameters Schema')}
                          </summary>
                          <pre className="mt-1 p-2 rounded-xl bg-canvas border border-onyx/10 overflow-x-auto text-[11px]">
                            {JSON.stringify(tool.parameters, null, 2)}
                          </pre>
                        </details>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {channels.length > 0 && (
              <div>
                <h4 className="text-caption font-semibold uppercase tracking-wider text-slate mb-2">
                  {t('stats.channels', 'Chat Channels')}
                </h4>
                <div className="space-y-2">
                  {channels.map((ch) => (
                    <div
                      key={ch.name}
                      className="p-3.5 rounded-[18px] bg-soft-meadow/80 border border-onyx/10 flex items-center justify-between"
                    >
                      <div>
                        <p className="font-semibold text-body-sm text-deep-ink">{ch.display_name || ch.name}</p>
                        <p className="font-mono text-[11px] text-slate">{t('hub.modal.channelId', 'channel_id')}: {ch.name}</p>
                      </div>
                      {ch.requires_pairing && (
                        <Badge variant="neutral" className="text-[10px]">
                          {t('hub.modal.requiresPairing', 'Requires Pairing Code')}
                        </Badge>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {connectors.length > 0 && (
              <div>
                <h4 className="text-caption font-semibold uppercase tracking-wider text-slate mb-2">
                  {t('stats.connectors', 'SaaS Connectors')}
                </h4>
                <div className="space-y-2">
                  {connectors.map((c) => (
                    <div
                      key={c.name}
                      className="p-3.5 rounded-[18px] bg-soft-meadow/80 border border-onyx/10 space-y-1.5"
                    >
                      <div className="flex items-center justify-between">
                        <p className="font-semibold text-body-sm text-deep-ink">{c.display_name || c.name}</p>
                        {c.auth_type && (
                          <span className="px-2 py-0.5 rounded-full bg-canvas text-[11px] font-mono text-slate border border-onyx/10">
                            {t('hub.modal.authType', 'auth')}: {c.auth_type}
                          </span>
                        )}
                      </div>
                      {c.actions && c.actions.length > 0 && (
                        <div className="flex flex-wrap gap-1">
                          {c.actions.map((act) => (
                            <span
                              key={act}
                              className="px-2 py-0.5 rounded-md bg-canvas text-[10px] font-mono text-deep-ink border border-onyx/5"
                            >
                              {act}
                            </span>
                          ))}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}
          </div>
        )}

        {/* Tab 3: Configuration Schema */}
        {activeTab === 'config' && (
          <div className="space-y-4">
            <p className="text-body-sm text-slate">
              {t('config.bannerNotice', 'This plugin declares the following configuration and credential parameters:')}
            </p>
            <div className="p-4 rounded-[20px] bg-soft-meadow/70 border border-onyx/10">
              <pre className="font-mono text-[11px] text-deep-ink overflow-x-auto max-h-[300px] leading-relaxed">
                {JSON.stringify(plugin.config_schema, null, 2)}
              </pre>
            </div>
          </div>
        )}

        {/* Tab 4: Raw JSON */}
        {activeTab === 'raw' && (
          <div className="relative">
            <button
              type="button"
              onClick={handleCopyJson}
              className="absolute right-3 top-3 flex items-center gap-1 rounded-full border border-onyx/10 bg-canvas px-3 py-1.5 text-caption font-semibold text-deep-ink hover:bg-soft-meadow cursor-pointer transition-colors shadow-2xs"
            >
              {copied ? <Check className="h-3.5 w-3.5 text-emerald-600" /> : <Copy className="h-3.5 w-3.5" />}
              <span>{copied ? t('actions.copied', 'Copied') : t('actions.copy', 'Copy')}</span>
            </button>
            <pre className="max-h-[350px] overflow-auto rounded-[20px] border border-onyx/10 bg-canvas p-4 font-mono text-[11px] text-deep-ink leading-relaxed">
              {JSON.stringify(plugin, null, 2)}
            </pre>
          </div>
        )}

        {/* Footer Actions */}
        <div className="flex items-center justify-between pt-4 border-t border-onyx/10">
          <Button variant="ghost" size="sm" onClick={onClose}>
            {t('actions.cancel', 'Close')}
          </Button>

          {plugin.installed ? (
            <div className="flex items-center gap-2">
              <Badge variant="active" className="py-1 px-3 text-caption flex items-center gap-1.5">
                <CheckCircle2 className="w-3.5 h-3.5" />
                <span>{t('hub.installed', 'Installed')}</span>
              </Badge>
            </div>
          ) : (
            <Button
              variant="primary"
              size="sm"
              icon={<Download className="w-4 h-4" />}
              disabled={isInstalling}
              onClick={() => onInstall(plugin.id, plugin.name, plugin.download_url || plugin.url)}
            >
              {isInstalling ? t('hub.installing', 'Installing...') : t('hub.install', 'Install Plugin')}
            </Button>
          )}
        </div>
      </div>
    </Modal>
  );
}

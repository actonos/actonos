import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import type { PluginInfo } from '@/lib/types';
import { PluginConfigForm } from './PluginConfigForm';
import {
  Globe,
  Key,
  Database,
  Radio,
  Wrench,
  Copy,
  Check,
  ShieldCheck,
  Code2,
  Sliders,
  Settings2,
} from 'lucide-react';

export interface PluginDetailModalProps {
  plugin: PluginInfo | null;
  isOpen: boolean;
  onClose: () => void;
  onPluginUpdated?: (plugin: PluginInfo) => void;
  initialTab?: 'overview' | 'config' | 'tools' | 'raw';
}

export function PluginDetailModal({
  plugin,
  isOpen,
  onClose,
  onPluginUpdated,
  initialTab = 'overview',
}: PluginDetailModalProps) {
  const { t } = useTranslation('plugins');
  const [copied, setCopied] = useState(false);
  const [activeTab, setActiveTab] = useState<'overview' | 'config' | 'tools' | 'raw'>(initialTab);

  useEffect(() => {
    if (isOpen) {
      setActiveTab(initialTab);
    }
  }, [isOpen, initialTab]);

  if (!plugin) return null;

  const manifest = plugin.manifest;
  const permissions = manifest.permissions || {};
  const tools = manifest.tools || [];
  const channels = manifest.channels || [];
  const connectors = manifest.connectors || [];
  const hasConfig = Boolean(
    manifest.config_schema &&
    manifest.config_schema.properties &&
    Object.keys(manifest.config_schema.properties).length > 0
  );

  const handleCopyManifest = () => {
    navigator.clipboard.writeText(JSON.stringify(manifest, null, 2));
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={manifest.name || manifest.id}
      maxWidth="max-w-3xl"
    >
      <div className="space-y-6">
        {/* Meta Header */}
        <div className="-mt-3 mb-2 flex items-center justify-between font-mono text-caption text-slate">
          <div>
            <span className="text-deep-ink font-semibold">{manifest.id}</span> • v{manifest.version || '1.0.0'}
            {manifest.author && <span> • {t('hub.byAuthor', { author: manifest.author })}</span>}
          </div>
          {manifest.license && (
            <span className="px-2.5 py-0.5 rounded-full bg-soft-meadow border border-onyx/10 text-[11px] font-semibold text-deep-ink">
              {manifest.license}
            </span>
          )}
        </div>

        {/* Navigation Tabs Pill Control */}
        <div className="flex max-w-full items-center gap-1 overflow-x-auto rounded-full border border-onyx/10 bg-soft-meadow p-1">
          <button
            type="button"
            onClick={() => setActiveTab('overview')}
            className={`shrink-0 rounded-full px-4 py-2 text-caption font-semibold transition-colors flex items-center gap-1.5 focus-visible:outline-none ${
              activeTab === 'overview'
                ? 'bg-deep-ink text-canvas shadow-xs'
                : 'text-slate hover:bg-canvas hover:text-deep-ink'
            }`}
          >
            <ShieldCheck className="h-3.5 w-3.5" />
            <span>{t('tabs.overview')}</span>
          </button>

          <button
            type="button"
            onClick={() => setActiveTab('config')}
            className={`shrink-0 rounded-full px-4 py-2 text-caption font-semibold transition-colors flex items-center gap-1.5 focus-visible:outline-none ${
              activeTab === 'config'
                ? 'bg-deep-ink text-canvas shadow-xs'
                : 'text-slate hover:bg-canvas hover:text-deep-ink'
            }`}
          >
            <Sliders className="h-3.5 w-3.5" />
            <span>{t('tabs.config')}</span>
            {hasConfig && (
              <span className="w-2 h-2 rounded-full bg-status-warning" />
            )}
          </button>

          <button
            type="button"
            onClick={() => setActiveTab('tools')}
            className={`shrink-0 rounded-full px-4 py-2 text-caption font-semibold transition-colors flex items-center gap-1.5 focus-visible:outline-none ${
              activeTab === 'tools'
                ? 'bg-deep-ink text-canvas shadow-xs'
                : 'text-slate hover:bg-canvas hover:text-deep-ink'
            }`}
          >
            <Wrench className="h-3.5 w-3.5" />
            <span>{t('tabs.tools')} ({tools.length})</span>
          </button>

          <button
            type="button"
            onClick={() => setActiveTab('raw')}
            className={`shrink-0 rounded-full px-4 py-2 text-caption font-semibold transition-colors flex items-center gap-1.5 focus-visible:outline-none ${
              activeTab === 'raw'
                ? 'bg-deep-ink text-canvas shadow-xs'
                : 'text-slate hover:bg-canvas hover:text-deep-ink'
            }`}
          >
            <Code2 className="h-3.5 w-3.5" />
            <span>{t('tabs.manifest')}</span>
          </button>
        </div>

        {/* Tab 1: Overview & Permissions */}
        {activeTab === 'overview' && (
          <div className="space-y-6">
            {manifest.description && (
              <p className="text-body leading-relaxed text-slate">
                {manifest.description}
              </p>
            )}

            {/* Subtle banner to configure */}
            {hasConfig && (
              <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 flex flex-col sm:flex-row sm:items-center justify-between gap-3 shadow-2xs">
                <div className="flex items-center gap-2.5 text-caption text-deep-ink font-medium">
                  <Settings2 className="w-4 h-4 text-slate shrink-0" />
                  <span>{t('config.bannerNotice', 'This plugin has configurable parameters and credentials.')}</span>
                </div>
                <Button
                  size="sm"
                  variant="secondary"
                  icon={<Sliders className="w-3.5 h-3.5" />}
                  onClick={() => setActiveTab('config')}
                  className="shrink-0 self-start sm:self-center"
                >
                  {t('config.configureNow', 'Configure')}
                </Button>
              </div>
            )}

            {/* Capabilities */}
            <div>
              <span className="mb-2.5 block text-caption font-semibold uppercase tracking-wider text-slate">
                {t('modals.capabilities')}
              </span>
              <div className="flex flex-wrap gap-2">
                {(manifest.capabilities || []).map((cap) => (
                  <Badge
                    key={cap}
                    variant={
                      cap === 'channel'
                        ? 'accent'
                        : cap === 'connector'
                        ? 'info'
                        : 'success'
                    }
                  >
                    {t(`capabilities.${cap}`, cap)}
                  </Badge>
                ))}
              </div>
            </div>

            {/* Channels exported */}
            {channels.length > 0 && (
              <div>
                <span className="mb-2.5 block text-caption font-semibold uppercase tracking-wider text-slate">
                  {t('modals.channels')}
                </span>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {channels.map((ch) => (
                    <div key={ch.name} className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 flex items-center justify-between">
                      <div>
                        <div className="font-serif font-bold text-body-sm text-deep-ink">{ch.display_name}</div>
                        <div className="text-caption font-mono text-slate mt-0.5">@{ch.name}</div>
                      </div>
                      {ch.requires_pairing && (
                        <Badge variant="accent">{t('modals.pairingCode')}</Badge>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Connectors exported */}
            {connectors.length > 0 && (
              <div>
                <span className="mb-2.5 block text-caption font-semibold uppercase tracking-wider text-slate">
                  {t('modals.connectors')}
                </span>
                <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                  {connectors.map((conn) => (
                    <div key={conn.name} className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-1.5">
                      <div className="flex items-center justify-between">
                        <span className="font-serif font-bold text-body-sm text-deep-ink">{conn.display_name}</span>
                        <span className="text-[10px] font-mono uppercase bg-canvas text-slate px-2 py-0.5 rounded-full border border-onyx/10 font-semibold">
                          {conn.auth_type || 'oauth2'}
                        </span>
                      </div>
                      {conn.actions && (
                        <div className="text-[11px] text-slate font-mono">
                          {t('modals.connectorActions', { list: conn.actions.join(', ') })}
                        </div>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Sandboxed Permissions Checklist */}
            <div>
              <span className="mb-3 block text-caption font-semibold uppercase tracking-wider text-slate">
                {t('permissions.title', 'Sandboxed Permissions')}
              </span>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {/* Outbound Network Domains */}
                <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-2">
                  <div className="flex items-center gap-2 font-semibold text-deep-ink text-body-sm">
                    <Globe className="h-4 w-4 text-slate" />
                    <span>{t('permissions.netOutbound', 'Outbound Domains')}</span>
                  </div>
                  <div className="flex flex-wrap gap-1.5 pt-1">
                    {(permissions.net_outbound || []).length > 0 ? (
                      permissions.net_outbound?.map((dom) => (
                        <span
                          key={dom}
                          className="inline-flex items-center gap-1 text-caption font-mono text-deep-ink bg-canvas px-2.5 py-1 rounded-full border border-onyx/10 shadow-2xs"
                        >
                          <Globe className="w-3 h-3 text-slate" />
                          <span>{dom}</span>
                        </span>
                      ))
                    ) : (
                      <span className="text-caption text-slate italic">{t('hub.modal.noEgress', 'None')}</span>
                    )}
                  </div>
                </div>

                {/* Vault Secrets */}
                <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-2">
                  <div className="flex items-center justify-between">
                    <div className="flex items-center gap-2 font-semibold text-deep-ink text-body-sm">
                      <Key className="h-4 w-4 text-slate" />
                      <span>{t('permissions.secrets', 'Vault Secrets')}</span>
                    </div>
                    <span className="text-[10px] uppercase font-mono text-slate">{t('modals.encrypted')}</span>
                  </div>
                  <div className="flex flex-wrap gap-1.5 pt-1">
                    {(permissions.secrets || []).length > 0 ? (
                      permissions.secrets?.map((sec) => (
                        <span
                          key={sec}
                          className="inline-flex items-center gap-1.5 text-caption font-mono text-deep-ink bg-canvas px-2.5 py-1 rounded-full border border-onyx/10 shadow-2xs"
                        >
                          <Key className="w-3 h-3 text-slate" />
                          <span>{sec}</span>
                        </span>
                      ))
                    ) : (
                      <span className="text-caption text-slate italic">{t('hub.modal.noSecrets', 'None')}</span>
                    )}
                  </div>
                </div>

                {/* Persistent Storage */}
                <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-2">
                  <div className="flex items-center gap-2 font-semibold text-deep-ink text-body-sm">
                    <Database className="h-4 w-4 text-slate" />
                    <span>{t('permissions.storage', 'Storage')}</span>
                  </div>
                  <div className="pt-1">
                    {permissions.storage ? (
                      <span className="inline-flex items-center gap-1.5 text-caption font-medium text-status-success bg-canvas px-2.5 py-1 rounded-full border border-onyx/10 shadow-2xs">
                        <Check className="h-3.5 w-3.5" /> {t('hub.modal.storageGranted', 'Enabled')}
                      </span>
                    ) : (
                      <span className="text-caption text-slate italic">{t('hub.modal.storageNone', 'None')}</span>
                    )}
                  </div>
                </div>

                {/* Event Bus Topics */}
                <div className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-2">
                  <div className="flex items-center gap-2 font-semibold text-deep-ink text-body-sm">
                    <Radio className="h-4 w-4 text-slate" />
                    <span>{t('permissions.busEvents', 'Event Bus')}</span>
                  </div>
                  <div className="flex flex-wrap gap-1.5 pt-1">
                    {(permissions.bus_events || []).length > 0 ? (
                      permissions.bus_events?.map((ev) => (
                        <span
                          key={ev}
                          className="inline-flex items-center gap-1 text-caption font-mono text-deep-ink bg-canvas px-2.5 py-1 rounded-full border border-onyx/10 shadow-2xs"
                        >
                          <Radio className="w-3 h-3 text-slate" />
                          <span>{ev}</span>
                        </span>
                      ))
                    ) : (
                      <span className="text-caption text-slate italic">{t('hub.modal.busNone', 'Standard')}</span>
                    )}
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Tab 2: Dynamic Schema Configuration */}
        {activeTab === 'config' && (
          <div>
            <PluginConfigForm
              plugin={plugin}
              onSaved={(updated) => {
                if (onPluginUpdated) {
                  onPluginUpdated(updated);
                }
              }}
            />
          </div>
        )}

        {/* Tab 3: Exported Tools */}
        {activeTab === 'tools' && (
          <div className="space-y-4">
            {tools.length > 0 ? (
              <div className="space-y-3.5">
                {tools.map((tool) => (
                  <div key={tool.name} className="p-4 rounded-2xl bg-soft-meadow border border-onyx/10 space-y-2">
                    <div className="flex items-center justify-between">
                      <div className="font-mono text-body-sm font-bold text-deep-ink">
                        {tool.name}
                      </div>
                      <span className="px-2.5 py-0.5 rounded-full bg-canvas border border-onyx/10 font-mono text-[11px] text-slate">
                        {t('modals.toolKind')}
                      </span>
                    </div>
                    {tool.description && (
                      <p className="text-caption text-slate leading-relaxed">{tool.description}</p>
                    )}
                    {(tool.parameters || tool.schema) && (
                      <div className="mt-3">
                        <span className="mb-1.5 block text-[11px] font-semibold uppercase text-slate">
                          {t('modals.parameters')}
                        </span>
                        <pre className="max-h-40 overflow-auto rounded-xl bg-canvas border border-onyx/10 p-3 font-mono text-[11px] text-deep-ink shadow-2xs">
                          {JSON.stringify(tool.parameters || tool.schema, null, 2)}
                        </pre>
                      </div>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <div className="py-8 text-center text-caption text-slate bg-soft-meadow rounded-2xl border border-onyx/10">
                {t('modals.noTools', 'No agent tools exported directly by this plugin.')}
              </div>
            )}
          </div>
        )}

        {/* Tab 4: Raw Manifest */}
        {activeTab === 'raw' && (
          <div className="space-y-3">
            <div className="flex justify-end">
              <Button
                variant="secondary"
                size="sm"
                icon={copied ? <Check className="h-3.5 w-3.5 text-status-success" /> : <Copy className="h-3.5 w-3.5" />}
                onClick={handleCopyManifest}
              >
                {copied ? t('actions.copied') : t('actions.copyJson')}
              </Button>
            </div>
            <pre className="max-h-96 overflow-auto rounded-2xl bg-soft-meadow border border-onyx/10 p-4 font-mono text-caption text-deep-ink leading-relaxed shadow-xs">
              {JSON.stringify(manifest, null, 2)}
            </pre>
          </div>
        )}

        {/* Modal Action Buttons */}
        <div className="flex justify-end border-t border-onyx/10 pt-4">
          <Button variant="ghost" onClick={onClose}>
            {t('actions.close')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

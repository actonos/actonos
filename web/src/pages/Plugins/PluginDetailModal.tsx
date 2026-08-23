import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { Card } from '@/components/ui/Card';
import type { PluginInfo } from '@/lib/types';
import {
  Globe,
  Key,
  Database,
  Radio,
  Wrench,
  Copy,
  Check,
  ShieldCheck,
  Layers,
  Code2,
} from 'lucide-react';

export interface PluginDetailModalProps {
  plugin: PluginInfo | null;
  isOpen: boolean;
  onClose: () => void;
}

export function PluginDetailModal({ plugin, isOpen, onClose }: PluginDetailModalProps) {
  const { t } = useTranslation('plugins');
  const [copied, setCopied] = useState(false);
  const [activeTab, setActiveTab] = useState<'overview' | 'tools' | 'raw'>('overview');

  if (!plugin) return null;

  const manifest = plugin.manifest;
  const permissions = manifest.permissions || {};
  const tools = manifest.tools || [];

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
      <div className="space-y-5">
        <div className="-mt-3 mb-2 font-mono text-caption text-slate">
          ID: {manifest.id} • v{manifest.version || '1.0.0'}
        </div>

        {/* Navigation Tabs */}
        <div className="flex border-b border-onyx/10 pb-2 dark:border-onyx/30">
          <div className="flex gap-2">
            <button
              type="button"
              onClick={() => setActiveTab('overview')}
              className={`flex items-center gap-1.5 rounded-pill px-3 py-1.5 text-caption font-semibold transition-colors ${
                activeTab === 'overview'
                  ? 'bg-deep-ink text-canvas dark:bg-hi-yellow dark:text-deep-ink'
                  : 'text-slate hover:bg-onyx/5 dark:hover:bg-onyx/20'
              }`}
            >
              <ShieldCheck className="h-3.5 w-3.5" />
              <span>Overview & Permissions</span>
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('tools')}
              className={`flex items-center gap-1.5 rounded-pill px-3 py-1.5 text-caption font-semibold transition-colors ${
                activeTab === 'tools'
                  ? 'bg-deep-ink text-canvas dark:bg-hi-yellow dark:text-deep-ink'
                  : 'text-slate hover:bg-onyx/5 dark:hover:bg-onyx/20'
              }`}
            >
              <Wrench className="h-3.5 w-3.5" />
              <span>Exported Tools ({tools.length})</span>
            </button>
            <button
              type="button"
              onClick={() => setActiveTab('raw')}
              className={`flex items-center gap-1.5 rounded-pill px-3 py-1.5 text-caption font-semibold transition-colors ${
                activeTab === 'raw'
                  ? 'bg-deep-ink text-canvas dark:bg-hi-yellow dark:text-deep-ink'
                  : 'text-slate hover:bg-onyx/5 dark:hover:bg-onyx/20'
              }`}
            >
              <Code2 className="h-3.5 w-3.5" />
              <span>Raw Manifest</span>
            </button>
          </div>
        </div>

        {/* Tab 1: Overview & Permissions */}
        {activeTab === 'overview' && (
          <div className="space-y-4">
            {manifest.description && (
              <p className="text-body leading-relaxed text-slate dark:text-cream/80">
                {manifest.description}
              </p>
            )}

            {/* Capabilities */}
            <div>
              <span className="mb-2 block text-caption font-semibold uppercase tracking-wider text-slate">
                Capabilities
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
                    className="flex items-center gap-1 px-2.5 py-1 text-caption"
                  >
                    <Layers className="h-3 w-3" />
                    <span>{t(`capabilities.${cap}`, cap)}</span>
                  </Badge>
                ))}
              </div>
            </div>

            {/* Sandboxed Permissions Checklist */}
            <div>
              <span className="mb-2 block text-caption font-semibold uppercase tracking-wider text-slate">
                {t('permissions.title', 'Sandboxed Permissions')}
              </span>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                {/* Outbound Network Domains */}
                <Card className="border border-onyx/10 bg-card-surface/40 p-3.5 dark:border-onyx/20 dark:bg-onyx/20">
                  <div className="flex items-center gap-2 font-semibold text-deep-ink dark:text-cream">
                    <Globe className="h-4 w-4 text-blue-500" />
                    <span>{t('permissions.netOutbound', 'Outbound Domains')}</span>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {(permissions.net_outbound || []).length > 0 ? (
                      permissions.net_outbound?.map((dom) => (
                        <span
                          key={dom}
                          className="rounded bg-blue-50 px-2 py-0.5 font-mono text-caption text-blue-700 dark:bg-blue-950/40 dark:text-blue-300"
                        >
                          {dom}
                        </span>
                      ))
                    ) : (
                      <span className="text-caption text-slate italic">Zero network access (Isolated)</span>
                    )}
                  </div>
                </Card>

                {/* Vault Secrets */}
                <Card className="border border-onyx/10 bg-card-surface/40 p-3.5 dark:border-onyx/20 dark:bg-onyx/20">
                  <div className="flex items-center gap-2 font-semibold text-deep-ink dark:text-cream">
                    <Key className="h-4 w-4 text-amber-500" />
                    <span>{t('permissions.secrets', 'Vault Secrets')}</span>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {(permissions.secrets || []).length > 0 ? (
                      permissions.secrets?.map((sec) => (
                        <span
                          key={sec}
                          className="rounded bg-amber-50 px-2 py-0.5 font-mono text-caption text-amber-700 dark:bg-amber-950/40 dark:text-amber-300"
                        >
                          {sec}
                        </span>
                      ))
                    ) : (
                      <span className="text-caption text-slate italic">No vault credentials requested</span>
                    )}
                  </div>
                </Card>

                {/* Persistent Storage */}
                <Card className="border border-onyx/10 bg-card-surface/40 p-3.5 dark:border-onyx/20 dark:bg-onyx/20">
                  <div className="flex items-center gap-2 font-semibold text-deep-ink dark:text-cream">
                    <Database className="h-4 w-4 text-emerald-500" />
                    <span>{t('permissions.storage', 'Persistent Storage')}</span>
                  </div>
                  <div className="mt-2">
                    {permissions.storage ? (
                      <span className="inline-flex items-center gap-1 text-caption font-semibold text-success">
                        <Check className="h-3.5 w-3.5" /> Scoped KV Storage Enabled
                      </span>
                    ) : (
                      <span className="text-caption text-slate italic">Stateless (Storage disabled)</span>
                    )}
                  </div>
                </Card>

                {/* Event Bus Topics */}
                <Card className="border border-onyx/10 bg-card-surface/40 p-3.5 dark:border-onyx/20 dark:bg-onyx/20">
                  <div className="flex items-center gap-2 font-semibold text-deep-ink dark:text-cream">
                    <Radio className="h-4 w-4 text-purple-500" />
                    <span>{t('permissions.busEvents', 'Event Bus Topics')}</span>
                  </div>
                  <div className="mt-2 flex flex-wrap gap-1.5">
                    {(permissions.bus_events || []).length > 0 ? (
                      permissions.bus_events?.map((top) => (
                        <span
                          key={top}
                          className="rounded bg-purple-50 px-2 py-0.5 font-mono text-caption text-purple-700 dark:bg-purple-950/40 dark:text-purple-300"
                        >
                          {top}
                        </span>
                      ))
                    ) : (
                      <span className="text-caption text-slate italic">No event topics registered</span>
                    )}
                  </div>
                </Card>
              </div>
            </div>
          </div>
        )}

        {/* Tab 2: Exported Tools */}
        {activeTab === 'tools' && (
          <div className="space-y-3">
            {tools.length > 0 ? (
              tools.map((tool) => (
                <Card key={tool.name} className="border border-onyx/10 p-4 dark:border-onyx/20 dark:bg-onyx/20">
                  <div className="flex items-center justify-between">
                    <span className="font-mono text-body font-bold text-deep-ink dark:text-cream">
                      {tool.name}
                    </span>
                    <Badge variant="neutral">WASM Tool</Badge>
                  </div>
                  <p className="mt-1 text-caption text-slate">{tool.description}</p>
                  {tool.parameters && Object.keys(tool.parameters).length > 0 && (
                    <div className="mt-3 rounded bg-canvas p-2.5 font-mono text-caption dark:bg-onyx/60">
                      <pre className="overflow-x-auto text-deep-ink dark:text-cream/90">
                        {JSON.stringify(tool.parameters, null, 2)}
                      </pre>
                    </div>
                  )}
                </Card>
              ))
            ) : (
              <div className="rounded-card border border-dashed border-onyx/15 p-6 text-center text-slate">
                {t('modals.noTools', 'No agent tools exported directly by this plugin.')}
              </div>
            )}
          </div>
        )}

        {/* Tab 3: Raw Manifest */}
        {activeTab === 'raw' && (
          <div>
            <div className="mb-2 flex items-center justify-end">
              <Button type="button" variant="secondary" size="sm" onClick={handleCopyManifest}>
                {copied ? <Check className="mr-1 h-3.5 w-3.5 text-success" /> : <Copy className="mr-1 h-3.5 w-3.5" />}
                <span>{copied ? 'Copied!' : 'Copy JSON'}</span>
              </Button>
            </div>
            <div className="max-h-96 overflow-y-auto rounded-card border border-onyx/15 bg-canvas p-4 font-mono text-caption text-deep-ink dark:border-onyx/30 dark:bg-onyx/60 dark:text-cream">
              <pre>{JSON.stringify(manifest, null, 2)}</pre>
            </div>
          </div>
        )}

        {/* Footer */}
        <div className="flex justify-end pt-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </Modal>
  );
}

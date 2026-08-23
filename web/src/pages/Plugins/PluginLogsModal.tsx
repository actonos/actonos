import { useState, useEffect } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { api } from '@/lib/api';
import type { PluginInfo } from '@/lib/types';
import { Terminal, RefreshCw } from 'lucide-react';

export interface PluginLogsModalProps {
  plugin: PluginInfo | null;
  isOpen: boolean;
  onClose: () => void;
}

export function PluginLogsModal({ plugin, isOpen, onClose }: PluginLogsModalProps) {
  const { t } = useTranslation('plugins');
  const [logs, setLogs] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);

  const fetchLogs = async () => {
    if (!plugin) return;
    try {
      setLoading(true);
      const res = await api.getPluginLogs(plugin.manifest.id);
      setLogs(res.logs || []);
    } catch {
      setLogs([]);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    if (isOpen && plugin) {
      fetchLogs();
    }
  }, [isOpen, plugin]);

  if (!plugin) return null;

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('modals.logsTitle', { id: plugin.manifest.id, defaultValue: `Plugin Runtime Logs: ${plugin.manifest.id}` })}
      maxWidth="max-w-3xl"
    >
      <div className="space-y-4">
        <div className="-mt-3 mb-1 font-mono text-caption text-slate">
          Sandbox Status: {plugin.status.toUpperCase()}
        </div>

        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2 text-caption text-slate">
            <Terminal className="h-4 w-4" />
            <span>Wazero Execution Trace Stream</span>
          </div>
          <Button
            type="button"
            variant="secondary"
            size="sm"
            onClick={fetchLogs}
            disabled={loading}
          >
            <RefreshCw className={`mr-1.5 h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
            <span>{t('modals.refreshLogs', 'Refresh Logs')}</span>
          </Button>
        </div>

        {/* Terminal log output */}
        <div className="h-80 overflow-y-auto rounded-card bg-[#130e30] p-4 font-mono text-caption text-cream shadow-inner dark:bg-black/80">
          {logs.length > 0 ? (
            <div className="space-y-1">
              {logs.map((line, idx) => (
                <div key={idx} className="leading-relaxed hover:bg-white/5">
                  <span className="text-slate select-none">[{idx + 1}] </span>
                  <span>{line}</span>
                </div>
              ))}
            </div>
          ) : (
            <div className="flex h-full flex-col items-center justify-center text-center text-slate">
              <Terminal className="h-8 w-8 mb-2 opacity-40" />
              <p>{t('modals.noLogs', 'No runtime log events recorded yet.')}</p>
              <p className="text-[11px] mt-1 text-slate/70">Syscalls and guest errors will appear here automatically.</p>
            </div>
          )}
        </div>

        <div className="flex justify-end pt-2">
          <Button type="button" variant="secondary" onClick={onClose}>
            Close
          </Button>
        </div>
      </div>
    </Modal>
  );
}

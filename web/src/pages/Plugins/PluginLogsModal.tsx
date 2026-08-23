import { useState, useEffect, useRef } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Button } from '@/components/ui/Button';
import { Badge } from '@/components/ui/Badge';
import { api } from '@/lib/api';
import type { PluginInfo } from '@/lib/types';
import { Terminal, RefreshCw, Copy, Check } from 'lucide-react';

export interface PluginLogsModalProps {
  plugin: PluginInfo | null;
  isOpen: boolean;
  onClose: () => void;
}

export function PluginLogsModal({ plugin, isOpen, onClose }: PluginLogsModalProps) {
  const { t } = useTranslation('plugins');
  const [logs, setLogs] = useState<string[]>([]);
  const [loading, setLoading] = useState(false);
  const [copied, setCopied] = useState(false);
  const logContainerRef = useRef<HTMLDivElement>(null);

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

  useEffect(() => {
    if (logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight;
    }
  }, [logs]);

  const handleCopyLogs = async () => {
    if (logs.length === 0) return;
    try {
      await navigator.clipboard.writeText(logs.join('\n'));
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // ignore
    }
  };

  if (!plugin) return null;

  const renderLogLine = (line: string) => {
    const isError = line.includes('[ERROR]') || line.includes('level=ERROR') || line.includes('level=error') || line.includes('err=');
    const isWarn = line.includes('[WARN]') || line.includes('level=WARN') || line.includes('level=warn');
    const isInfo = line.includes('[INFO]') || line.includes('level=INFO') || line.includes('level=info');
    const isDebug = line.includes('[DEBUG]') || line.includes('level=DEBUG') || line.includes('level=debug');

    let colorClass = 'text-white/90';
    if (isError) colorClass = 'text-rose-400 font-medium';
    else if (isWarn) colorClass = 'text-amber-300 font-medium';
    else if (isInfo) colorClass = 'text-cyan-300';
    else if (isDebug) colorClass = 'text-white/60';

    return <span className={colorClass}>{line}</span>;
  };

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('modals.logsTitle', { id: plugin.manifest.id, defaultValue: `Plugin Runtime Logs: ${plugin.manifest.id}` })}
      maxWidth="max-w-3xl"
    >
      <div className="space-y-4">
        <div className="flex flex-wrap items-center justify-between gap-2 border-b border-onyx/5 dark:border-white/5 pb-3">
          <div className="flex items-center gap-2">
            <Badge variant={plugin.enabled ? 'active' : 'neutral'} className="font-mono uppercase text-[11px]">
              {plugin.status}
            </Badge>
            <span className="font-mono text-caption text-slate">
              {logs.length} {logs.length === 1 ? 'event' : 'events'} recorded
            </span>
          </div>

          <div className="flex items-center gap-2">
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={handleCopyLogs}
              disabled={logs.length === 0}
              icon={copied ? <Check className="mr-1.5 h-3.5 w-3.5 text-emerald-500" /> : <Copy className="mr-1.5 h-3.5 w-3.5" />}
            >
              {copied ? 'Copied' : 'Copy Logs'}
            </Button>
            <Button
              type="button"
              variant="secondary"
              size="sm"
              onClick={fetchLogs}
              disabled={loading}
              icon={<RefreshCw className={`mr-1.5 h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />}
            >
              {t('modals.refreshLogs', 'Refresh Logs')}
            </Button>
          </div>
        </div>

        {/* Terminal log output */}
        <div
          ref={logContainerRef}
          className="h-88 overflow-y-auto rounded-card bg-[#0e0a24] p-4 font-mono text-[12px] text-white/90 shadow-2xl border border-onyx/10 dark:border-white/10"
        >
          {logs.length > 0 ? (
            <div className="space-y-1">
              {logs.map((line, idx) => (
                <div key={idx} className="flex items-start gap-2 leading-relaxed hover:bg-white/5 px-1 py-0.5 rounded transition-colors">
                  <span className="text-white/30 select-none text-right shrink-0 min-w-[28px]">
                    {idx + 1}
                  </span>
                  <div className="flex-1 break-all whitespace-pre-wrap">
                    {renderLogLine(line)}
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <div className="flex h-full flex-col items-center justify-center text-center text-white/50">
              <Terminal className="h-8 w-8 mb-2 opacity-40 text-cyan-400" />
              <p className="text-white/80 font-medium">{t('modals.noLogs', 'No runtime log events recorded yet.')}</p>
              <p className="text-[11px] mt-1 text-white/40">Syscalls and guest runtime traces will appear here automatically.</p>
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

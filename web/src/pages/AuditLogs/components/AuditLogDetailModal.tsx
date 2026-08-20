import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Modal } from '@/components/ui/Modal';
import { Badge } from '@/components/ui/Badge';
import { Button } from '@/components/ui/Button';
import type { AuditLogItem } from '@/lib/api';
import {
  ShieldAlert,
  ShieldCheck,
  Clock,
  Cpu,
  Wrench,
  Copy,
  Check,
  Hash,
  FileCode,
  Calendar,
} from 'lucide-react';

export interface AuditLogDetailModalProps {
  log: AuditLogItem | null;
  isOpen: boolean;
  onClose: () => void;
}

export function AuditLogDetailModal({ log, isOpen, onClose }: AuditLogDetailModalProps) {
  const { t } = useTranslation('audit');
  const [copiedTrace, setCopiedTrace] = useState(false);
  const [copiedHash, setCopiedHash] = useState(false);
  const [copiedJson, setCopiedJson] = useState(false);

  if (!log) return null;

  const handleCopyTrace = () => {
    navigator.clipboard.writeText(log.trace_id);
    setCopiedTrace(true);
    setTimeout(() => setCopiedTrace(false), 2000);
  };

  const handleCopyHash = () => {
    if (log.entry_hash) {
      navigator.clipboard.writeText(log.entry_hash);
      setCopiedHash(true);
      setTimeout(() => setCopiedHash(false), 2000);
    }
  };

  const handleCopyJson = () => {
    navigator.clipboard.writeText(JSON.stringify(log, null, 2));
    setCopiedJson(true);
    setTimeout(() => setCopiedJson(false), 2000);
  };

  const localTime = new Date(log.timestamp).toLocaleString();
  const utcTime = new Date(log.timestamp).toUTCString();

  return (
    <Modal
      isOpen={isOpen}
      onClose={onClose}
      title={t('detail.title', 'Audit Log Record Details')}
      maxWidth="max-w-2xl"
    >
      <div className="space-y-5">
        {/* Header Summary */}
        <div className="flex flex-wrap items-center justify-between gap-3 p-3.5 bg-soft-meadow rounded-[18px] border border-onyx/10">
          <div className="flex items-center gap-2.5">
            <div className="p-2 rounded-xl bg-canvas border border-onyx/10">
              <Wrench className="w-5 h-5 text-deep-ink" />
            </div>
            <div>
              <div className="font-mono font-bold text-deep-ink text-body-sm">{log.tool_name}</div>
              <div className="font-sans text-caption text-slate flex items-center gap-1.5 mt-0.5">
                <Cpu className="w-3.5 h-3.5" />
                <span>{log.agent_id}</span>
              </div>
            </div>
          </div>

          <div className="flex items-center gap-2">
            <Badge
              variant={
                log.risk_level === 'High'
                  ? 'accent'
                  : log.risk_level === 'Medium'
                  ? 'stopped'
                  : 'neutral'
              }
            >
              {log.risk_level} Risk
            </Badge>
            <Badge
              variant={
                log.status === 'Success'
                  ? 'neutral'
                  : log.status === 'Blocked'
                  ? 'stopped'
                  : 'accent'
              }
            >
              {log.status}
            </Badge>
          </div>
        </div>

        {/* Error / Block Callout */}
        {log.error && (
          <div className="p-3.5 rounded-[18px] bg-accent-coral/10 border border-accent-coral/30 space-y-1">
            <div className="flex items-center gap-1.5 text-accent-coral font-bold text-caption">
              <ShieldAlert className="w-4 h-4" />
              <span>{t('detail.errorCallout', 'Execution Error / Policy Block')}</span>
            </div>
            <p className="font-mono text-body-sm text-deep-ink whitespace-pre-wrap break-words">
              {log.error}
            </p>
          </div>
        )}

        {/* Trace ID & Timing Grid */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
          {/* Trace ID */}
          <div className="p-3 bg-canvas rounded-[18px] border border-onyx/10 space-y-1">
            <span className="text-caption font-medium text-slate">{t('detail.traceId', 'Trace ID (W3C)')}</span>
            <div className="flex items-center justify-between gap-2">
              <span className="font-mono text-caption text-deep-ink truncate" title={log.trace_id}>
                {log.trace_id}
              </span>
              <button
                type="button"
                onClick={handleCopyTrace}
                className="p-1 rounded hover:bg-soft-meadow text-slate hover:text-deep-ink transition-colors cursor-pointer"
                title={t('detail.copyTrace', 'Copy Trace ID')}
              >
                {copiedTrace ? <Check className="w-3.5 h-3.5 text-green-600" /> : <Copy className="w-3.5 h-3.5" />}
              </button>
            </div>
          </div>

          {/* Latency */}
          <div className="p-3 bg-canvas rounded-[18px] border border-onyx/10 space-y-1">
            <span className="text-caption font-medium text-slate flex items-center gap-1">
              <Clock className="w-3.5 h-3.5" />
              <span>{t('detail.duration', 'Execution Latency')}</span>
            </span>
            <div className="font-mono font-bold text-body text-deep-ink">
              {log.execution_time_ms} ms
            </div>
          </div>
        </div>

        {/* Timestamps */}
        <div className="p-3.5 bg-canvas rounded-[18px] border border-onyx/10 space-y-2">
          <div className="flex items-center gap-1.5 text-caption font-medium text-slate">
            <Calendar className="w-4 h-4" />
            <span>{t('detail.timestamp', 'Timestamp')}</span>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-2 text-caption">
            <div>
              <span className="text-slate block">Local Time:</span>
              <span className="font-mono text-deep-ink font-medium">{localTime}</span>
            </div>
            <div>
              <span className="text-slate block">UTC Time:</span>
              <span className="font-mono text-deep-ink font-medium">{utcTime}</span>
            </div>
          </div>
        </div>

        {/* Cryptographic Hash Chain (Tamper-Evidence) */}
        {(log.entry_hash || log.previous_hash) && (
          <div className="p-3.5 bg-soft-meadow rounded-[18px] border border-onyx/10 space-y-2.5">
            <div className="flex items-center justify-between">
              <div className="flex items-center gap-1.5 text-caption font-bold text-deep-ink">
                <Hash className="w-4 h-4 text-deep-ink" />
                <span>{t('detail.cryptoSection', 'Cryptographic Hash Chain')}</span>
              </div>
              <span className="inline-flex items-center gap-1 text-[11px] font-mono text-emerald-700 bg-emerald-100 px-2 py-0.5 rounded-full font-medium">
                <ShieldCheck className="w-3 h-3" />
                SHA-256 Sealed
              </span>
            </div>

            {log.entry_hash && (
              <div className="space-y-1">
                <span className="text-[11px] font-mono text-slate block">{t('detail.entryHash', 'Entry SHA-256 Hash')}:</span>
                <div className="flex items-center justify-between gap-2 p-2 bg-canvas rounded-xl border border-onyx/10">
                  <span className="font-mono text-[11px] text-deep-ink break-all select-all">
                    {log.entry_hash}
                  </span>
                  <button
                    type="button"
                    onClick={handleCopyHash}
                    className="p-1 rounded hover:bg-soft-meadow text-slate hover:text-deep-ink transition-colors cursor-pointer shrink-0"
                    title="Copy Entry Hash"
                  >
                    {copiedHash ? <Check className="w-3.5 h-3.5 text-green-600" /> : <Copy className="w-3.5 h-3.5" />}
                  </button>
                </div>
              </div>
            )}

            {log.previous_hash && (
              <div className="space-y-1">
                <span className="text-[11px] font-mono text-slate block">{t('detail.prevHash', 'Previous Block Hash')}:</span>
                <div className="p-2 bg-canvas/60 rounded-xl border border-onyx/10">
                  <span className="font-mono text-[11px] text-slate break-all select-all">
                    {log.previous_hash}
                  </span>
                </div>
              </div>
            )}
          </div>
        )}

        {/* Raw JSON Payload */}
        <div className="space-y-1.5">
          <div className="flex items-center justify-between">
            <span className="text-caption font-medium text-slate flex items-center gap-1.5">
              <FileCode className="w-4 h-4" />
              <span>{t('detail.rawJson', 'Raw JSON Payload')}</span>
            </span>
            <button
              type="button"
              onClick={handleCopyJson}
              className="text-caption font-medium text-deep-ink hover:text-slate flex items-center gap-1 cursor-pointer transition-colors"
            >
              {copiedJson ? <Check className="w-3.5 h-3.5 text-green-600" /> : <Copy className="w-3.5 h-3.5" />}
              <span>{copiedJson ? t('detail.copied', 'Copied!') : t('detail.copyJson', 'Copy JSON')}</span>
            </button>
          </div>
          <pre className="p-3.5 bg-deep-ink text-white rounded-[18px] text-[11px] font-mono overflow-auto max-h-48">
            {JSON.stringify(log, null, 2)}
          </pre>
        </div>

        {/* Modal Footer */}
        <div className="flex justify-end pt-2 border-t border-onyx/10">
          <Button variant="ghost" onClick={onClose}>
            {t('detail.close', 'Close')}
          </Button>
        </div>
      </div>
    </Modal>
  );
}

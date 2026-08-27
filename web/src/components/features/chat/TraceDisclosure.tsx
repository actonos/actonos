import { ChevronDown, ChevronRight, FileCode, ShieldCheck, Terminal } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import type { ChatMessage } from '@/pages/Chat/chatTypes';

export function TraceDisclosure({
  message,
  expanded,
  activeTab,
  onToggle,
  onTabChange,
}: {
  message: ChatMessage;
  expanded: boolean;
  activeTab: 'traces' | 'audit';
  onToggle: () => void;
  onTabChange: (tab: 'traces' | 'audit') => void;
}) {
  const { t } = useTranslation('chat');
  const hasTraces = Boolean(message.toolCalls?.length || message.thought);
  const hasAudits = Boolean(message.auditLogs?.length);
  if (!hasTraces && !hasAudits) return null;
  return (
    <div className="mt-3 border-t border-onyx/10 pt-2.5 text-caption">
      <div className="flex items-center justify-between gap-2">
        <button type="button" onClick={onToggle} aria-expanded={expanded} className="flex min-w-0 items-center gap-1.5 font-mono text-[11px] font-semibold text-slate hover:text-deep-ink">
          <Terminal className="h-3.5 w-3.5 shrink-0 text-deep-ink" aria-hidden="true" />
          <span className="truncate">{t('executionDetails', { tools: message.toolCalls?.length || 0, audits: message.auditLogs?.length || 0 })}</span>
          {expanded ? <ChevronDown className="h-3.5 w-3.5" /> : <ChevronRight className="h-3.5 w-3.5" />}
        </button>
        {expanded && hasAudits && (
          <div className="flex items-center gap-1 rounded-lg border border-onyx/5 bg-canvas p-0.5" role="tablist">
            {(['traces', 'audit'] as const).map((tab) => (
              <button
                key={tab}
                type="button"
                role="tab"
                aria-selected={activeTab === tab}
                onClick={() => onTabChange(tab)}
                className={`rounded px-2 py-0.5 text-[10px] font-semibold ${activeTab === tab ? 'bg-deep-ink text-white' : 'text-slate hover:text-deep-ink'}`}
              >
                {tab === 'traces' ? t('traces') : t('auditLogs')}
              </button>
            ))}
          </div>
        )}
      </div>
      {expanded && (
        <div className="mt-2 space-y-2 rounded-[14px] border border-onyx/10 bg-canvas p-3 font-mono text-caption text-slate">
          {activeTab === 'traces' ? (
            message.toolCalls?.length ? message.toolCalls.map((call, index) => (
              <div key={`${call.tool}-${index}`} className="space-y-1 rounded-xl border border-onyx/5 bg-soft-meadow p-2 text-[11px]">
                <div className="flex items-center justify-between font-semibold text-deep-ink">
                  <span className="flex items-center gap-1.5"><FileCode className="h-3.5 w-3.5 text-hi-yellow" />{call.tool}</span>
                  <span className="flex items-center gap-2">
                    {call.status === 'awaiting_approval' && <span>{t('toolAwaitingApproval')}</span>}
                    {call.status === 'rejected' && <span>{t('toolRejected')}</span>}
                    {call.latency_ms !== undefined && <span>{t('milliseconds', { value: call.latency_ms })}</span>}
                  </span>
                </div>
                {call.args != null && <div className="break-words text-[10px]">{JSON.stringify(call.args)}</div>}
                {call.result && <div className="max-h-24 overflow-y-auto whitespace-pre-wrap rounded bg-canvas p-1.5 text-[10px] text-deep-ink">{call.result}</div>}
              </div>
            )) : <p className="text-[11px] italic">{t('noToolCalls')}</p>
          ) : (
            <div className="max-h-40 space-y-1.5 overflow-y-auto">
              {message.auditLogs?.map((log, index) => (
                <div key={`${log.timestamp}-${index}`} className="rounded-xl border border-onyx/5 bg-soft-meadow p-2 text-[10px]">
                  <div className="flex items-center justify-between font-semibold text-deep-ink">
                    <span className="flex items-center gap-1"><ShieldCheck className="h-3 w-3 text-success" />{log.action}</span>
                    <span>{log.status}</span>
                  </div>
                  <div className="mt-1 flex justify-between gap-2"><span>{log.verification}</span><span>{t('milliseconds', { value: log.duration_ms })}</span></div>
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

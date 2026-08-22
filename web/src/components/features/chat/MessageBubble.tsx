import { useState, useEffect, useRef } from 'react';
import {
  Activity,
  Bot,
  Check,
  Clock,
  Copy,
  Cpu,
  Sparkles,
  User,
  Folder,
  FileCode,
  FileText,
  FileSpreadsheet,
  ExternalLink,
  X,
} from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { MarkdownContent } from '@/components/chat/MarkdownContent';
import type { ChatMessage } from '@/pages/Chat/chatTypes';
import { TraceDisclosure } from './TraceDisclosure';

function cleanReasoning(text: string): string {
  return text
    .replace(/<[|｜]{1,2}DSML[|｜]{1,2}[\s\S]*?<\/[|｜]{1,2}DSML[|｜]{1,2}tool_calls>/g, '')
    .replace(/<[|｜]{1,2}[\s\S]*?>/g, '')
    .replace(/<\/?(?:tool_call|function_call|invoke|parameter)[^>]*>/g, '')
    .trim();
}

function StreamingReasoningBlock({
  text,
  title,
  defaultOpen = true,
}: {
  text: string;
  title: string;
  defaultOpen?: boolean;
}) {
  const containerRef = useRef<HTMLDivElement>(null);
  const cleaned = cleanReasoning(text);

  useEffect(() => {
    if (containerRef.current) {
      containerRef.current.scrollTop = containerRef.current.scrollHeight;
    }
  }, [text]);

  if (!cleaned) return null;

  return (
    <details
      className="group mb-2.5 rounded-xl border border-onyx/10 bg-canvas/60 p-2.5 font-mono text-caption text-slate"
      open={defaultOpen}
    >
      <summary className="flex cursor-pointer select-none items-center gap-2 font-semibold text-deep-ink">
        <Sparkles className="h-3.5 w-3.5 text-hi-yellow" />
        <span>{title}</span>
      </summary>
      <div
        ref={containerRef}
        className="mt-2 max-h-48 overflow-y-auto whitespace-pre-wrap border-t border-onyx/5 pt-2 text-[11px] leading-relaxed text-deep-ink/80 scroll-smooth"
      >
        {cleaned}
      </div>
    </details>
  );
}

export function MessageBubble({
  message,
  copied,
  traceExpanded,
  traceTab,
  onCopy,
  onToggleTrace,
  onTraceTabChange,
}: {
  message: ChatMessage;
  copied: boolean;
  traceExpanded: boolean;
  traceTab: 'traces' | 'audit';
  onCopy: () => void;
  onToggleTrace: () => void;
  onTraceTabChange: (tab: 'traces' | 'audit') => void;
}) {
  const { t } = useTranslation('chat');
  const [activeImagePreview, setActiveImagePreview] = useState<{ url: string; name: string } | null>(null);
  const isUser = message.role === 'user';
  const hasSegments = message.segments && message.segments.length > 0;
  const reasoningSteps = hasSegments
    ? message.segments!.filter((s) => s.type === 'reasoning' && cleanReasoning(s.text).length > 0)
    : [];

  return (
    <>
      <article className={`flex flex-col ${isUser ? 'items-end' : 'items-start'}`}>
        <div
          className={`group relative max-w-[90%] rounded-[20px] p-4 transition-all sm:max-w-[80%] ${
            isUser
              ? 'rounded-br-none bg-deep-ink text-white'
              : 'rounded-bl-none border border-onyx/10 bg-soft-meadow text-deep-ink'
          }`}
        >
          <div className="mb-2 flex items-center gap-1.5 text-[11px] font-semibold opacity-75">
            {isUser ? <User className="h-3 w-3" /> : <Bot className="h-3 w-3" />}
            <span>{isUser ? t('you') : t('assistant')}</span>
          </div>

          {/* Transient operational thought status (e.g. Deliberating with agent...) */}
          {message.thought && (
            <div className="mb-3 flex items-center gap-2 rounded-xl border border-onyx/10 bg-canvas/90 p-2.5 font-mono text-caption text-deep-ink shadow-xs">
              <Activity className="h-4 w-4 shrink-0 animate-spin text-hi-yellow" aria-hidden="true" />
              <span className="line-clamp-2 leading-tight">{message.thought}</span>
            </div>
          )}

        {isUser ? (
          <div className="font-sans text-body-sm leading-relaxed space-y-2.5">
            {message.attachments && message.attachments.length > 0 && (
              <div className="flex flex-wrap gap-2 pb-2 mb-1 border-b border-white/15">
                {message.attachments.map((att, idx) => {
                  if (att.isImage && att.previewUrl) {
                    return (
                      <div
                        key={`msg-att-${idx}`}
                        onClick={() => setActiveImagePreview({ url: att.previewUrl!, name: att.name })}
                        className="group relative cursor-pointer flex items-center gap-2 rounded-xl border border-white/25 bg-white/10 p-1.5 pr-3 hover:bg-white/20 transition-all text-xs text-white shadow-xs"
                        title={t('attachments.viewImage', 'Click to view image')}
                      >
                        <img
                          src={att.thumbnailUrl || att.previewUrl}
                          alt={att.name}
                          className="w-10 h-10 rounded-lg object-cover border border-white/20 shrink-0 bg-black/20"
                        />
                        <div className="flex flex-col min-w-0">
                          <span className="font-mono text-[11px] font-medium truncate max-w-[140px]">
                            {att.name}
                          </span>
                          <span className="text-[10px] text-white/70">
                            {att.width && att.height ? `${att.width}x${att.height} • ` : ''}
                            {att.size < 1024 ? `${att.size} B` : `${(att.size / 1024).toFixed(1)} KB`}
                          </span>
                        </div>
                      </div>
                    );
                  }

                  const ext = att.name.split('.').pop()?.toLowerCase() || '';
                  const isSpreadsheet = ['xlsx', 'xls', 'ods', 'csv', 'tsv'].includes(ext);
                  const isCode = [
                    'ts', 'tsx', 'js', 'jsx', 'go', 'py', 'rs', 'c', 'cpp', 'h', 'java', 'kt', 'cs',
                    'json', 'yaml', 'yml', 'toml', 'xml', 'html', 'css', 'sql', 'sh', 'env'
                  ].includes(ext);

                  return (
                    <div
                      key={`msg-att-${idx}`}
                      className="flex items-center gap-1.5 rounded-full border border-white/20 bg-white/10 px-2.5 py-1 text-xs text-white"
                    >
                      {att.isWorkspace ? (
                        <Folder className="w-3.5 h-3.5 text-hi-yellow shrink-0" />
                      ) : isSpreadsheet ? (
                        <FileSpreadsheet className="w-3.5 h-3.5 text-emerald-300 shrink-0" />
                      ) : isCode ? (
                        <FileCode className="w-3.5 h-3.5 text-sky-300 shrink-0" />
                      ) : (
                        <FileText className="w-3.5 h-3.5 text-white/80 shrink-0" />
                      )}
                      <span className="font-mono text-[11px] truncate max-w-[150px]">{att.name}</span>
                      {att.isWorkspace && (
                        <span className="px-1 py-0.2 rounded-xs bg-hi-yellow/30 text-white text-[8px] font-semibold uppercase tracking-wider">
                          Workspace
                        </span>
                      )}
                      <span className="text-[9px] text-white/60">
                        ({att.size < 1024 ? `${att.size} B` : `${(att.size / 1024).toFixed(1)} KB`})
                      </span>
                    </div>
                  );
                })}
              </div>
            )}
            <MarkdownContent content={message.displayContent || message.content} isUser={true} />
          </div>
        ) : hasSegments ? (
          message.finalized ? (
            /* Mode 1: Finalized — intermediate thinking auto-collapsed, clean final answer */
            <>
              {reasoningSteps.length > 0 && (
                <details className="group mb-3 rounded-xl border border-onyx/10 bg-canvas/60 p-2.5 font-mono text-caption text-slate">
                  <summary className="flex cursor-pointer select-none items-center gap-2 font-semibold text-deep-ink">
                    <Sparkles className="h-3.5 w-3.5 text-hi-yellow" />
                    <span>{t('showThinking', { count: reasoningSteps.length, defaultValue: `Show thinking process (${reasoningSteps.length} steps)` })}</span>
                  </summary>
                  <div className="mt-2.5 space-y-2.5 border-t border-onyx/5 pt-2 max-h-60 overflow-y-auto">
                    {reasoningSteps.map((seg, i) => (
                      <div
                        key={`final-seg-${i}`}
                        className="rounded-lg bg-canvas/80 p-2 text-[11px] text-deep-ink/80 whitespace-pre-wrap leading-relaxed border border-onyx/5"
                      >
                        {reasoningSteps.length > 1 && (
                          <span className="font-semibold text-[10px] text-slate uppercase tracking-wider block mb-0.5">
                            {t('thinkingStep', { step: i + 1, defaultValue: `Step ${i + 1}` })}
                          </span>
                        )}
                        {cleanReasoning(seg.text)}
                      </div>
                    ))}
                  </div>
                </details>
              )}
              <div className="font-sans text-body-sm leading-relaxed">
                {message.content ? (
                  <MarkdownContent content={message.content} isUser={false} />
                ) : message.toolCalls && message.toolCalls.length > 0 ? (
                  <span className="text-slate italic font-mono text-[11px]">
                    {t('operationsCompleted', 'Completed operations successfully.')}
                  </span>
                ) : (
                  <div className="flex items-center gap-2 text-caption font-mono text-slate animate-pulse py-1">
                    <Sparkles className="h-3.5 w-3.5 text-hi-yellow animate-spin" />
                    <span>{t('connecting')}</span>
                  </div>
                )}
              </div>
            </>
          ) : (
            /* Mode 2: Live streaming — interleaved thinking blocks and content in real-time */
            <div className="space-y-2.5">
              {message.segments!.map((seg, i) =>
                seg.type === 'reasoning' ? (
                  <StreamingReasoningBlock
                    key={`stream-seg-${i}`}
                    text={seg.text}
                    title={t('thinkingProcess', 'Thinking Process')}
                    defaultOpen={true}
                  />
                ) : (
                  <div key={`stream-seg-${i}`} className="font-sans text-body-sm leading-relaxed">
                    <MarkdownContent content={seg.text} isUser={false} />
                  </div>
                )
              )}
            </div>
          )
        ) : (
          /* Mode 3: Legacy fallback for historical messages loaded from DB */
          <>
            {message.reasoning && (
              <StreamingReasoningBlock
                text={message.reasoning}
                title={t('thinkingProcess', 'Thinking Process')}
                defaultOpen={!message.content}
              />
            )}
            <div className="font-sans text-body-sm leading-relaxed">
              {message.content ? (
                <MarkdownContent content={message.content} isUser={false} />
              ) : message.toolCalls && message.toolCalls.length > 0 ? (
                <span className="text-slate italic font-mono text-[11px]">
                  {t('operationsCompleted', 'Completed operations successfully.')}
                </span>
              ) : (
                !message.thought && !message.reasoning && (
                  <div className="flex items-center gap-2 text-caption font-mono text-slate animate-pulse py-1">
                    <Sparkles className="h-3.5 w-3.5 text-hi-yellow animate-spin" />
                    <span>{t('connecting')}</span>
                  </div>
                )
              )}
            </div>
          </>
        )}
        <TraceDisclosure
          message={message}
          expanded={traceExpanded}
          activeTab={traceTab}
          onToggle={onToggleTrace}
          onTabChange={onTraceTabChange}
        />
        <footer className="mt-2 flex items-center justify-between gap-3 pt-1 text-[11px] opacity-70">
          <div className="flex min-w-0 items-center gap-2">
            <span className="flex items-center gap-1"><Clock className="h-3 w-3" />{message.timestamp}</span>
            {message.model && <span className="flex min-w-0 items-center gap-1 font-mono text-[10px]"><Cpu className="h-3 w-3" /><span className="truncate">{message.model}</span></span>}
          </div>
          <button type="button" onClick={onCopy} aria-label={t('copyResponse')} className="rounded-full p-1 hover:bg-onyx/5">
            {copied ? <Check className="h-3 w-3 text-success" /> : <Copy className="h-3 w-3" />}
          </button>
        </footer>
      </div>
    </article>

    {/* Image Preview Lightbox Modal */}
    {activeImagePreview && (
      <div
        role="dialog"
        aria-modal="true"
        className="fixed inset-0 z-50 flex items-center justify-center bg-black/80 backdrop-blur-xs p-4 animate-in fade-in duration-150"
        onClick={() => setActiveImagePreview(null)}
      >
        <div
          className="relative max-w-4xl max-h-[90vh] flex flex-col items-center gap-2"
          onClick={(e) => e.stopPropagation()}
        >
          <div className="flex items-center justify-between w-full px-2 text-white">
            <span className="font-mono text-xs truncate max-w-[300px]">{activeImagePreview.name}</span>
            <div className="flex items-center gap-2">
              <a
                href={activeImagePreview.url}
                target="_blank"
                rel="noreferrer"
                className="p-1 rounded-full hover:bg-white/20 text-white/80 transition-colors"
                title={t('attachments.openOriginal', 'Open original')}
              >
                <ExternalLink className="w-4 h-4" />
              </a>
              <button
                type="button"
                onClick={() => setActiveImagePreview(null)}
                className="p-1 rounded-full hover:bg-white/20 text-white transition-colors cursor-pointer"
              >
                <X className="w-4 h-4" />
              </button>
            </div>
          </div>
          <img
            src={activeImagePreview.url}
            alt={activeImagePreview.name}
            className="max-h-[80vh] max-w-full rounded-xl object-contain shadow-2xl border border-white/20"
          />
        </div>
      </div>
    )}
  </>
  );
}

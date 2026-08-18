import { Activity, Bot, Check, Clock, Copy, Cpu, User } from 'lucide-react';
import { useTranslation } from 'react-i18next';
import { MarkdownContent } from '@/components/chat/MarkdownContent';
import type { ChatMessage } from '@/pages/Chat/chatTypes';
import { TraceDisclosure } from './TraceDisclosure';

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
  const isUser = message.role === 'user';
  return (
    <article className={`flex flex-col ${isUser ? 'items-end' : 'items-start'}`}>
      <div className={`group relative max-w-[90%] rounded-[20px] p-4 transition-all sm:max-w-[80%] ${
        isUser ? 'rounded-br-none bg-deep-ink text-white' : 'rounded-bl-none border border-onyx/10 bg-soft-meadow text-deep-ink'
      }`}>
        <div className="mb-2 flex items-center gap-1.5 text-[11px] font-semibold opacity-75">
          {isUser ? <User className="h-3 w-3" /> : <Bot className="h-3 w-3" />}
          <span>{isUser ? t('you') : t('assistant')}</span>
        </div>
        {message.thought && (
          <div className="mb-3 flex items-center gap-2 rounded-xl border border-onyx/10 bg-canvas/80 p-2.5 font-mono text-caption text-deep-ink">
            <Activity className="h-4 w-4 shrink-0 animate-spin text-hi-yellow" aria-hidden="true" />
            <span className="line-clamp-2">{message.thought}</span>
          </div>
        )}
        <div className="font-sans text-body-sm leading-relaxed">
          {message.content ? <MarkdownContent content={message.content} isUser={isUser} /> : <span>{message.thought ? '' : t('connecting')}</span>}
        </div>
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
  );
}

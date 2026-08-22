import type { RefObject } from 'react';
import type { ChatMessage } from '@/pages/Chat/chatTypes';
import { ChatEmptyState } from './ChatEmptyState';
import { MessageBubble } from './MessageBubble';

export function MessageTimeline({
  messages,
  agentName,
  prompts,
  copiedIndex,
  expandedTraces,
  traceTabs,
  endRef,
  containerRef,
  onScroll,
  onPrompt,
  onCopy,
  onToggleTrace,
  onTraceTabChange,
}: {
  messages: ChatMessage[];
  loading?: boolean;
  agentName: string;
  prompts: string[];
  copiedIndex: number | null;
  expandedTraces: Record<string, boolean>;
  traceTabs: Record<string, 'traces' | 'audit'>;
  endRef: RefObject<HTMLDivElement | null>;
  containerRef?: RefObject<HTMLDivElement | null>;
  onScroll?: () => void;
  onPrompt: (prompt: string) => void;
  onCopy: (content: string, index: number) => void;
  onToggleTrace: (messageID: string) => void;
  onTraceTabChange: (messageID: string, tab: 'traces' | 'audit') => void;
}) {
  return (
    <div
      ref={containerRef}
      onScroll={onScroll}
      className="h-full flex-1 space-y-4 overflow-y-auto pr-2 min-h-0 relative"
      aria-live="polite"
    >
      {messages.length === 0 ? (
        <ChatEmptyState agentName={agentName} prompts={prompts} onPrompt={onPrompt} />
      ) : messages.map((message, index) => (
        <MessageBubble
          key={message.id || index}
          message={message}
          copied={copiedIndex === index}
          traceExpanded={Boolean(expandedTraces[message.id])}
          traceTab={traceTabs[message.id] || 'traces'}
          onCopy={() => onCopy(message.content, index)}
          onToggleTrace={() => onToggleTrace(message.id)}
          onTraceTabChange={(tab) => onTraceTabChange(message.id, tab)}
        />
      ))}
      <div ref={endRef} />
    </div>
  );
}

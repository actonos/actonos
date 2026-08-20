export interface AuditLogItem {
  timestamp: string;
  agent_id: string;
  action: string;
  tool_name?: string;
  parameters?: unknown;
  status: string;
  verification: string;
  duration_ms: number;
}

export interface ToolCallTrace {
  tool: string;
  args?: unknown;
  result?: string;
  status?: string;
  latency_ms?: number;
}

export interface ChatMessage {
  id: string;
  role: 'user' | 'assistant' | 'system';
  content: string;
  timestamp: string;
  model?: string;
  tokens_used?: number;
  thought?: string;
  reasoning?: string;
  toolCalls?: ToolCallTrace[];
  auditLogs?: AuditLogItem[];
}

export function formatRelativeTime(dateStr: string | undefined, locale: string): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  const diffSeconds = Math.round((date.getTime() - Date.now()) / 1000);
  const formatter = new Intl.RelativeTimeFormat(locale, { numeric: 'auto' });

  if (Math.abs(diffSeconds) < 60) return formatter.format(diffSeconds, 'second');
  const diffMinutes = Math.round(diffSeconds / 60);
  if (Math.abs(diffMinutes) < 60) return formatter.format(diffMinutes, 'minute');
  const diffHours = Math.round(diffMinutes / 60);
  if (Math.abs(diffHours) < 24) return formatter.format(diffHours, 'hour');
  const diffDays = Math.round(diffHours / 24);
  if (Math.abs(diffDays) < 7) return formatter.format(diffDays, 'day');
  return new Intl.DateTimeFormat(locale, { month: 'short', day: 'numeric' }).format(date);
}

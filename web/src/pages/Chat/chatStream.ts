import { isChatSourcedApproval, type ApprovalRequest } from '@/lib/types';
import type { AuditLogItem, ChatMessage, ToolCallTrace } from './chatTypes';

export function isApprovalRequiredErrorText(error: unknown): boolean {
  const text = typeof error === 'string' ? error : '';
  return text.includes('human approval required') || text.includes('approval_id=');
}

function asRecord(value: unknown): Record<string, unknown> | null {
  return typeof value === 'object' && value !== null ? (value as Record<string, unknown>) : null;
}

export function attachPendingApprovalToMessages(
  messages: ChatMessage[],
  approvals: ApprovalRequest[],
  agentID: string
): ChatMessage[] {
  const pending = approvals.find(
    (item) => item.agent_id === agentID && item.status === 'pending' && isChatSourcedApproval(item)
  );
  if (!pending) return messages;
  let lastAssistantIndex = -1;
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].role === 'assistant') {
      lastAssistantIndex = index;
      break;
    }
  }
  if (lastAssistantIndex < 0) return messages;
  return messages.map((message, index) => {
    if (index !== lastAssistantIndex) return message;
    const toolCalls = message.toolCalls?.length
      ? message.toolCalls.map((call) =>
          call.tool === pending.tool_name ? { ...call, status: 'awaiting_approval' } : call
        )
      : [{ tool: pending.tool_name, status: 'awaiting_approval', args: pending.input }];
    return { ...message, pendingApproval: pending, finalized: false, toolCalls };
  });
}

export function parseApprovalPayload(value: unknown): ApprovalRequest | undefined {
  const rec = asRecord(value);
  if (!rec || typeof rec.id !== 'string' || typeof rec.tool_name !== 'string') {
    return undefined;
  }
  return value as ApprovalRequest;
}

function updateMatchingToolCall(
  calls: ToolCallTrace[],
  parsed: Record<string, unknown>,
  patch: Partial<ToolCallTrace>
): ToolCallTrace[] {
  const toolCallId = typeof parsed.tool_call_id === 'string' ? parsed.tool_call_id : undefined;
  const tool = typeof parsed.tool === 'string' ? parsed.tool : undefined;
  let updated = false;
  return calls.map((call) => {
    if (updated) return call;
    const matches = toolCallId
      ? call.toolCallId === toolCallId
      : Boolean(tool) && call.tool === tool;
    if (!matches) return call;
    updated = true;
    return { ...call, ...patch };
  });
}

export function applyChatStreamEvent(
  message: ChatMessage,
  event: string,
  parsed: Record<string, unknown>
): ChatMessage {
  if (event === 'thought' && typeof parsed.thought === 'string') {
    return { ...message, thought: parsed.thought };
  }

  if (event === 'reasoning' && typeof parsed.reasoning === 'string') {
    const segs = [...(message.segments || [])];
    const last = segs[segs.length - 1];
    if (last && last.type === 'reasoning') {
      segs[segs.length - 1] = { ...last, text: last.text + parsed.reasoning };
    } else {
      segs.push({ type: 'reasoning', text: parsed.reasoning });
    }
    return {
      ...message,
      reasoning: (message.reasoning ?? '') + parsed.reasoning,
      segments: segs,
    };
  }

  if (event === 'token' && typeof parsed.content === 'string') {
    const segs = [...(message.segments || [])];
    const last = segs[segs.length - 1];
    if (last && last.type === 'content') {
      segs[segs.length - 1] = { ...last, text: last.text + parsed.content };
    } else {
      segs.push({ type: 'content', text: parsed.content });
    }
    return {
      ...message,
      content: message.content + parsed.content,
      thought: undefined,
      segments: segs,
    };
  }

  if (event === 'token_reset') {
    return {
      ...message,
      content: '',
      segments: (message.segments || []).filter((segment) => segment.type === 'reasoning'),
    };
  }

  if (event === 'tool_call') {
    const newToolCall: ToolCallTrace = {
      tool: typeof parsed.tool === 'string' ? parsed.tool : 'native_tool',
      toolCallId: typeof parsed.tool_call_id === 'string' ? parsed.tool_call_id : undefined,
      args: parsed.args,
      status: 'running',
    };
    return {
      ...message,
      toolCalls: [...(message.toolCalls || []), newToolCall],
    };
  }

  if (event === 'tool_result') {
    const status = typeof parsed.status === 'string' ? parsed.status : undefined;
    const next: ChatMessage = {
      ...message,
      toolCalls: updateMatchingToolCall(message.toolCalls || [], parsed, {
        result: typeof parsed.result === 'string' ? parsed.result : undefined,
        status,
        latency_ms: typeof parsed.latency_ms === 'number' ? parsed.latency_ms : undefined,
      }),
    };
    if (status && status !== 'awaiting_approval' && status !== 'running') {
      next.pendingApproval = undefined;
    }
    return next;
  }

  if (event === 'audit' && parsed.audit_log) {
    return {
      ...message,
      auditLogs: [...(message.auditLogs || []), parsed.audit_log as AuditLogItem],
    };
  }

  if (event === 'approval_required') {
    const approval = parseApprovalPayload(parsed.approval);
    const tool = typeof parsed.tool === 'string' ? parsed.tool : approval?.tool_name;
    return {
      ...message,
      pendingApproval: approval,
      finalized: false,
      thought: undefined,
      toolCalls: updateMatchingToolCall(message.toolCalls || [], parsed, {
        status: 'awaiting_approval',
        tool: tool,
      }),
    };
  }

  if (event === 'done') {
    const fallback =
      typeof parsed.content === 'string' && parsed.content ? parsed.content : undefined;
    return {
      ...message,
      content:
        message.content ||
        fallback ||
        (message.toolCalls?.length ? message.content : ''),
      model: typeof parsed.model === 'string' ? parsed.model : message.model,
      tokens_used: typeof parsed.tokens_used === 'number' ? parsed.tokens_used : message.tokens_used,
      thought: undefined,
      finalized: true,
      pendingApproval: undefined,
    };
  }

  if (event === 'error') {
    if (message.pendingApproval || isApprovalRequiredErrorText(parsed.error)) {
      return {
        ...message,
        finalized: false,
      };
    }
    const errorText = typeof parsed.error === 'string' ? parsed.error : 'unknown error';
    return {
      ...message,
      content: message.content + `\n\nError: ${errorText}`,
      thought: undefined,
      finalized: true,
    };
  }

  return message;
}

/** Keep a live assistant bubble even if a snapshot replaced its optimistic id. */
export function upsertStreamingAssistant(
  messages: ChatMessage[],
  assistantMsgId: string,
  next: ChatMessage
): ChatMessage[] {
  const byId = messages.findIndex((message) => message.id === assistantMsgId);
  if (byId >= 0) {
    return messages.map((message, index) => (index === byId ? next : message));
  }
  let lastAssistant = -1;
  for (let index = messages.length - 1; index >= 0; index -= 1) {
    if (messages[index].role === 'assistant') {
      lastAssistant = index;
      break;
    }
  }
  if (lastAssistant >= 0 && messages[lastAssistant].finalized === false) {
    const existingId = messages[lastAssistant].id;
    return messages.map((message, index) =>
      index === lastAssistant ? { ...next, id: existingId } : message
    );
  }
  return [...messages, next];
}

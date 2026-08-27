import { describe, expect, it } from 'vitest';
import type { ApprovalRequest } from '@/lib/types';
import {
  applyChatStreamEvent,
  attachPendingApprovalToMessages,
  isApprovalRequiredErrorText,
  upsertStreamingAssistant,
} from './chatStream';
import type { ChatMessage } from './chatTypes';

const baseMessage = (): ChatMessage => ({
  id: 'msg_1',
  role: 'assistant',
  content: '',
  timestamp: '10:00',
  toolCalls: [{ tool: 'native_file_write', toolCallId: 'call-1', status: 'running' }],
  finalized: false,
});

const approval: ApprovalRequest = {
  id: 'apr_1',
  trace_id: 'trace-1',
  agent_id: 'agent_system_core',
  tool_name: 'native_file_write',
  risk_level: 'High',
  action_hash: 'hash',
  input: { path: 'notes.txt' },
  status: 'pending',
  requested_at: new Date().toISOString(),
  expires_at: new Date().toISOString(),
  source: 'stream',
};

describe('applyChatStreamEvent', () => {
  it('pauses on approval_required without finalizing an error', () => {
    const next = applyChatStreamEvent(baseMessage(), 'approval_required', {
      tool: 'native_file_write',
      tool_call_id: 'call-1',
      approval,
    });
    expect(next.pendingApproval?.id).toBe('apr_1');
    expect(next.finalized).toBe(false);
    expect(next.content).toBe('');
    expect(next.toolCalls?.[0]?.status).toBe('awaiting_approval');
  });

  it('does not treat approval-required SSE errors as a failed turn', () => {
    const paused = applyChatStreamEvent(baseMessage(), 'approval_required', {
      tool: 'native_file_write',
      tool_call_id: 'call-1',
      approval,
    });
    const next = applyChatStreamEvent(paused, 'error', {
      error: 'human approval required: approval_id=apr_1 tool=native_file_write risk=High',
    });
    expect(next.finalized).toBe(false);
    expect(next.content).toBe('');
    expect(next.pendingApproval?.id).toBe('apr_1');
  });

  it('continues the same bubble after an approved tool_result', () => {
    const paused = applyChatStreamEvent(baseMessage(), 'approval_required', {
      tool: 'native_file_write',
      tool_call_id: 'call-1',
      approval,
    });
    const withResult = applyChatStreamEvent(paused, 'tool_result', {
      tool: 'native_file_write',
      tool_call_id: 'call-1',
      result: 'wrote notes.txt',
      status: 'success',
    });
    const done = applyChatStreamEvent(withResult, 'token', { content: 'File saved.' });
    expect(withResult.pendingApproval).toBeUndefined();
    expect(done.content).toBe('File saved.');
    expect(done.finalized).not.toBe(true);
  });
});

describe('attachPendingApprovalToMessages', () => {
  it('pins a chat-sourced pending approval onto the last assistant message', () => {
    const messages: ChatMessage[] = [
      { id: 'u1', role: 'user', content: 'write it', timestamp: '10:00' },
      { id: 'a1', role: 'assistant', content: '', timestamp: '10:01', toolCalls: [{ tool: 'native_file_write', status: 'success' }] },
    ];
    const next = attachPendingApprovalToMessages(messages, [approval], 'agent_system_core');
    expect(next[1]?.pendingApproval?.id).toBe('apr_1');
    expect(next[1]?.toolCalls?.[0]?.status).toBe('awaiting_approval');
  });
});

describe('isApprovalRequiredErrorText', () => {
  it('detects the engine approval error string', () => {
    expect(isApprovalRequiredErrorText('human approval required: approval_id=apr_1 tool=x risk=High')).toBe(true);
  });
});

describe('upsertStreamingAssistant', () => {
  it('replaces the optimistic assistant bubble by id', () => {
    const messages: ChatMessage[] = [
      { id: 'u1', role: 'user', content: 'hi', timestamp: '10:00' },
      { id: 'a-opt', role: 'assistant', content: '', timestamp: '10:00', finalized: false },
    ];
    const next: ChatMessage = {
      id: 'a-opt',
      role: 'assistant',
      content: 'Hello.',
      timestamp: '10:00',
      finalized: false,
    };
    const result = upsertStreamingAssistant(messages, 'a-opt', next);
    expect(result).toHaveLength(2);
    expect(result[1]?.content).toBe('Hello.');
  });

  it('reinserts the live bubble after a user-only snapshot', () => {
    const messages: ChatMessage[] = [
      { id: 'u-server', role: 'user', content: 'hi', timestamp: '10:00' },
    ];
    const next: ChatMessage = {
      id: 'a-opt',
      role: 'assistant',
      content: 'Hello.',
      timestamp: '10:00',
      finalized: false,
    };
    const result = upsertStreamingAssistant(messages, 'a-opt', next);
    expect(result).toHaveLength(2);
    expect(result[1]?.id).toBe('a-opt');
    expect(result[1]?.content).toBe('Hello.');
  });

  it('continues an unfinalized assistant that lost its optimistic id', () => {
    const messages: ChatMessage[] = [
      { id: 'u1', role: 'user', content: 'hi', timestamp: '10:00' },
      { id: 'a-server', role: 'assistant', content: '', timestamp: '10:00', finalized: false },
    ];
    const next: ChatMessage = {
      id: 'a-opt',
      role: 'assistant',
      content: 'Hello.',
      timestamp: '10:00',
      finalized: false,
    };
    const result = upsertStreamingAssistant(messages, 'a-opt', next);
    expect(result).toHaveLength(2);
    expect(result[1]?.id).toBe('a-server');
    expect(result[1]?.content).toBe('Hello.');
  });
});

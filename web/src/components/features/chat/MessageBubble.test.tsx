import { cleanup, render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { afterEach, describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import { ToastProvider } from '@/components/ui/Toast';
import type { ApprovalRequest } from '@/lib/types';
import type { ChatMessage } from '@/pages/Chat/chatTypes';
import { MessageBubble } from './MessageBubble';

vi.mock('@/lib/api', () => ({
  api: {
    approveAction: vi.fn(),
    rejectAction: vi.fn(),
  },
}));

const approval: ApprovalRequest = {
  id: 'apr_bubble',
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

function renderBubble(message: ChatMessage, traceExpanded = false) {
  return render(
    <I18nextProvider i18n={i18n}>
      <ToastProvider>
        <MessageBubble
          message={message}
          copied={false}
          traceExpanded={traceExpanded}
          traceTab="traces"
          onCopy={() => undefined}
          onToggleTrace={() => undefined}
          onTraceTabChange={() => undefined}
        />
      </ToastProvider>
    </I18nextProvider>
  );
}

describe('MessageBubble approval placement', () => {
  afterEach(() => {
    cleanup();
  });

  it('renders the approval card after live reasoning so it stays at the visible end of the bubble', () => {
    renderBubble({
      id: 'a1',
      role: 'assistant',
      content: '',
      timestamp: '10:00',
      finalized: false,
      pendingApproval: approval,
      segments: [{ type: 'reasoning', text: 'Considering whether to write notes.txt' }],
    });

    const reasoning = screen.getByText(/Considering whether to write notes.txt/i);
    const approvalRegion = screen.getByRole('region', { name: /approval|phê duyệt/i });
    expect(reasoning.compareDocumentPosition(approvalRegion) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('keeps the approval card after expanded execution activity', () => {
    renderBubble(
      {
        id: 'a1',
        role: 'assistant',
        content: '',
        timestamp: '10:00',
        finalized: false,
        pendingApproval: approval,
        toolCalls: [{ tool: 'native_file_write', status: 'awaiting_approval', args: { path: 'notes.txt' } }],
        segments: [{ type: 'reasoning', text: 'Considering whether to write notes.txt' }],
      },
      true
    );

    expect(screen.getByRole('button', { expanded: true })).toBeInTheDocument();
    const toolName = screen.getAllByText('native_file_write')[0];
    const approvalRegion = screen.getByRole('region', { name: /approval|phê duyệt/i });
    expect(toolName.compareDocumentPosition(approvalRegion) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  });

  it('does not show a completed placeholder while tools are still running', () => {
    renderBubble({
      id: 'a1',
      role: 'assistant',
      content: '',
      timestamp: '10:00',
      finalized: false,
      toolCalls: [{ tool: 'native_http_fetch', status: 'running' }],
    });

    expect(screen.queryByText(/Completed operations successfully|Đã hoàn thành các thao tác/i)).not.toBeInTheDocument();
    expect(screen.getByText(/Running tools|Đang chạy công cụ/i)).toBeInTheDocument();
  });

  it('shows the completed placeholder only after the turn is finalized', () => {
    renderBubble({
      id: 'a1',
      role: 'assistant',
      content: '',
      timestamp: '10:00',
      finalized: true,
      toolCalls: [{ tool: 'native_http_fetch', status: 'success' }],
    });

    expect(screen.getByText(/Completed operations successfully|Đã hoàn thành các thao tác/i)).toBeInTheDocument();
    expect(screen.queryByText(/Running tools|Đang chạy công cụ/i)).not.toBeInTheDocument();
  });
});

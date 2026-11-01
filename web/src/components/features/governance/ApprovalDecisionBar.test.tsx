import { cleanup, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApprovalDecisionBar } from './ApprovalDecisionBar';
import type { ApprovalRequest } from '@/lib/types';

const labels = { reject: 'Reject', approve: 'Approve', dontAskTask: 'Not this task', dontAskToday: 'Not today' };

function approval(tool: string): ApprovalRequest {
  return {
    id: 'a1', trace_id: 't', agent_id: 'ag', tool_name: tool, risk_level: 'High', action_hash: 'h', input: {},
    status: 'pending', requested_at: '2026-11-01T00:00:00Z', expires_at: '2026-11-02T00:00:00Z',
  } as ApprovalRequest;
}

describe('ApprovalDecisionBar', () => {
  afterEach(cleanup);

  it('offers "don\'t ask again" for ordinary tools', () => {
    render(<ApprovalDecisionBar approval={approval('native_file_write')} deciding={false} labels={labels} onReject={vi.fn()} onApprove={vi.fn()} />);
    expect(screen.getByText('Not today')).toBeInTheDocument();
  });

  it.each(['native_mail_send', 'native_calendar_create_event', 'native_mail_delete'])(
    'never offers it for %s, whose effect leaves the device',
    (tool) => {
      render(<ApprovalDecisionBar approval={approval(tool)} deciding={false} labels={labels} onReject={vi.fn()} onApprove={vi.fn()} />);
      expect(screen.getByText('Approve')).toBeInTheDocument();
      expect(screen.queryByText('Not today')).not.toBeInTheDocument();
    }
  );
});

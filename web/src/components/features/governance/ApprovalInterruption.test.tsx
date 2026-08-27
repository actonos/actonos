import { render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import type { ApprovalRequest } from '@/lib/types';
import { ToastProvider } from '@/components/ui/Toast';
import { ApprovalInterruption } from './ApprovalInterruption';

vi.mock('@/components/providers/RealtimeProvider', () => ({
  useRealtime: () => ({ snapshot: null }),
}));

vi.mock('@/lib/api', () => ({
  api: {
    approveAction: vi.fn(),
    rejectAction: vi.fn(),
    listApprovals: vi.fn(),
  },
}));

const streamApproval: ApprovalRequest = {
  id: 'apr_stream',
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

const heartbeatApproval: ApprovalRequest = {
  ...streamApproval,
  id: 'apr_beat',
  source: 'heartbeat',
  tool_name: 'native_exec',
};

describe('ApprovalInterruption', () => {
  it('does not open the overlay for chat stream approvals', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <ToastProvider>
          <ApprovalInterruption />
        </ToastProvider>
      </I18nextProvider>
    );
    window.dispatchEvent(new CustomEvent('actonos:approval-required', { detail: streamApproval }));
    expect(screen.queryByRole('alertdialog')).not.toBeInTheDocument();
  });

  it('opens the overlay for heartbeat approvals', async () => {
    render(
      <I18nextProvider i18n={i18n}>
        <ToastProvider>
          <ApprovalInterruption />
        </ToastProvider>
      </I18nextProvider>
    );
    window.dispatchEvent(new CustomEvent('actonos:approval-required', { detail: heartbeatApproval }));
    expect(await screen.findByRole('alertdialog')).toBeInTheDocument();
  });
});

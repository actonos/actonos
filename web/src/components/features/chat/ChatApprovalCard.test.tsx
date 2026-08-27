import { render, screen } from '@testing-library/react';
import { I18nextProvider } from 'react-i18next';
import { describe, expect, it, vi } from 'vitest';
import i18n from '@/lib/i18n';
import type { ApprovalRequest } from '@/lib/types';
import { ToastProvider } from '@/components/ui/Toast';
import { ChatApprovalCard } from './ChatApprovalCard';

vi.mock('@/lib/api', () => ({
  api: {
    approveAction: vi.fn().mockResolvedValue({ status: 'continued' }),
    rejectAction: vi.fn().mockResolvedValue({}),
  },
}));

const approval: ApprovalRequest = {
  id: 'apr_card',
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

describe('ChatApprovalCard', () => {
  it('shows only approve and reject actions', () => {
    render(
      <I18nextProvider i18n={i18n}>
        <ToastProvider>
          <ChatApprovalCard approval={approval} />
        </ToastProvider>
      </I18nextProvider>
    );
    expect(screen.getByRole('button', { name: /approve|chấp thuận/i })).toBeEnabled();
    expect(screen.getByRole('button', { name: /reject|từ chối/i })).toBeEnabled();
    expect(screen.queryByRole('textbox')).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /don't ask|không hỏi/i })).not.toBeInTheDocument();
  });
});

import { describe, expect, it } from 'vitest';
import { isApprovalRequired } from './types';

describe('isApprovalRequired', () => {
  it('recognizes a durable approval response', () => {
    expect(isApprovalRequired({
      status: 'approval_required',
      approval: {
        id: 'approval-1',
        trace_id: 'trace-1',
        agent_id: 'agent_system_core',
        tool_name: 'admin_workspace_write',
        risk_level: 'High',
        action_hash: 'hash',
        input: {},
        status: 'pending',
        requested_at: new Date().toISOString(),
        expires_at: new Date().toISOString(),
      },
    })).toBe(true);
  });

  it('rejects completed mutations', () => {
    expect(isApprovalRequired({ status: 'saved' })).toBe(false);
  });
});

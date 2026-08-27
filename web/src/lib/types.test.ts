import { describe, expect, it } from 'vitest';
import { isApprovalRequired, isModalEligibleApproval } from './types';

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

describe('isModalEligibleApproval', () => {
  const base = {
    id: 'approval-1',
    trace_id: 'trace-1',
    agent_id: 'agent_system_core',
    tool_name: 'native_file_write',
    risk_level: 'High' as const,
    action_hash: 'hash',
    input: {},
    status: 'pending' as const,
    requested_at: new Date().toISOString(),
    expires_at: new Date().toISOString(),
  };

  it('hides the overlay for chat and stream sources', () => {
    expect(isModalEligibleApproval({ ...base, source: 'stream' })).toBe(false);
    expect(isModalEligibleApproval({ ...base, source: 'chat' })).toBe(false);
  });

  it('keeps the overlay for heartbeat, missions, and REST mutations', () => {
    expect(isModalEligibleApproval({ ...base, source: 'heartbeat' })).toBe(true);
    expect(isModalEligibleApproval({ ...base, task_id: 'task_1' })).toBe(true);
    expect(isModalEligibleApproval(base)).toBe(true);
  });
});

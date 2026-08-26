import { describe, expect, it } from 'vitest';
import type { AgentRun } from '@/lib/types';
import { isRunNotFoundError, mergeVisibleRuns } from './operations-runs';

function run(partial: Partial<AgentRun> & Pick<AgentRun, 'id' | 'goal'>): AgentRun {
  return {
    trace_id: 'tr',
    agent_id: 'agent_system_core',
    source: 'chat',
    status: 'completed',
    iterations: 1,
    prompt_tokens: 1,
    completion_tokens: 1,
    total_tokens: 2,
    started_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...partial,
  };
}

describe('mergeVisibleRuns', () => {
  it('prepends a fetched run that is absent from the snapshot', () => {
    const snapshot = [run({ id: 'run_new', goal: 'recent' })];
    const fetched = run({ id: 'run_old', goal: 'long running heartbeat', status: 'running' });
    const merged = mergeVisibleRuns(snapshot, fetched);
    expect(merged.map((item) => item.id)).toEqual(['run_old', 'run_new']);
    expect(merged.find((item) => item.id === 'run_old')?.goal).toBe('long running heartbeat');
  });

  it('does not duplicate when the snapshot already has the run', () => {
    const snapshot = [run({ id: 'run_old', goal: 'from snapshot' })];
    const fetched = run({ id: 'run_old', goal: 'from fetch' });
    const merged = mergeVisibleRuns(snapshot, fetched);
    expect(merged).toHaveLength(1);
    expect(merged[0].goal).toBe('from snapshot');
  });
});

describe('isRunNotFoundError', () => {
  it('matches 404 payloads from GET /api/runs/{id}', () => {
    expect(isRunNotFoundError(new Error('run not found'))).toBe(true);
    expect(isRunNotFoundError(new Error('HTTP Error 404'))).toBe(true);
    expect(isRunNotFoundError(new Error('network down'))).toBe(false);
  });
});

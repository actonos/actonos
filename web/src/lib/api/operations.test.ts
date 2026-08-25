import { afterEach, describe, expect, it, vi } from 'vitest';
import { operationsApi } from './operations';

describe('operationsApi run control plane', () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it('lists runs with status and agent filters', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: { runs: [] } }),
    }));
    await operationsApi.listAgentRuns({ limit: 20, status: 'running', agent_id: 'agent_system_core' });
    const url = String((fetch as unknown as { mock: { calls: unknown[][] } }).mock.calls[0][0]);
    expect(url).toContain('/runs?');
    expect(url).toContain('limit=20');
    expect(url).toContain('status=running');
    expect(url).toContain('agent_id=agent_system_core');
  });

  it('posts cancel to the durable run id', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ data: { status: 'cancelled', id: 'run_1' } }),
    }));
    const result = await operationsApi.cancelAgentRun('run_1');
    expect(result).toEqual({ status: 'cancelled', id: 'run_1' });
    const [url, init] = (fetch as unknown as { mock: { calls: [string, RequestInit][] } }).mock.calls[0];
    expect(String(url)).toContain('/runs/run_1/cancel');
    expect(init.method).toBe('POST');
  });
});

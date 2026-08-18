import { describe, expect, it, vi } from 'vitest';
import { fetchJSON } from './api';

describe('fetchJSON approval contract', () => {
  it('dispatches approval-required without treating the mutation as completed', async () => {
    const approval = { id: 'approval-1', tool_name: 'admin_mcp_toggle' };
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      status: 202,
      json: async () => ({
        data: { status: 'approval_required', approval },
      }),
    }));
    const listener = vi.fn();
    window.addEventListener('actonos:approval-required', listener);

    const result = await fetchJSON('/tools/mcp/example', { method: 'PUT' });

    expect(result).toEqual({ status: 'approval_required', approval });
    expect(listener).toHaveBeenCalledOnce();
    expect((listener.mock.calls[0][0] as CustomEvent).detail).toEqual(approval);
    window.removeEventListener('actonos:approval-required', listener);
    vi.unstubAllGlobals();
  });
});

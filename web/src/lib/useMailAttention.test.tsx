import { act, cleanup, renderHook, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useMailAttention } from './useMailAttention';

const listProposals = vi.fn();
let approvals: { tool_name: string }[] = [];

vi.mock('@/components/providers/RealtimeProvider', () => ({ useRealtime: () => ({ snapshot: { approvals } }) }));
vi.mock('@/lib/api/mailcal', async () => {
  const actual = await vi.importActual<typeof import('@/lib/api/mailcal')>('@/lib/api/mailcal');
  return { ...actual, mailcalApi: { listProposals: (...a: unknown[]) => listProposals(...a) } };
});

beforeEach(() => {
  listProposals.mockReset();
  approvals = [];
});
afterEach(() => {
  cleanup();
  vi.useRealTimers();
});

describe('useMailAttention', () => {
  it('counts pending proposals and approvals for tools that leave the device, and nothing else', async () => {
    listProposals.mockResolvedValue({ proposals: [], count: 2 });
    approvals = [{ tool_name: 'native_mail_send' }, { tool_name: 'native_file_write' }, { tool_name: 'native_calendar_create_event' }];
    const { result } = renderHook(() => useMailAttention());
    await waitFor(() => expect(result.current).toBe(4));
  });

  it('is zero when the API fails', async () => {
    listProposals.mockRejectedValue(new Error('down'));
    const { result } = renderHook(() => useMailAttention());
    await waitFor(() => expect(listProposals).toHaveBeenCalled());
    expect(result.current).toBe(0);
  });

  it('refreshes on a timer', async () => {
    vi.useFakeTimers();
    listProposals.mockResolvedValue({ proposals: [], count: 1 });
    const { result } = renderHook(() => useMailAttention());
    await act(async () => { await Promise.resolve(); });
    expect(result.current).toBe(1);
    listProposals.mockResolvedValue({ proposals: [], count: 3 });
    await act(async () => { await vi.advanceTimersByTimeAsync(60_000); });
    expect(result.current).toBe(3);
  });
});

import { act, render, screen } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { RealtimeProvider, useRealtime } from './RealtimeProvider';

class MockSocket {
  onopen: (() => void) | null = null;
  onmessage: ((event: MessageEvent) => void) | null = null;
  onclose: (() => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
}

const sockets: MockSocket[] = [];
vi.mock('@/lib/api', () => ({
  createRealtimeSocket: () => {
    const socket = new MockSocket();
    sockets.push(socket);
    return socket;
  },
}));

function Probe() {
  const { connection, snapshot } = useRealtime();
  return <div>{connection}:{snapshot?.type ?? 'none'}</div>;
}

describe('RealtimeProvider', () => {
  afterEach(() => {
    sockets.length = 0;
    vi.useRealTimers();
  });

  it('accepts snapshots and rejects malformed frames', () => {
    render(<RealtimeProvider><Probe /></RealtimeProvider>);
    const socket = sockets[0];
    act(() => socket.onopen?.());
    expect(screen.getByText('online:none')).toBeInTheDocument();

    act(() => socket.onmessage?.({ data: JSON.stringify({ type: 'snapshot' }) } as MessageEvent));
    expect(screen.getByText('online:snapshot')).toBeInTheDocument();

    act(() => socket.onmessage?.({ data: '{invalid' } as MessageEvent));
    expect(socket.close).toHaveBeenCalledWith(1003, 'invalid message');
  });

  it('reconnects after a bounded backoff', () => {
    vi.useFakeTimers();
    vi.spyOn(Math, 'random').mockReturnValue(0);
    render(<RealtimeProvider><Probe /></RealtimeProvider>);
    act(() => sockets[0].onclose?.());
    expect(screen.getByText('offline:none')).toBeInTheDocument();

    act(() => vi.advanceTimersByTime(2000));
    expect(sockets).toHaveLength(2);
    expect(screen.getByText('connecting:none')).toBeInTheDocument();
  });
});

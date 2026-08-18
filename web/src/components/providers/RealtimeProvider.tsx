import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { createRealtimeSocket } from '@/lib/api';
import type { RealtimeSnapshot } from '@/lib/types';

export type RealtimeConnection = 'connecting' | 'online' | 'offline';

interface RealtimeContextValue {
  connection: RealtimeConnection;
  snapshot: RealtimeSnapshot | null;
}

const RealtimeContext = createContext<RealtimeContextValue>({
  connection: 'connecting',
  snapshot: null,
});

export function RealtimeProvider({ children }: { children: ReactNode }) {
  const [connection, setConnection] = useState<RealtimeConnection>('connecting');
  const [snapshot, setSnapshot] = useState<RealtimeSnapshot | null>(null);

  useEffect(() => {
    let socket: WebSocket | null = null;
    let timer: number | undefined;
    let disposed = false;
    let attempts = 0;

    const connect = () => {
      setConnection('connecting');
      socket = createRealtimeSocket();
      socket.onopen = () => {
        attempts = 0;
        setConnection('online');
      };
      socket.onmessage = (message) => {
        try {
          const next = JSON.parse(String(message.data)) as RealtimeSnapshot;
          if (next.type === 'snapshot') setSnapshot(next);
        } catch {
          socket?.close(1003, 'invalid snapshot');
        }
      };
      socket.onclose = () => {
        setConnection('offline');
        if (disposed) return;
        attempts += 1;
        const backoff = Math.min(30000, 1000 * 2 ** Math.min(attempts, 5));
        const jitter = Math.floor(Math.random() * 500);
        timer = window.setTimeout(connect, backoff + jitter);
      };
      socket.onerror = () => socket?.close();
    };

    connect();
    return () => {
      disposed = true;
      if (timer) window.clearTimeout(timer);
      socket?.close();
    };
  }, []);

  const value = useMemo(() => ({ connection, snapshot }), [connection, snapshot]);
  return <RealtimeContext.Provider value={value}>{children}</RealtimeContext.Provider>;
}

export function useRealtime() {
  return useContext(RealtimeContext);
}

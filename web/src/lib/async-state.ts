export type AsyncStatus = 'idle' | 'loading' | 'success' | 'error';

export interface AsyncState<T> {
  status: AsyncStatus;
  data: T | null;
  error?: string;
  updatedAt?: string;
}

export const idleAsyncState = <T,>(): AsyncState<T> => ({
  status: 'idle',
  data: null,
});

export function loadingAsyncState<T>(current?: T | null): AsyncState<T> {
  return { status: 'loading', data: current ?? null };
}

export function successAsyncState<T>(data: T): AsyncState<T> {
  return { status: 'success', data, updatedAt: new Date().toISOString() };
}

export function errorAsyncState<T>(error: unknown, current?: T | null): AsyncState<T> {
  return {
    status: 'error',
    data: current ?? null,
    error: error instanceof Error ? error.message : String(error),
  };
}

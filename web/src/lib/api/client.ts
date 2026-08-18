export const API_BASE = '/api';
export const HTTP_STATUS_ACCEPTED = 202;

export function getAuthHeaders(extra?: Record<string, string>): Record<string, string> {
  return { ...extra };
}

export async function fetchJSON<T>(endpoint: string, options?: RequestInit): Promise<T> {
  const headers = getAuthHeaders({
    'Content-Type': 'application/json',
    ...(options?.headers as Record<string, string>),
  });
  const response = await fetch(`${API_BASE}${endpoint}`, { ...options, headers });
  if (!response.ok) {
    let message = `HTTP Error ${response.status}`;
    try {
      const body = await response.json();
      if (body.error?.message) message = body.error.message;
    } catch {
      // Preserve the HTTP status when the error body is not JSON.
    }
    throw new Error(message);
  }
  const envelope = await response.json();
  const data = envelope.data !== undefined ? envelope.data : envelope;
  if (
    response.status === HTTP_STATUS_ACCEPTED &&
    data?.status === 'approval_required' &&
    data?.approval
  ) {
    window.dispatchEvent(
      new CustomEvent('actonos:approval-required', { detail: data.approval })
    );
  }
  return data;
}

export function createRealtimeSocket(): WebSocket {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  return new WebSocket(`${protocol}//${window.location.host}${API_BASE}/realtime`);
}

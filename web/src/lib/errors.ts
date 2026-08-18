export function getErrorMessage(cause: unknown, fallback = 'Unexpected error'): string {
  if (cause instanceof Error && cause.message.trim()) {
    return cause.message;
  }
  if (typeof cause === 'string' && cause.trim()) {
    return cause;
  }
  return fallback;
}

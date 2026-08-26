import type { AgentRun } from '@/lib/types';

/** Snapshot recency window plus a deep-linked run fetched by id. */
export function mergeVisibleRuns(snapshotRuns: AgentRun[], fetchedRun: AgentRun | null): AgentRun[] {
  if (!fetchedRun) {
    return snapshotRuns;
  }
  if (snapshotRuns.some((run) => run.id === fetchedRun.id)) {
    return snapshotRuns;
  }
  return [fetchedRun, ...snapshotRuns];
}

export function isRunNotFoundError(cause: unknown): boolean {
  const message = cause instanceof Error ? cause.message : String(cause ?? '');
  return /not found|HTTP Error 404/i.test(message);
}

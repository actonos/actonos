import type { HealthReport } from './types';

export const SUPERVISOR_COMPONENTS = ['llm', 'heartbeat', 'embedding', 'disk'] as const;
export type SupervisorComponent = (typeof SUPERVISOR_COMPONENTS)[number];

export function supervisorTone(
  status?: string
): 'success' | 'warning' | 'danger' | 'neutral' {
  switch (status) {
    case 'healthy':
      return 'success';
    case 'degraded':
    case 'stopped':
      return 'warning';
    case 'unhealthy':
      return 'danger';
    default:
      return 'neutral';
  }
}

export function isSupervisorHealthy(health: Pick<HealthReport, 'status'> | null | undefined): boolean {
  return health?.status === 'healthy';
}

export function componentStatus(
  health: HealthReport | null | undefined,
  key: SupervisorComponent
): string {
  return health?.components?.[key] || 'unknown';
}

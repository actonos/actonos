import { describe, expect, it } from 'vitest';
import { componentStatus, isSupervisorHealthy, supervisorTone } from './health';
import type { HealthReport } from './types';

const degraded: HealthReport = {
  status: 'degraded',
  components: { llm: 'unhealthy', heartbeat: 'healthy', embedding: 'degraded', disk: 'healthy' },
};

describe('supervisor health helpers', () => {
  it('maps component statuses to tones', () => {
    expect(supervisorTone('healthy')).toBe('success');
    expect(supervisorTone('degraded')).toBe('warning');
    expect(supervisorTone('unhealthy')).toBe('danger');
    expect(supervisorTone(undefined)).toBe('neutral');
  });

  it('treats only healthy as fully healthy', () => {
    expect(isSupervisorHealthy(degraded)).toBe(false);
    expect(isSupervisorHealthy({ status: 'healthy' })).toBe(true);
  });

  it('reads component status from the health report', () => {
    expect(componentStatus(degraded, 'llm')).toBe('unhealthy');
    expect(componentStatus(degraded, 'embedding')).toBe('degraded');
    expect(componentStatus(null, 'disk')).toBe('unknown');
  });
});

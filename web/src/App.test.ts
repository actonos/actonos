import { afterEach, describe, expect, it } from 'vitest';
import { tabFromLocation } from './App';

describe('hash routing', () => {
  afterEach(() => {
    window.location.hash = '';
  });

  it('restores a valid deep link', () => {
    window.location.hash = '#/operations';
    expect(tabFromLocation()).toBe('operations');
  });

  it('falls back to dashboard for an unknown route', () => {
    window.location.hash = '#/unknown';
    expect(tabFromLocation()).toBe('dashboard');
  });

  it('maps nested agent routes to Agent Studio', () => {
    window.location.hash = '#/agents/agent_system_core';
    expect(tabFromLocation()).toBe('agent-studio');
  });

  it('keeps operations as the primary route when a view query is present', () => {
    window.location.hash = '#/operations?view=runtime';
    expect(tabFromLocation()).toBe('operations');
  });
});

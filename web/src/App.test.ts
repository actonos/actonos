import { afterEach, describe, expect, it } from 'vitest';
import { tabFromLocation } from './App';
import enSettings from './locales/en/settings.json';
import viSettings from './locales/vi/settings.json';
import enCosts from './locales/en/costs.json';
import viCosts from './locales/vi/costs.json';

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

  it('maps costs as a first-class system route', () => {
    window.location.hash = '#/costs';
    expect(tabFromLocation()).toBe('costs');
  });

  it('keeps costs when the transactions view is present', () => {
    window.location.hash = '#/costs?view=transactions';
    expect(tabFromLocation()).toBe('costs');
  });

  it('maps the former settings tokens tab to costs', () => {
    window.location.hash = '#/settings?view=tokens';
    expect(tabFromLocation()).toBe('costs');
  });

  it('keeps operations compact cost view on operations', () => {
    window.location.hash = '#/operations?view=cost';
    expect(tabFromLocation()).toBe('operations');
  });

  it('restores the channels pairing surface', () => {
    window.location.hash = '#/channels';
    expect(tabFromLocation()).toBe('channels');
  });

  it('keeps channels as the primary route for pairing deep links', () => {
    window.location.hash = '#/channels?view=pairing';
    expect(tabFromLocation()).toBe('channels');
  });

  it('hosts ledger copy in the costs namespace', () => {
    expect(enCosts.tokenLedger.title).toBeTruthy();
    expect(viCosts.tokenLedger.title).toBeTruthy();
    expect('tokenLedger' in enSettings).toBe(false);
    expect('tokenLedger' in viSettings).toBe(false);
    expect('tokens' in enSettings.tabs).toBe(false);
    expect('tokens' in viSettings.tabs).toBe(false);
  });
});

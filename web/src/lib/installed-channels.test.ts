import { describe, expect, it } from 'vitest';
import { accountsFromPlugins, installedChannelsFromPlugins, livePairingRequired } from './installed-channels';
import type { PluginInfo } from './types';

function plugin(partial: Partial<PluginInfo['manifest']> & { status?: PluginInfo['status'] }): PluginInfo {
  const { status, ...manifest } = partial;
  return {
    enabled: true,
    status: status || 'running',
    manifest: {
      id: 'plugin-x',
      name: 'Plugin X',
      version: '1.0.0',
      capabilities: [],
      permissions: {},
      ...manifest,
    },
  };
}

describe('installedChannelsFromPlugins', () => {
  it('reads channel defs from installed chat plugins', () => {
    const got = installedChannelsFromPlugins([
      plugin({
        id: 'channel-zalo',
        name: 'Zalo Bot',
        capabilities: ['channel'],
        channels: [{ name: 'zalo', display_name: 'Zalo', requires_pairing: true }],
      }),
      plugin({
        id: 'tool-search',
        name: 'Search',
        capabilities: ['tool'],
      }),
    ]);
    expect(got).toEqual([
      {
        id: 'zalo',
        pluginId: 'channel-zalo',
        label: 'Zalo',
        requiresPairing: true,
        running: true,
      },
    ]);
  });

  it('falls back to plugin id when the manifest omits channel defs', () => {
    const got = installedChannelsFromPlugins([
      plugin({
        id: 'channel-line',
        name: 'LINE',
        capabilities: ['channel'],
      }),
    ]);
    expect(got[0]?.id).toBe('channel-line');
    expect(got[0]?.label).toBe('LINE');
  });

  it('does not inject a hardcoded telegram catalog', () => {
    const got = installedChannelsFromPlugins([
      plugin({
        id: 'tool-search',
        name: 'Search',
        capabilities: ['tool'],
      }),
    ]);
    expect(got.map((c) => c.id)).not.toContain('telegram');
    expect(installedChannelsFromPlugins([])).toEqual([]);
  });

  it('reads discord bot accounts from plugin config', () => {
    const got = accountsFromPlugins([
      plugin({
        id: 'channel-discord',
        name: 'Discord Bot Channel',
        capabilities: ['channel'],
        channels: [{ name: 'discord', display_name: 'Discord Bot Gateway', requires_pairing: true }],
        config: {
          accounts: [
            {
              account_id: 'astro',
              display_name: 'Astro',
              default_agent: 'agent_system_core',
              bot_token: 'secret-must-not-leak',
            },
          ],
        },
      }),
    ]);
    expect(got).toEqual([
      {
        id: 'astro',
        name: 'Astro',
        channel: 'discord',
        enabled: true,
        bound_agent_ids: ['agent_system_core'],
        requires_pairing: true,
      },
    ]);
    expect(JSON.stringify(got)).not.toContain('secret-must-not-leak');
  });

  it('synthesizes a zalo account when the plugin has no accounts array', () => {
    const got = accountsFromPlugins([
      plugin({
        id: 'channel-zalo',
        name: 'Zalo Bot Platform Channel',
        capabilities: ['channel'],
        channels: [{ name: 'zalo', display_name: 'Zalo Bot Platform', requires_pairing: true }],
        config: { zalo_bot_token: 'secret-must-not-leak', default_agent: 'agent_support' },
      }),
    ]);
    expect(got).toEqual([
      {
        id: 'channel-zalo',
        name: 'Zalo Bot Platform',
        channel: 'zalo',
        enabled: true,
        bound_agent_ids: ['agent_support'],
        requires_pairing: true,
      },
    ]);
  });

  it('keeps every channel def from one plugin', () => {
    const got = installedChannelsFromPlugins([
      plugin({
        id: 'channel-pack',
        name: 'Chat Pack',
        capabilities: ['channel'],
        channels: [
          { name: 'zalo', display_name: 'Zalo', requires_pairing: true },
          { name: 'line', display_name: 'LINE', requires_pairing: false },
        ],
      }),
    ]);
    expect(got.map((c) => c.id)).toEqual(['line', 'zalo']);
  });
});

describe('livePairingRequired', () => {
  it('uses the plugin default until a policy is stored', () => {
    expect(livePairingRequired('zalo', {}, true)).toBe(true);
    expect(livePairingRequired('zalo', {}, false)).toBe(false);
  });

  it('lets a stored policy turn pairing off even when the plugin default is on', () => {
    expect(livePairingRequired('zalo', { zalo: false }, true)).toBe(false);
    expect(livePairingRequired('discord', { discord: true }, false)).toBe(true);
  });
});

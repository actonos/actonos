import { useEffect, useState } from 'react';
import { api } from '@/lib/api';
import type { ChannelAccount, PluginInfo } from '@/lib/types';

export interface InstalledChannel {
  id: string;
  pluginId: string;
  label: string;
  requiresPairing: boolean;
  running: boolean;
}

export function installedChannelsFromPlugins(plugins: PluginInfo[]): InstalledChannel[] {
  const out: InstalledChannel[] = [];
  const seen = new Set<string>();
  for (const plugin of plugins) {
    const caps = plugin.manifest.capabilities || [];
    const defs = plugin.manifest.channels || [];
    if (!caps.includes('channel') && defs.length === 0) {
      continue;
    }
    const entries = defs.length > 0
      ? defs
      : [{ name: plugin.manifest.id, display_name: plugin.manifest.name, requires_pairing: false }];
    for (const def of entries) {
      const id = (def.name || plugin.manifest.id || '').trim().toLowerCase();
      if (!id || seen.has(id)) {
        continue;
      }
      seen.add(id);
      out.push({
        id,
        pluginId: plugin.manifest.id,
        label: def.display_name || plugin.manifest.name || id,
        requiresPairing: Boolean(def.requires_pairing),
        running: plugin.status === 'running',
      });
    }
  }
  return out.sort((a, b) => a.label.localeCompare(b.label));
}

function asRecord(value: unknown): Record<string, unknown> | null {
  if (value && typeof value === 'object' && !Array.isArray(value)) {
    return value as Record<string, unknown>;
  }
  return null;
}

function asString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function configAccountMaps(config?: Record<string, unknown>): Record<string, unknown>[] {
  const raw = config?.accounts;
  if (!Array.isArray(raw)) return [];
  return raw.map(asRecord).filter((item): item is Record<string, unknown> => item !== null);
}

export function accountsFromPlugins(plugins: PluginInfo[]): ChannelAccount[] {
  const out: ChannelAccount[] = [];
  for (const plugin of plugins) {
    const channels = installedChannelsFromPlugins([plugin]);
    if (channels.length === 0) continue;
    const channel = channels[0];
    const items = configAccountMaps(plugin.manifest.config);
    const rows = items.length > 0
      ? items
      : [{
          account_id: plugin.manifest.id,
          display_name: channel.label,
          default_agent: asString(plugin.manifest.config?.default_agent),
        }];
    const seen = new Set<string>();
    for (const raw of rows) {
      const id = asString(raw.account_id) || asString(raw.id) || plugin.manifest.id;
      if (!id || seen.has(id)) continue;
      seen.add(id);
      const agent = asString(raw.default_agent);
      const enabledFlag = typeof raw.enabled === 'boolean' ? raw.enabled : true;
      out.push({
        id,
        name: asString(raw.display_name) || asString(raw.name) || channel.label,
        channel: channel.id,
        enabled: enabledFlag && plugin.enabled,
        bound_agent_ids: agent ? [agent] : ['*'],
        requires_pairing: typeof raw.requires_pairing === 'boolean'
          ? raw.requires_pairing
          : channel.requiresPairing,
      });
    }
  }
  return out;
}

export function livePairingRequired(
  channelId: string,
  policies: Record<string, boolean>,
  pluginDefault = false,
): boolean {
  const id = channelId.trim().toLowerCase();
  if (!id) return false;
  if (Object.prototype.hasOwnProperty.call(policies, id)) {
    return Boolean(policies[id]);
  }
  return pluginDefault;
}

export function useInstalledChannels() {
  const [channels, setChannels] = useState<InstalledChannel[]>([]);
  const [accounts, setAccounts] = useState<ChannelAccount[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    api.listPlugins()
      .then((res) => {
        if (!cancelled) {
          const plugins = res.plugins || [];
          setChannels(installedChannelsFromPlugins(plugins));
          setAccounts(accountsFromPlugins(plugins));
        }
      })
      .catch(() => {
        if (!cancelled) {
          setChannels([]);
          setAccounts([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  return { channels, accounts, loading };
}

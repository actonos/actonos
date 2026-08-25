package plugin

import (
	"strings"

	"github.com/actonos/actonos/internal/channels"
)

// AccountsFromPlugins projects installed channel plugins into ChannelAccount
// records. Bot tokens stay in plugin config/vault and are never copied here.
func AccountsFromPlugins(plugins []PluginInfo) []channels.ChannelAccount {
	var out []channels.ChannelAccount
	for _, info := range plugins {
		out = append(out, accountsFromPlugin(info)...)
	}
	return out
}

func accountsFromPlugin(info PluginInfo) []channels.ChannelAccount {
	m := info.Manifest
	if !isChannelPlugin(m) {
		return nil
	}
	channelID := strings.ToLower(strings.TrimSpace(m.ID))
	display := m.Name
	requiresPairing := false
	if len(m.Channels) > 0 {
		if name := strings.TrimSpace(m.Channels[0].Name); name != "" {
			channelID = strings.ToLower(name)
		}
		if label := strings.TrimSpace(m.Channels[0].DisplayName); label != "" {
			display = label
		}
		requiresPairing = m.Channels[0].RequiresPairing
	}
	if channelID == "" {
		return nil
	}
	if display == "" {
		display = channelID
	}

	items := configAccountMaps(m.Config)
	if len(items) == 0 {
		items = []map[string]any{{
			"account_id":    m.ID,
			"display_name":  display,
			"default_agent": configString(m.Config, "default_agent"),
		}}
	}

	out := make([]channels.ChannelAccount, 0, len(items))
	seen := map[string]bool{}
	for _, raw := range items {
		id := firstMapString(raw, "account_id", "id")
		if id == "" {
			id = m.ID
		}
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		name := firstMapString(raw, "display_name", "name")
		if name == "" {
			name = display
		}
		agent := firstMapString(raw, "default_agent")
		bound := []string{"*"}
		if agent != "" {
			bound = []string{agent}
		}
		enabled := info.Enabled
		if v, ok := raw["enabled"].(bool); ok {
			enabled = v && info.Enabled
		}
		accRequires := requiresPairing
		if v, ok := raw["requires_pairing"].(bool); ok {
			accRequires = v
		}
		out = append(out, channels.ChannelAccount{
			ID:              id,
			Name:            name,
			Channel:         channelID,
			Enabled:         enabled,
			BoundAgentIDs:   bound,
			RequiresPairing: accRequires,
		})
	}
	return out
}

func isChannelPlugin(m PluginManifest) bool {
	for _, cap := range m.Capabilities {
		if cap == string(CapabilityChannel) {
			return true
		}
	}
	return len(m.Channels) > 0
}

func configAccountMaps(cfg map[string]any) []map[string]any {
	if cfg == nil {
		return nil
	}
	raw, ok := cfg["accounts"]
	if !ok {
		return nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, m)
	}
	return out
}

func configString(cfg map[string]any, key string) string {
	if cfg == nil {
		return ""
	}
	return asConfigString(cfg[key])
}

func firstMapString(m map[string]any, keys ...string) string {
	for _, key := range keys {
		if s := asConfigString(m[key]); s != "" {
			return s
		}
	}
	return ""
}

func asConfigString(v any) string {
	s, _ := v.(string)
	return strings.TrimSpace(s)
}

// MergeChannelAccounts keeps plugin-derived accounts and overlays matching
// disk records (pairing flags). Leftover disk accounts for channels that are
// not installed plugins are dropped.
func MergeChannelAccounts(pluginAccs, diskAccs []channels.ChannelAccount) []channels.ChannelAccount {
	installed := map[string]bool{}
	byID := make(map[string]channels.ChannelAccount, len(pluginAccs)+len(diskAccs))
	order := make([]string, 0, len(pluginAccs)+len(diskAccs))

	add := func(acc channels.ChannelAccount) {
		if acc.ID == "" {
			return
		}
		if _, exists := byID[acc.ID]; !exists {
			order = append(order, acc.ID)
		}
		byID[acc.ID] = acc
	}

	for _, acc := range pluginAccs {
		installed[strings.ToLower(acc.Channel)] = true
		add(acc)
	}
	if len(installed) == 0 {
		return diskAccs
	}
	for _, acc := range diskAccs {
		ch := strings.ToLower(acc.Channel)
		if ch != "" && !installed[ch] {
			continue
		}
		if prev, ok := byID[acc.ID]; ok {
			if acc.RequiresPairing {
				prev.RequiresPairing = true
			}
			if strings.TrimSpace(acc.Name) != "" {
				prev.Name = acc.Name
			}
			byID[acc.ID] = prev
			continue
		}
		if installed[ch] {
			add(acc)
		}
	}

	out := make([]channels.ChannelAccount, 0, len(order))
	for _, id := range order {
		out = append(out, byID[id])
	}
	return out
}

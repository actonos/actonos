package plugin

import (
	"testing"

	"github.com/actonos/actonos/internal/channels"
)

func TestAccountsFromPlugins(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []PluginInfo
		want []channels.ChannelAccount
	}{
		{
			name: "skips_tool_plugins",
			in: []PluginInfo{{
				Enabled: true,
				Manifest: PluginManifest{
					ID:           "tool-search",
					Name:         "Search",
					Capabilities: []string{string(CapabilityTool)},
				},
			}},
		},
		{
			name: "discord_accounts_array",
			in: []PluginInfo{{
				Enabled: true,
				Status:  StatusRunning,
				Manifest: PluginManifest{
					ID:           "channel-discord",
					Name:         "Discord Bot Channel",
					Capabilities: []string{string(CapabilityChannel)},
					Channels:     []PluginChannelDef{{Name: "discord", DisplayName: "Discord Bot Gateway", RequiresPairing: true}},
					Config: map[string]any{
						"accounts": []any{
							map[string]any{
								"account_id":    "astro",
								"display_name":  "Astro",
								"default_agent": "agent_system_core",
								"bot_token":     "secret-must-not-leak",
							},
						},
					},
				},
			}},
			want: []channels.ChannelAccount{{
				ID:              "astro",
				Name:            "Astro",
				Channel:         "discord",
				Enabled:         true,
				BoundAgentIDs:   []string{"agent_system_core"},
				RequiresPairing: true,
			}},
		},
		{
			name: "zalo_primary_without_accounts_array",
			in: []PluginInfo{{
				Enabled: true,
				Manifest: PluginManifest{
					ID:           "channel-zalo",
					Name:         "Zalo Bot Platform Channel",
					Capabilities: []string{string(CapabilityChannel)},
					Channels:     []PluginChannelDef{{Name: "zalo", DisplayName: "Zalo Bot Platform", RequiresPairing: true}},
					Config: map[string]any{
						"zalo_bot_token": "secret-must-not-leak",
						"default_agent":  "agent_support",
					},
				},
			}},
			want: []channels.ChannelAccount{{
				ID:              "channel-zalo",
				Name:            "Zalo Bot Platform",
				Channel:         "zalo",
				Enabled:         true,
				BoundAgentIDs:   []string{"agent_support"},
				RequiresPairing: true,
			}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := AccountsFromPlugins(tt.in)
			if len(got) != len(tt.want) {
				t.Fatalf("len=%d want %d: %+v", len(got), len(tt.want), got)
			}
			for i := range tt.want {
				if got[i].ID != tt.want[i].ID || got[i].Name != tt.want[i].Name || got[i].Channel != tt.want[i].Channel {
					t.Errorf("account[%d]=%+v want %+v", i, got[i], tt.want[i])
				}
				if got[i].Enabled != tt.want[i].Enabled || got[i].RequiresPairing != tt.want[i].RequiresPairing {
					t.Errorf("account[%d] flags=%+v want %+v", i, got[i], tt.want[i])
				}
				if stringsJoin(got[i].BoundAgentIDs) != stringsJoin(tt.want[i].BoundAgentIDs) {
					t.Errorf("account[%d] agents=%v want %v", i, got[i].BoundAgentIDs, tt.want[i].BoundAgentIDs)
				}
				if got[i].Token != "" {
					t.Errorf("account[%d] leaked token", i)
				}
			}
		})
	}
}

func TestMergeChannelAccountsDropsUninstalledTelegram(t *testing.T) {
	t.Parallel()
	pluginAccs := []channels.ChannelAccount{{ID: "astro", Name: "Astro", Channel: "discord", Enabled: true}}
	disk := []channels.ChannelAccount{
		{ID: "tg_default", Name: "Primary Telegram Bot", Channel: "telegram", Enabled: true},
		{ID: "astro", Name: "Astro", Channel: "discord", RequiresPairing: true},
	}
	got := MergeChannelAccounts(pluginAccs, disk)
	if len(got) != 1 || got[0].ID != "astro" {
		t.Fatalf("got %+v", got)
	}
	if !got[0].RequiresPairing {
		t.Fatal("expected disk pairing flag to overlay plugin account")
	}
}

func TestMergeChannelAccountsKeepsDiskWhenNoPlugins(t *testing.T) {
	t.Parallel()
	disk := []channels.ChannelAccount{{ID: "zalo_bot", Name: "Zalo", Channel: "zalo", Enabled: true}}
	got := MergeChannelAccounts(nil, disk)
	if len(got) != 1 || got[0].ID != "zalo_bot" {
		t.Fatalf("got %+v", got)
	}
}

func stringsJoin(v []string) string {
	out := ""
	for i, s := range v {
		if i > 0 {
			out += ","
		}
		out += s
	}
	return out
}

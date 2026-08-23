package plugin

import (
	"encoding/json"
	"time"
)

// PluginCapability defines supported feature dimensions of a WASM plugin.
type PluginCapability string

const (
	CapabilityTool      PluginCapability = "tool"
	CapabilityChannel   PluginCapability = "channel"
	CapabilityConnector PluginCapability = "connector"
)

// PluginPermissions declares granular capabilities requested by the plugin.
type PluginPermissions struct {
	NetOutbound []string `json:"net_outbound,omitempty"` // Whitelist of allowed domain patterns (e.g. "api.telegram.org", "*.slack.com")
	Secrets     []string `json:"secrets,omitempty"`      // Whitelist of secret IDs this plugin is authorized to retrieve
	Storage     bool     `json:"storage,omitempty"`      // True if plugin is granted scoped persistent KV storage
	BusEvents   []string `json:"bus_events,omitempty"`   // Whitelist of event topics plugin can emit to internal/bus
}

// PluginToolDef describes an individual tool exported by the plugin.
type PluginToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
}

// PluginManifest represents the formal descriptor (manifest.json) of a plugin package.
type PluginManifest struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Author       string            `json:"author,omitempty"`
	Description  string            `json:"description,omitempty"`
	Capabilities []string          `json:"capabilities"`
	Permissions  PluginPermissions `json:"permissions"`
	Tools        []PluginToolDef   `json:"tools,omitempty"`
	Config       map[string]string `json:"config,omitempty"`
}

// PluginStatus represents runtime lifecycle state of an active plugin.
type PluginStatus string

const (
	StatusRunning  PluginStatus = "running"
	StatusStopped  PluginStatus = "stopped"
	StatusDisabled PluginStatus = "disabled"
	StatusError    PluginStatus = "error"
)

// PluginInfo is the API and UI projection of an installed plugin.
type PluginInfo struct {
	Manifest    PluginManifest `json:"manifest"`
	Enabled     bool           `json:"enabled"`
	Status      PluginStatus   `json:"status"`
	Error       string         `json:"error,omitempty"`
	LoadedAt    time.Time      `json:"loaded_at,omitempty"`
	Path        string         `json:"path,omitempty"`
	MemoryBytes uint64         `json:"memory_bytes,omitempty"`
}

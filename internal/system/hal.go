package system

import (
	"context"
	"time"
)

// SystemMetrics represents live hardware performance readings.
type SystemMetrics struct {
	CPU struct {
		Model        string  `json:"model"`
		Cores        int     `json:"cores"`
		UsagePercent float64 `json:"usage_percent"`
		TempCelsius  float64 `json:"temperature_celsius"`
	} `json:"cpu"`
	Memory struct {
		TotalMB  uint64  `json:"total_mb"`
		UsedMB   uint64  `json:"used_mb"`
		ActondMB float64 `json:"actond_mb"`
	} `json:"memory"`
	Disk struct {
		TotalGB   float64 `json:"total_gb"`
		UsedGB    float64 `json:"used_gb"`
		DataDirGB float64 `json:"data_dir_gb"`
	} `json:"disk"`
	Containers    []ContainerStatus `json:"containers"`
	RuntimeMode   string            `json:"runtime_mode"`
	CanvasURL     string            `json:"canvas_url,omitempty"`
	UptimeSeconds uint64            `json:"uptime_seconds"`
	Timestamp     time.Time         `json:"timestamp"`
}

// ContainerStatus describes a Docker container visible to the ActonOS host.
type ContainerStatus struct {
	ID            string  `json:"id"`
	Name          string  `json:"name"`
	Image         string  `json:"image"`
	State         string  `json:"state"`
	Status        string  `json:"status"`
	CPUPercent    float64 `json:"cpu_percent"`
	MemoryUsageMB float64 `json:"memory_usage_mb"`
	MemoryLimitMB float64 `json:"memory_limit_mb"`
}

// WifiNetwork describes a detected Wi-Fi Access Point.
type WifiNetwork struct {
	SSID     string `json:"ssid"`
	BSSID    string `json:"bssid,omitempty"`
	Signal   int    `json:"signal"`   // Signal strength in dBm or percentage
	Security string `json:"security"` // "WPA2", "WPA3", "Open"
}

// WifiStatus describes current wireless interface connection state.
type WifiStatus struct {
	Connected   bool   `json:"connected"`
	SSID        string `json:"ssid,omitempty"`
	IPAddress   string `json:"ip_address,omitempty"`
	Signal      int    `json:"signal,omitempty"`
	HotspotMode bool   `json:"hotspot_mode"`
}

// HAL (Hardware Abstraction Layer) isolates bare-metal vs container differences.
type HAL interface {
	// RuntimeMode returns "baremetal" or "docker".
	RuntimeMode() string

	// GetMetrics returns snapshot of system resources.
	GetMetrics(ctx context.Context) (*SystemMetrics, error)

	// ScanWifi scans for available wireless access points.
	ScanWifi(ctx context.Context) ([]WifiNetwork, error)

	// ConnectWifi connects to a Wi-Fi network.
	ConnectWifi(ctx context.Context, ssid, password string) error

	// GetWifiStatus returns the current Wi-Fi status.
	GetWifiStatus(ctx context.Context) (*WifiStatus, error)

	// RestartDaemon triggers a graceful daemon restart.
	RestartDaemon(ctx context.Context) error
}

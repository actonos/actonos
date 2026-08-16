package system

import (
	"context"
	"os"
	"runtime"
	"time"
)

// DockerHAL is the containerized and development fallback HAL.
type DockerHAL struct {
	startTime time.Time
	dataDir   string
}

// NewDockerHAL creates a new DockerHAL instance.
func NewDockerHAL(dataDir string) *DockerHAL {
	return &DockerHAL{
		startTime: time.Now(),
		dataDir:   dataDir,
	}
}

func (h *DockerHAL) RuntimeMode() string {
	return "docker"
}

func (h *DockerHAL) GetMetrics(ctx context.Context) (*SystemMetrics, error) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	metrics := &SystemMetrics{
		UptimeSeconds: uint64(time.Since(h.startTime).Seconds()),
		Timestamp:     time.Now().UTC(),
	}

	metrics.CPU.Model = runtime.GOARCH + " (" + runtime.GOOS + ")"
	metrics.CPU.Cores = runtime.NumCPU()
	metrics.CPU.UsagePercent = 2.5
	metrics.CPU.TempCelsius = 42.0

	metrics.Memory.TotalMB = 8192
	metrics.Memory.UsedMB = 1024
	metrics.Memory.ActondMB = float64(m.Alloc) / (1024 * 1024)

	metrics.Disk.TotalGB = 64.0
	metrics.Disk.UsedGB = 12.0
	metrics.Disk.DataDirGB = 0.8

	return metrics, nil
}

func (h *DockerHAL) ScanWifi(ctx context.Context) ([]WifiNetwork, error) {
	// Wi-Fi scanning is not available in Docker container mode; return simulated networks in dev
	return []WifiNetwork{
		{SSID: "ActonOS-Dev-WiFi", Signal: 90, Security: "WPA2"},
		{SSID: "Office-Network-5G", Signal: 75, Security: "WPA3"},
	}, nil
}

func (h *DockerHAL) ConnectWifi(ctx context.Context, ssid, password string) error {
	// Handled via host networking in container
	return nil
}

func (h *DockerHAL) GetWifiStatus(ctx context.Context) (*WifiStatus, error) {
	return &WifiStatus{
		Connected:   true,
		SSID:        "Container-Bridge",
		IPAddress:   "172.17.0.2",
		Signal:      100,
		HotspotMode: false,
	}, nil
}

func (h *DockerHAL) RestartDaemon(ctx context.Context) error {
	os.Exit(0) // Process manager (Docker/systemd) will restart container
	return nil
}

// AutoDetectHAL inspects the environment and returns the appropriate HAL implementation.
func AutoDetectHAL(dataDir string) HAL {
	modeEnv := os.Getenv("RUNTIME_MODE")
	if modeEnv == "docker" {
		return NewDockerHAL(dataDir)
	}

	// Check if running inside Docker container
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return NewDockerHAL(dataDir)
	}

	// If on Linux and not forced to Docker, attempt baremetal
	if runtime.GOOS == "linux" && modeEnv != "docker" {
		return getLinuxBaremetalHAL(dataDir)
	}

	return NewDockerHAL(dataDir)
}

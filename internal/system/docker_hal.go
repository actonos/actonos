package system

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
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
	metrics := collectLiveHostMetrics(h.dataDir, h.startTime)
	metrics.Containers = dockerContainerStatuses(ctx)
	return metrics, nil
}

func dockerContainerStatuses(ctx context.Context) []ContainerStatus {
	if _, err := exec.LookPath("docker"); err != nil {
		return []ContainerStatus{}
	}
	listCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	output, err := exec.CommandContext(
		listCtx,
		"docker", "ps", "-a", "--no-trunc",
		"--format", `{"id":"{{.ID}}","name":"{{.Names}}","image":"{{.Image}}","state":"{{.State}}","status":"{{.Status}}"}`,
	).Output()
	if err != nil {
		return []ContainerStatus{}
	}
	items := make([]ContainerStatus, 0)
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var item ContainerStatus
		if json.Unmarshal([]byte(line), &item) == nil {
			items = append(items, item)
		}
	}
	if len(items) == 0 {
		return items
	}
	statsCtx, statsCancel := context.WithTimeout(ctx, 3*time.Second)
	defer statsCancel()
	stats, err := exec.CommandContext(
		statsCtx,
		"docker", "stats", "--no-stream",
		"--format", `{"name":"{{.Name}}","cpu":"{{.CPUPerc}}","mem":"{{.MemUsage}}"}`,
	).Output()
	if err != nil {
		return items
	}
	byName := make(map[string]int, len(items))
	for index := range items {
		byName[items[index].Name] = index
	}
	for _, line := range strings.Split(strings.TrimSpace(string(stats)), "\n") {
		var raw struct {
			Name string `json:"name"`
			CPU  string `json:"cpu"`
			Mem  string `json:"mem"`
		}
		if json.Unmarshal([]byte(line), &raw) != nil {
			continue
		}
		index, ok := byName[raw.Name]
		if !ok {
			continue
		}
		items[index].CPUPercent = parseDockerPercent(raw.CPU)
		parts := strings.Split(raw.Mem, "/")
		items[index].MemoryUsageMB = parseDockerMemory(parts[0])
		if len(parts) > 1 {
			items[index].MemoryLimitMB = parseDockerMemory(parts[1])
		}
	}
	return items
}

func parseDockerPercent(value string) float64 {
	result, _ := strconv.ParseFloat(strings.TrimSpace(strings.TrimSuffix(value, "%")), 64)
	return result
}

func parseDockerMemory(value string) float64 {
	value = strings.TrimSpace(value)
	multiplier := 1.0
	switch {
	case strings.HasSuffix(value, "GiB"):
		multiplier = 1024
		value = strings.TrimSuffix(value, "GiB")
	case strings.HasSuffix(value, "MiB"):
		value = strings.TrimSuffix(value, "MiB")
	case strings.HasSuffix(value, "KiB"):
		multiplier = 1.0 / 1024
		value = strings.TrimSuffix(value, "KiB")
	case strings.HasSuffix(value, "GB"):
		multiplier = 1000
		value = strings.TrimSuffix(value, "GB")
	case strings.HasSuffix(value, "MB"):
		value = strings.TrimSuffix(value, "MB")
	}
	result, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return result * multiplier
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

func (h *DockerHAL) RestartEmbeddingd(ctx context.Context) error {
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

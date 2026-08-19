//go:build linux

package system

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/godbus/dbus/v5"
)

// BaremetalHAL implements HAL for Linux bare-metal MiniPCs.
type BaremetalHAL struct {
	startTime time.Time
	dataDir   string
}

// NewBaremetalHAL creates a new BaremetalHAL instance.
func NewBaremetalHAL(dataDir string) *BaremetalHAL {
	return &BaremetalHAL{
		startTime: time.Now(),
		dataDir:   dataDir,
	}
}

func getLinuxBaremetalHAL(dataDir string) HAL {
	return NewBaremetalHAL(dataDir)
}

func (h *BaremetalHAL) RuntimeMode() string {
	return "baremetal"
}

func (h *BaremetalHAL) GetMetrics(ctx context.Context) (*SystemMetrics, error) {
	metrics := collectLiveHostMetrics(h.dataDir, h.startTime)
	metrics.RuntimeMode = "baremetal"
	metrics.Containers = dockerContainerStatuses(ctx)
	return metrics, nil
}

func (h *BaremetalHAL) ScanWifi(ctx context.Context) ([]WifiNetwork, error) {
	conn, err := dbus.SystemBus()
	if err != nil {
		return nil, fmt.Errorf("connecting to system dbus: %w", err)
	}

	obj := conn.Object("org.freedesktop.NetworkManager", "/org/freedesktop/NetworkManager")
	var devices []dbus.ObjectPath
	err = obj.CallWithContext(ctx, "org.freedesktop.NetworkManager.GetDevices", 0).Store(&devices)
	if err != nil {
		return nil, fmt.Errorf("getting network devices via dbus: %w", err)
	}

	var networks []WifiNetwork
	for _, devPath := range devices {
		devObj := conn.Object("org.freedesktop.NetworkManager", devPath)
		devType, err := devObj.GetProperty("org.freedesktop.NetworkManager.Device.DeviceType")
		if err != nil {
			continue
		}

		// DeviceType 2 == NM_DEVICE_TYPE_WIFI
		if devType.Value().(uint32) == 2 {
			var apPaths []dbus.ObjectPath
			err := devObj.CallWithContext(ctx, "org.freedesktop.NetworkManager.Device.Wireless.GetAccessPoints", 0).Store(&apPaths)
			if err != nil {
				continue
			}

			for _, apPath := range apPaths {
				apObj := conn.Object("org.freedesktop.NetworkManager", apPath)
				ssidProp, _ := apObj.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Ssid")
				strengthProp, _ := apObj.GetProperty("org.freedesktop.NetworkManager.AccessPoint.Strength")

				var ssid string
				if ssidBytes, ok := ssidProp.Value().([]byte); ok {
					ssid = string(ssidBytes)
				}

				if ssid != "" {
					strength := 50
					if s, ok := strengthProp.Value().(uint8); ok {
						strength = int(s)
					}
					networks = append(networks, WifiNetwork{
						SSID:     ssid,
						Signal:   strength,
						Security: "WPA2",
					})
				}
			}
		}
	}

	return networks, nil
}

func (h *BaremetalHAL) ConnectWifi(ctx context.Context, ssid, password string) error {
	// Connect via nmcli or dbus
	cmd := exec.CommandContext(ctx, "nmcli", "dev", "wifi", "connect", ssid, "password", password)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("nmcli wifi connect: %w (%s)", err, string(output))
	}
	return nil
}

func (h *BaremetalHAL) GetWifiStatus(ctx context.Context) (*WifiStatus, error) {
	return &WifiStatus{
		Connected:   true,
		SSID:        "HomeNetwork",
		IPAddress:   "192.168.1.100",
		Signal:      85,
		HotspotMode: false,
	}, nil
}

func (h *BaremetalHAL) RestartDaemon(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "systemctl", "restart", "actond")
	return cmd.Run()
}

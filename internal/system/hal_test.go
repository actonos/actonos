package system

import (
	"context"
	"os"
	"testing"
)

func TestCanvasURLFromEnvironment(t *testing.T) {
	original, existed := os.LookupEnv("ACTONOS_CANVAS_URL")
	t.Cleanup(func() {
		if existed {
			_ = os.Setenv("ACTONOS_CANVAS_URL", original)
		} else {
			_ = os.Unsetenv("ACTONOS_CANVAS_URL")
		}
	})

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "relative", raw: "/canvas/session", want: "/canvas/session"},
		{name: "https", raw: "https://canvas.example.com/live", want: "https://canvas.example.com/live"},
		{name: "localhost http", raw: "http://localhost:6080/vnc.html", want: "http://localhost:6080/vnc.html"},
		{name: "ipv4 loopback http", raw: "http://127.0.0.1:6080/vnc.html", want: "http://127.0.0.1:6080/vnc.html"},
		{name: "ipv6 loopback http", raw: "http://[::1]:6080/vnc.html", want: "http://[::1]:6080/vnc.html"},
		{name: "remote http rejected", raw: "http://canvas.example.com/live", want: ""},
		{name: "javascript rejected", raw: "javascript:alert(1)", want: ""},
		{name: "protocol relative rejected", raw: "//canvas.example.com/live", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := os.Setenv("ACTONOS_CANVAS_URL", tt.raw); err != nil {
				t.Fatalf("setting environment: %v", err)
			}
			if got := canvasURLFromEnvironment(); got != tt.want {
				t.Fatalf("canvasURLFromEnvironment() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDockerHAL_GetMetrics(t *testing.T) {
	hal := NewDockerHAL(t.TempDir())

	if hal.RuntimeMode() != "docker" {
		t.Fatalf("expected runtime mode 'docker', got '%s'", hal.RuntimeMode())
	}

	ctx := context.Background()
	metrics, err := hal.GetMetrics(ctx)
	if err != nil {
		t.Fatalf("GetMetrics failed: %v", err)
	}

	if metrics.CPU.Cores <= 0 {
		t.Fatalf("expected positive CPU cores, got %d", metrics.CPU.Cores)
	}

	if metrics.Memory.ActondMB <= 0 {
		t.Fatalf("expected non-zero actond memory usage")
	}
}

func TestParseDockerTelemetry(t *testing.T) {
	if got := parseDockerPercent("12.5%"); got != 12.5 {
		t.Fatalf("parseDockerPercent() = %v, want 12.5", got)
	}
	if got := parseDockerMemory("1.5GiB"); got != 1536 {
		t.Fatalf("parseDockerMemory() = %v, want 1536", got)
	}
	if got := parseDockerMemory("512KiB"); got != 0.5 {
		t.Fatalf("parseDockerMemory() = %v, want 0.5", got)
	}
}

func TestDockerHAL_WifiScan(t *testing.T) {
	hal := NewDockerHAL(t.TempDir())
	ctx := context.Background()

	networks, err := hal.ScanWifi(ctx)
	if err != nil {
		t.Fatalf("ScanWifi failed: %v", err)
	}

	if len(networks) == 0 {
		t.Fatalf("expected simulated dev networks")
	}

	status, err := hal.GetWifiStatus(ctx)
	if err != nil {
		t.Fatalf("GetWifiStatus failed: %v", err)
	}

	if !status.Connected {
		t.Fatalf("expected connected status")
	}
}

func TestTailscaleManager_StatusDisabled(t *testing.T) {
	t.Setenv("DISABLE_TAILSCALE", "true")
	mgr := NewTailscaleManager(t.TempDir(), "test-node", "")

	ctx := context.Background()
	status, err := mgr.GetStatus(ctx)
	if err != nil {
		t.Fatalf("GetStatus failed: %v", err)
	}

	if status.Connected {
		t.Fatalf("expected disabled tailscale not to be connected")
	}
}

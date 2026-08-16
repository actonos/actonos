package system

import (
	"context"
	"testing"
)

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

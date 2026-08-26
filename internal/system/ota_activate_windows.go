//go:build windows

package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"golang.org/x/sys/windows"
)

func (o *OTAEngine) activateActond(src string) error {
	return copyReplace(src, filepath.Join(o.binDir, "actond.exe"))
}

func (o *OTAEngine) activateEmbeddingd(src string) error {
	return copyReplace(src, filepath.Join(o.binDir, "embeddingd.exe"))
}

// SpawnOTAChild starts the versioned actond.exe which waits for this PID to exit.
func SpawnOTAChild(dataDir, version string) error {
	exe := filepath.Join(dataDir, "releases", version, "actond.exe")
	if _, err := os.Stat(exe); err != nil {
		exe = filepath.Join(dataDir, "bin", "actond.exe")
	}
	cmd := exec.Command(exe, os.Args[1:]...)
	cmd.Env = append(os.Environ(),
		"ACTONOS_OTA_CHILD=1",
		fmt.Sprintf("ACTONOS_OTA_PARENT_PID=%d", os.Getpid()),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Start()
}

// WaitForOTAParent blocks until ACTONOS_OTA_PARENT_PID exits (or 30s).
func WaitForOTAParent() {
	if os.Getenv("ACTONOS_OTA_CHILD") != "1" {
		return
	}
	pid, _ := strconv.Atoi(os.Getenv("ACTONOS_OTA_PARENT_PID"))
	if pid <= 0 {
		return
	}
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(handle)
	_, _ = windows.WaitForSingleObject(handle, uint32((30 * time.Second).Milliseconds()))
}

package system

import (
	"os"
	"sync"
)

const (
	// freezeFreeBytes is the absolute floor below which heavy writes must stop.
	freezeFreeBytes = 500 * 1024 * 1024
	// freezeFreeRatio is the relative floor (5% of total).
	freezeFreeRatio = 0.05
)

var (
	diskMu          sync.RWMutex
	freeSpaceLookup = nativeFreeSpace
	forcedFreeze    *bool
)

// SetFreeSpaceLookup overrides disk inspection (tests only).
func SetFreeSpaceLookup(fn func(path string) (free, total uint64, err error)) {
	diskMu.Lock()
	defer diskMu.Unlock()
	if fn == nil {
		freeSpaceLookup = nativeFreeSpace
		return
	}
	freeSpaceLookup = fn
}

// ForceWriteFreeze overrides freeze detection (tests only). Pass nil to restore.
func ForceWriteFreeze(freeze *bool) {
	diskMu.Lock()
	defer diskMu.Unlock()
	forcedFreeze = freeze
}

// WritesFrozen reports whether new heavy writes (embeddings, OTA, backups, audit
// rotation targets) should be refused because the data volume is exhausted.
func WritesFrozen(dataDir string) bool {
	diskMu.RLock()
	forced := forcedFreeze
	lookup := freeSpaceLookup
	diskMu.RUnlock()
	if forced != nil {
		return *forced
	}
	if dataDir == "" {
		dataDir = "."
	}
	free, total, err := lookup(dataDir)
	if err != nil {
		return false
	}
	if free < freezeFreeBytes {
		return true
	}
	if total > 0 && float64(free)/float64(total) < freezeFreeRatio {
		return true
	}
	return false
}

// DiskUsage returns free/total bytes for dataDir.
func DiskUsage(dataDir string) (free, total uint64, err error) {
	diskMu.RLock()
	lookup := freeSpaceLookup
	diskMu.RUnlock()
	if dataDir == "" {
		dataDir = "."
	}
	return lookup(dataDir)
}

func nativeFreeSpace(path string) (free, total uint64, err error) {
	info, err := os.Stat(path)
	if err != nil {
		if mkErr := os.MkdirAll(path, 0755); mkErr != nil {
			return 0, 0, err
		}
		info, err = os.Stat(path)
		if err != nil {
			return 0, 0, err
		}
	}
	_ = info
	return platformFreeSpace(path)
}

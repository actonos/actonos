package system

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	ErrChecksumMismatch = errors.New("ota update failed: sha256 checksum mismatch")
)

// UpdateRelease represents an available update release.
type UpdateRelease struct {
	Version     string `json:"version"`
	DownloadURL string `json:"download_url"`
	ChecksumSHA string `json:"checksum_sha256"`
	ReleaseDate string `json:"release_date"`
}

// OTAEngine handles atomic binary updates and self-healing watchdog rollbacks.
type OTAEngine struct {
	mu           sync.RWMutex
	dataDir      string
	releasesDir  string
	binSymlink   string
	activeBuild  string
	previousBuild string
	httpClient   *http.Client
}

// NewOTAEngine creates a new OTAEngine.
func NewOTAEngine(dataDir string) *OTAEngine {
	releasesDir := filepath.Join(dataDir, "releases")
	binDir := filepath.Join(dataDir, "bin")
	_ = os.MkdirAll(releasesDir, 0755)
	_ = os.MkdirAll(binDir, 0755)

	binSymlink := filepath.Join(binDir, "actond")

	return &OTAEngine{
		dataDir:     dataDir,
		releasesDir: releasesDir,
		binSymlink:  binSymlink,
		httpClient:  &http.Client{Timeout: 300 * time.Second},
	}
}

// ApplyUpdate downloads, verifies, and performs atomic symlink swap.
func (o *OTAEngine) ApplyUpdate(ctx context.Context, release UpdateRelease) error {
	o.mu.Lock()
	defer o.mu.Unlock()

	targetDir := filepath.Join(o.releasesDir, release.Version)
	_ = os.MkdirAll(targetDir, 0755)
	targetBinary := filepath.Join(targetDir, "actond")

	// 1. Download new binary
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, release.DownloadURL, nil)
	if err != nil {
		return err
	}

	resp, err := o.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("downloading update binary: %w", err)
	}
	defer resp.Body.Close()

	out, err := os.OpenFile(targetBinary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}

	hasher := sha256.New()
	multiWriter := io.MultiWriter(out, hasher)

	if _, err := io.Copy(multiWriter, resp.Body); err != nil {
		_ = out.Close()
		return fmt.Errorf("saving update binary: %w", err)
	}
	_ = out.Close()

	// 2. Verify SHA256 Checksum if provided
	if release.ChecksumSHA != "" {
		calculatedSHA := hex.EncodeToString(hasher.Sum(nil))
		if calculatedSHA != release.ChecksumSHA {
			_ = os.Remove(targetBinary)
			return fmt.Errorf("%w (expected %s, got %s)", ErrChecksumMismatch, release.ChecksumSHA, calculatedSHA)
		}
	}

	// 3. Save previous build path for rollback
	if currentTarget, err := os.Readlink(o.binSymlink); err == nil {
		o.previousBuild = currentTarget
	}

	// 4. Atomic Symlink Swap
	tempSymlink := o.binSymlink + ".tmp"
	_ = os.Remove(tempSymlink)
	if err := os.Symlink(targetBinary, tempSymlink); err != nil {
		return fmt.Errorf("creating temp symlink: %w", err)
	}

	if err := os.Rename(tempSymlink, o.binSymlink); err != nil {
		return fmt.Errorf("atomic symlink swap failed: %w", err)
	}

	o.activeBuild = targetBinary
	slog.Info("atomic ota symlink swapped", "version", release.Version, "path", targetBinary)

	return nil
}

// Rollback restores the previous stable build if health check fails.
func (o *OTAEngine) Rollback() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	if o.previousBuild == "" {
		return errors.New("no previous build available for rollback")
	}

	tempSymlink := o.binSymlink + ".tmp"
	_ = os.Remove(tempSymlink)
	if err := os.Symlink(o.previousBuild, tempSymlink); err != nil {
		return err
	}
	if err := os.Rename(tempSymlink, o.binSymlink); err != nil {
		return err
	}

	slog.Warn("watchdog auto-rollback performed", "restored_build", o.previousBuild)
	return nil
}

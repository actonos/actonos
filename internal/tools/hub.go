package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
)

const (
	DefaultRegistryURL     = "https://raw.githubusercontent.com/actonos/actonos-skills/refs/heads/master/registry.json"
	DefaultSkillRawBaseURL = "https://raw.githubusercontent.com/actonos/actonos-skills/refs/heads/master/skills"
)

// HubSkill represents a ready-to-install skill available in the Acton/Claw Community Hub.
type HubSkill struct {
	ID              string   `json:"id"`
	Slug            string   `json:"slug,omitempty"`
	Name            string   `json:"name"`
	Description     string   `json:"description"`
	Category        string   `json:"category"`
	Author          string   `json:"author"`
	Version         string   `json:"version,omitempty"`
	Icon            string   `json:"icon,omitempty"`
	Tags            []string `json:"tags,omitempty"`
	Stars           int      `json:"stars,omitempty"`
	UpdatedAt       any      `json:"updatedAt,omitempty"`
	ContentLanguage string   `json:"contentLanguage,omitempty"`
	Path            string   `json:"path,omitempty"`
	Files           []string `json:"files,omitempty"`
	IsMultiFile     bool     `json:"isMultiFile,omitempty"`
	SkillURL        string   `json:"skillUrl,omitempty"`
	SourceGithubURL string   `json:"sourceGithubUrl,omitempty"`
	Installed       bool     `json:"installed"`
	SkillMD         string   `json:"skill_md,omitempty"`
	Entrypoint      string   `json:"entrypoint,omitempty"`
	ScriptContent   string   `json:"script_content,omitempty"`
}

type remoteRegistryResponse struct {
	Version     string     `json:"version"`
	UpdatedAt   any        `json:"updatedAt"`
	TotalSkills int        `json:"totalSkills"`
	Skills      []HubSkill `json:"skills"`
}

// HubManager provides access to community skills and 1-click installation.
type HubManager struct {
	mu          sync.RWMutex
	skillsDir   string
	catalog     []HubSkill
	registryURL string
	rawBaseURL  string
	lastFetch   time.Time
	httpClient  *http.Client
	eventBus    *bus.EventBus
}

// NewHubManager creates a HubManager and initiates remote catalog fetch.
func NewHubManager(skillsDir string) *HubManager {
	hm := &HubManager{
		skillsDir:   skillsDir,
		catalog:     nil,
		registryURL: DefaultRegistryURL,
		rawBaseURL:  DefaultSkillRawBaseURL,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}

	// Fetch remote catalog in background on startup
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := hm.FetchRemoteCatalog(ctx); err != nil {
			slog.Debug("initial remote skills registry fetch deferred", "error", err)
		}
	}()

	return hm
}

// NewHubManagerWithRegistry creates a HubManager with custom registry URLs without background fetching.
func NewHubManagerWithRegistry(skillsDir, registryURL, rawBaseURL string) *HubManager {
	return &HubManager{
		skillsDir:   skillsDir,
		catalog:     nil,
		registryURL: registryURL,
		rawBaseURL:  rawBaseURL,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
	}
}

// SetEventBus sets the event bus for broadcasting installation progress events.
func (hm *HubManager) SetEventBus(eventBus *bus.EventBus) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.eventBus = eventBus
}

func (hm *HubManager) publishProgress(skillID, step, message string, progress int, extra map[string]any) {
	eb := hm.eventBus
	if eb == nil {
		return
	}

	payload := map[string]any{
		"skill_id": skillID,
		"step":     step,
		"message":  message,
		"progress": progress,
	}
	for k, v := range extra {
		payload[k] = v
	}

	eb.Publish(bus.NewEvent(bus.EventSkillProgress, skillID, payload))
}

// SetRegistryURL overrides the registry URL (useful for testing or private mirrors).
func (hm *HubManager) SetRegistryURL(registryURL, rawBaseURL string) {
	hm.mu.Lock()
	defer hm.mu.Unlock()
	hm.registryURL = registryURL
	hm.rawBaseURL = rawBaseURL
}

// FetchRemoteCatalog fetches the live registry.json from GitHub.
func (hm *HubManager) FetchRemoteCatalog(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hm.registryURL, nil)
	if err != nil {
		return fmt.Errorf("creating registry request: %w", err)
	}

	resp, err := hm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry response status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading registry response: %w", err)
	}

	var reg remoteRegistryResponse
	if err := json.Unmarshal(body, &reg); err != nil {
		return fmt.Errorf("parsing registry json: %w", err)
	}

	if len(reg.Skills) == 0 {
		return errors.New("empty skills list in remote registry")
	}

	hm.mu.Lock()
	hm.catalog = reg.Skills
	hm.lastFetch = time.Now()
	hm.mu.Unlock()

	slog.Info("skills registry updated from remote", "count", len(reg.Skills))
	return nil
}

// ListCatalog returns available skills marked with current installation status.
func (hm *HubManager) ListCatalog() []HubSkill {
	hm.mu.RLock()
	needRefresh := time.Since(hm.lastFetch) > 1*time.Hour || len(hm.catalog) == 0
	hm.mu.RUnlock()

	if needRefresh {
		// Trigger background refresh if stale or empty
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			_ = hm.FetchRemoteCatalog(ctx)
		}()
	}

	hm.mu.RLock()
	defer hm.mu.RUnlock()

	if len(hm.catalog) == 0 {
		return []HubSkill{}
	}

	result := make([]HubSkill, len(hm.catalog))
	copy(result, hm.catalog)

	for i := range result {
		slug := result[i].Slug
		if slug == "" {
			slug = result[i].ID
		}

		skillDir := filepath.Join(hm.skillsDir, slug)
		skillDirByID := filepath.Join(hm.skillsDir, result[i].ID)

		if _, err := os.Stat(skillDir); err == nil {
			result[i].Installed = true
		} else if _, err := os.Stat(skillDirByID); err == nil {
			result[i].Installed = true
		} else {
			result[i].Installed = false
		}
	}

	return result
}

// InstallSkill writes the skill files into the skills directory for hot-reloading.
func (hm *HubManager) InstallSkill(skillID string) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var target *HubSkill
	for _, s := range hm.catalog {
		if s.ID == skillID || (s.Slug != "" && s.Slug == skillID) || s.Name == skillID {
			target = &s
			break
		}
	}

	if target == nil {
		return fmt.Errorf("skill '%s' not found in hub catalog", skillID)
	}

	slug := target.Slug
	if slug == "" {
		slug = target.ID
	}

	hm.publishProgress(skillID, "resolving", fmt.Sprintf("Preparing installation for %s", target.Name), 10, map[string]any{"slug": slug, "name": target.Name})

	destDir := filepath.Join(hm.skillsDir, slug)
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("creating skill directory: %w", err)
	}

	// Multi-file or remote file download
	filesToDownload := target.Files
	if len(filesToDownload) == 0 {
		filesToDownload = []string{"SKILL.md"}
	}

	totalFiles := len(filesToDownload)
	hm.publishProgress(skillID, "downloading", fmt.Sprintf("Downloading %d package file(s)", totalFiles), 20, map[string]any{"slug": slug, "total_files": totalFiles})

	for i, relFile := range filesToDownload {
		relFile = strings.TrimPrefix(relFile, "/")
		fileURL := fmt.Sprintf("%s/%s/%s", hm.rawBaseURL, slug, relFile)

		progressPct := 20 + int(float64(i+1)/float64(totalFiles)*60)
		hm.publishProgress(skillID, "downloading", fmt.Sprintf("Downloading %s (%d/%d)", relFile, i+1, totalFiles), progressPct, map[string]any{
			"slug":         slug,
			"current_file": relFile,
			"file_index":   i + 1,
			"total_files":  totalFiles,
		})

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
		if err != nil {
			cancel()
			return fmt.Errorf("preparing download for %s: %w", relFile, err)
		}
		req.Close = true

		resp, err := hm.httpClient.Do(req)
		if err != nil {
			cancel()
			return fmt.Errorf("downloading %s: %w", relFile, err)
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			cancel()
			return fmt.Errorf("failed to download %s (status %d)", relFile, resp.StatusCode)
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		cancel()
		if err != nil {
			return fmt.Errorf("reading file %s: %w", relFile, err)
		}

		destFilePath := filepath.Join(destDir, relFile)
		if err := os.MkdirAll(filepath.Dir(destFilePath), 0755); err != nil {
			return fmt.Errorf("creating parent directory for %s: %w", relFile, err)
		}

		mode := os.FileMode(0644)
		if strings.HasSuffix(relFile, ".sh") || strings.HasSuffix(relFile, ".py") || strings.HasSuffix(relFile, ".js") {
			mode = 0755
		}

		if err := os.WriteFile(destFilePath, data, mode); err != nil {
			return fmt.Errorf("writing file %s: %w", relFile, err)
		}
	}

	hm.publishProgress(skillID, "verifying", "Verifying prerequisites & registering skill", 90, map[string]any{"slug": slug})

	// Emit completion event on EventBus
	if hm.eventBus != nil {
		hm.eventBus.Publish(bus.NewEvent(bus.EventSkillInstalled, skillID, map[string]any{
			"skill_id": skillID,
			"slug":     slug,
			"name":     target.Name,
		}))
	}

	hm.publishProgress(skillID, "completed", fmt.Sprintf("Skill %s successfully installed", target.Name), 100, map[string]any{"slug": slug})

	return nil
}

// UninstallSkill removes the skill directory.
func (hm *HubManager) UninstallSkill(skillID string) error {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	var targetSlug string
	for _, s := range hm.catalog {
		if s.ID == skillID || s.Slug == skillID {
			targetSlug = s.Slug
			break
		}
	}
	if targetSlug == "" {
		targetSlug = skillID
	}

	hm.publishProgress(skillID, "removing", fmt.Sprintf("Removing skill package '%s'", targetSlug), 40, map[string]any{"slug": targetSlug})

	destDir := filepath.Join(hm.skillsDir, targetSlug)
	if _, err := os.Stat(destDir); err == nil {
		_ = os.RemoveAll(destDir)
	}

	destDirID := filepath.Join(hm.skillsDir, skillID)
	if _, err := os.Stat(destDirID); err == nil {
		_ = os.RemoveAll(destDirID)
	}

	if hm.eventBus != nil {
		hm.eventBus.Publish(bus.NewEvent(bus.EventSkillUninstalled, skillID, map[string]any{
			"skill_id": skillID,
			"slug":     targetSlug,
		}))
	}

	hm.publishProgress(skillID, "completed", fmt.Sprintf("Skill '%s' uninstalled", targetSlug), 100, map[string]any{"slug": targetSlug})

	return nil
}




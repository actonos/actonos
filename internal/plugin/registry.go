package plugin

import (
	"archive/zip"
	"bytes"
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
	// DefaultPluginRegistryURL is the official GitHub Releases endpoint for the plugin catalog.
	DefaultPluginRegistryURL = "https://github.com/actonos/plugin-sdk/releases/latest/download/plugin-registry.json"
	// DefaultPluginDownloadBaseURL is the base URL for downloading individual .actonpkg release assets.
	DefaultPluginDownloadBaseURL = "https://github.com/actonos/plugin-sdk/releases/latest/download"
)

// RegistryPlugin represents an official or community plugin listed in the remote ActonOS registry.
type RegistryPlugin struct {
	ID               string               `json:"id"`
	Name             string               `json:"name"`
	Version          string               `json:"version"`
	Author           string               `json:"author,omitempty"`
	Description      string               `json:"description,omitempty"`
	License          string               `json:"license,omitempty"`
	Category         string               `json:"category,omitempty"`
	Filename         string               `json:"filename,omitempty"`
	Icon             string               `json:"icon,omitempty"`
	Tags             []string             `json:"tags,omitempty"`
	Stars            int                  `json:"stars,omitempty"`
	DownloadURL      string               `json:"download_url,omitempty"`
	URL              string               `json:"url,omitempty"`
	SHA256           string               `json:"sha256,omitempty"`
	SizeBytes        int64                `json:"size_bytes,omitempty"`
	Size             int64                `json:"size,omitempty"`
	Capabilities     []string             `json:"capabilities,omitempty"`
	Permissions      *PluginPermissions   `json:"permissions,omitempty"`
	Tools            []PluginToolDef      `json:"tools,omitempty"`
	Channels         []PluginChannelDef   `json:"channels,omitempty"`
	Connectors       []PluginConnectorDef `json:"connectors,omitempty"`
	ConfigSchema     json.RawMessage      `json:"config_schema,omitempty"`
	Installed        bool                 `json:"installed"`
	InstalledStatus  PluginStatus         `json:"installed_status,omitempty"`
	InstalledVersion string               `json:"installed_version,omitempty"`
}

type remotePluginRegistryResponse struct {
	SchemaVersion   string           `json:"schema_version,omitempty"`
	GeneratedAt     string           `json:"generated_at,omitempty"`
	SdkVersion      string           `json:"sdk_version,omitempty"`
	TotalPlugins    int              `json:"total_plugins,omitempty"`
	DownloadBaseURL string           `json:"download_base_url,omitempty"`
	Version         string           `json:"version,omitempty"`
	Plugins         []RegistryPlugin `json:"plugins"`
}

// PluginRegistryManager handles remote catalog fetching, caching, and 1-click installation of WASM plugins.
type PluginRegistryManager struct {
	mu              sync.RWMutex
	pluginsDir      string
	catalog         []RegistryPlugin
	registryURL     string
	downloadBaseURL string
	lastFetch       time.Time
	httpClient      *http.Client
	eventBus        *bus.EventBus
	pluginMgr       *Manager
}

// NewPluginRegistryManager creates a new PluginRegistryManager.
func NewPluginRegistryManager(pluginsDir string, pluginMgr *Manager, eventBus *bus.EventBus) *PluginRegistryManager {
	regURL := os.Getenv("ACTONOS_PLUGIN_REGISTRY_URL")
	if regURL == "" {
		regURL = DefaultPluginRegistryURL
	}
	dlURL := os.Getenv("ACTONOS_PLUGIN_DOWNLOAD_BASE_URL")
	if dlURL == "" {
		dlURL = DefaultPluginDownloadBaseURL
	}

	prm := &PluginRegistryManager{
		pluginsDir:      pluginsDir,
		catalog:         nil,
		registryURL:     regURL,
		downloadBaseURL: dlURL,
		httpClient: &http.Client{
			Timeout: 20 * time.Second,
		},
		eventBus:  eventBus,
		pluginMgr: pluginMgr,
	}

	// Asynchronously fetch remote catalog on startup
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if err := prm.FetchRemoteCatalog(ctx); err != nil {
			slog.Debug("initial remote plugins registry fetch deferred", "error", err)
		}
	}()

	return prm
}

// NewPluginRegistryManagerWithURLs creates a PluginRegistryManager with custom URLs (ideal for testing).
func NewPluginRegistryManagerWithURLs(pluginsDir string, pluginMgr *Manager, eventBus *bus.EventBus, registryURL, downloadBaseURL string) *PluginRegistryManager {
	return &PluginRegistryManager{
		pluginsDir:      pluginsDir,
		catalog:         nil,
		registryURL:     registryURL,
		downloadBaseURL: downloadBaseURL,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
		eventBus:  eventBus,
		pluginMgr: pluginMgr,
	}
}

// SetEventBus sets the active event bus.
func (prm *PluginRegistryManager) SetEventBus(eb *bus.EventBus) {
	prm.mu.Lock()
	defer prm.mu.Unlock()
	prm.eventBus = eb
}

// SetPluginManager sets the active plugin manager.
func (prm *PluginRegistryManager) SetPluginManager(mgr *Manager) {
	prm.mu.Lock()
	defer prm.mu.Unlock()
	prm.pluginMgr = mgr
}

// SetRegistryURLs overrides the registry URL and download base URL.
func (prm *PluginRegistryManager) SetRegistryURLs(registryURL, downloadBaseURL string) {
	prm.mu.Lock()
	defer prm.mu.Unlock()
	prm.registryURL = registryURL
	prm.downloadBaseURL = downloadBaseURL
}

func (prm *PluginRegistryManager) publishProgress(pluginID, step, message string, progress int, extra map[string]any) {
	prm.mu.RLock()
	eb := prm.eventBus
	prm.mu.RUnlock()

	if eb == nil {
		return
	}

	payload := map[string]any{
		"plugin_id": pluginID,
		"step":      step,
		"message":   message,
		"progress":  progress,
	}
	for k, v := range extra {
		payload[k] = v
	}

	eb.Publish(bus.NewEvent(bus.EventPluginProgress, pluginID, payload))
}

// FetchRemoteCatalog fetches and unmarshals the remote plugin catalog.
func (prm *PluginRegistryManager) FetchRemoteCatalog(ctx context.Context) error {
	prm.mu.RLock()
	url := prm.registryURL
	prm.mu.RUnlock()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("creating registry request: %w", err)
	}
	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("User-Agent", "ActonOS-Daemon/1.0")

	resp, err := prm.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching plugin registry from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("registry server returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return fmt.Errorf("reading registry response: %w", err)
	}

	var plugins []RegistryPlugin
	var baseURL string

	// Try unmarshaling as object wrapper { "plugins": [...] }
	var wrapped remotePluginRegistryResponse
	if err := json.Unmarshal(body, &wrapped); err == nil && len(wrapped.Plugins) > 0 {
		plugins = wrapped.Plugins
		baseURL = wrapped.DownloadBaseURL
	} else {
		// Try unmarshaling as direct array [...]
		if err := json.Unmarshal(body, &plugins); err != nil {
			return fmt.Errorf("parsing plugin registry json: %w", err)
		}
	}

	if len(plugins) == 0 {
		return errors.New("empty plugins list in remote registry")
	}

	prm.mu.RLock()
	fallbackBase := prm.downloadBaseURL
	prm.mu.RUnlock()

	if baseURL == "" {
		baseURL = fallbackBase
	}

	for i := range plugins {
		p := &plugins[i]
		if p.SizeBytes > 0 && p.Size == 0 {
			p.Size = p.SizeBytes
		} else if p.Size > 0 && p.SizeBytes == 0 {
			p.SizeBytes = p.Size
		}
		if p.DownloadURL == "" && p.Filename != "" {
			p.DownloadURL = fmt.Sprintf("%s/%s", strings.TrimRight(baseURL, "/"), p.Filename)
		}
		if p.Category == "" {
			if containsStr(p.Capabilities, "channel") {
				p.Category = "channel"
			} else if containsStr(p.Capabilities, "connector") {
				p.Category = "connector"
			} else {
				p.Category = "tool"
			}
		}
		if len(p.Tags) == 0 {
			p.Tags = generatePluginTags(p.ID, p.Capabilities)
		}
		if p.Stars == 0 {
			p.Stars = getDefaultPluginStars(p.ID)
		}
		if p.Permissions == nil {
			p.Permissions = getDefaultPluginPermissions(p.ID)
		}
	}

	prm.mu.Lock()
	prm.catalog = plugins
	prm.lastFetch = time.Now()
	prm.mu.Unlock()

	slog.Info("plugin registry updated from remote", "count", len(plugins), "url", url)
	return nil
}

// ListCatalog returns the list of available plugins with current installation status populated.
func (prm *PluginRegistryManager) ListCatalog(ctx context.Context, installed []PluginInfo) []RegistryPlugin {
	prm.mu.RLock()
	needRefresh := time.Since(prm.lastFetch) > 1*time.Hour || len(prm.catalog) == 0
	prm.mu.RUnlock()

	if needRefresh {
		go func() {
			refreshCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			if err := prm.FetchRemoteCatalog(refreshCtx); err != nil {
				slog.Debug("background plugin registry refresh error", "error", err)
			}
		}()
	}

	prm.mu.RLock()
	rawCatalog := prm.catalog
	prm.mu.RUnlock()

	var catalogList []RegistryPlugin
	if len(rawCatalog) > 0 {
		catalogList = make([]RegistryPlugin, len(rawCatalog))
		copy(catalogList, rawCatalog)
	} else {
		catalogList = []RegistryPlugin{}
	}

	installedMap := make(map[string]PluginInfo)
	for _, inst := range installed {
		installedMap[strings.ToLower(inst.Manifest.ID)] = inst
	}

	for i := range catalogList {
		normID := strings.ToLower(catalogList[i].ID)
		if inst, found := installedMap[normID]; found {
			catalogList[i].Installed = true
			catalogList[i].InstalledStatus = inst.Status
			catalogList[i].InstalledVersion = inst.Manifest.Version
		} else if prm.pluginsDir != "" {
			// Check disk directly in case plugin directory exists
			dirPath := filepath.Join(prm.pluginsDir, catalogList[i].ID)
			if _, err := os.Stat(dirPath); err == nil {
				catalogList[i].Installed = true
				catalogList[i].InstalledStatus = StatusRunning
			} else {
				catalogList[i].Installed = false
			}
		} else {
			catalogList[i].Installed = false
		}
	}

	return catalogList
}

// InstallPlugin downloads, unpacks, verifies, and hot-reloads a plugin from the registry.
func (prm *PluginRegistryManager) InstallPlugin(ctx context.Context, pluginID, downloadURL string) (*PluginInfo, error) {
	cleanID := strings.TrimSpace(pluginID)
	if cleanID == "" || strings.Contains(cleanID, "..") || strings.ContainsAny(cleanID, `/\`) {
		return nil, fmt.Errorf("invalid plugin ID: '%s'", pluginID)
	}

	prm.mu.RLock()
	var target *RegistryPlugin
	for _, p := range prm.catalog {
		if strings.EqualFold(p.ID, cleanID) ||
			strings.EqualFold(strings.TrimPrefix(p.ID, "channel-"), cleanID) ||
			strings.EqualFold(strings.TrimPrefix(p.ID, "connector-"), cleanID) {
			target = &p
			break
		}
	}
	baseDownloadURL := prm.downloadBaseURL
	prm.mu.RUnlock()

	resolvedURL := downloadURL
	displayName := cleanID
	if target != nil {
		displayName = target.Name
		if resolvedURL == "" {
			if target.DownloadURL != "" {
				resolvedURL = target.DownloadURL
			} else if target.Filename != "" {
				resolvedURL = fmt.Sprintf("%s/%s", strings.TrimRight(baseDownloadURL, "/"), target.Filename)
			} else if target.URL != "" {
				resolvedURL = target.URL
			}
		}
	}

	if resolvedURL == "" {
		// Construct standard release asset URL format
		resolvedURL = fmt.Sprintf("%s/%s.actonpkg", strings.TrimRight(baseDownloadURL, "/"), cleanID)
	}

	prm.publishProgress(cleanID, "resolving", fmt.Sprintf("Preparing installation for %s", displayName), 10, map[string]any{
		"plugin_id":    cleanID,
		"name":         displayName,
		"download_url": resolvedURL,
	})

	prm.publishProgress(cleanID, "downloading", fmt.Sprintf("Downloading plugin package from %s", resolvedURL), 25, map[string]any{
		"plugin_id":    cleanID,
		"download_url": resolvedURL,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resolvedURL, nil)
	if err != nil {
		return nil, fmt.Errorf("preparing download request: %w", err)
	}
	req.Header.Set("User-Agent", "ActonOS-Daemon/1.0")

	resp, err := prm.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("downloading plugin package from %s: %w", resolvedURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("plugin download server returned status %d from %s", resp.StatusCode, resolvedURL)
	}

	pkgBytes, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20)) // 64 MB maximum package cap
	if err != nil {
		return nil, fmt.Errorf("reading plugin package: %w", err)
	}

	prm.publishProgress(cleanID, "extracting", "Extracting package contents and validating bytecode", 65, map[string]any{
		"plugin_id": cleanID,
		"size":      len(pkgBytes),
	})

	var (
		manifestBytes []byte
		wasmBytes     []byte
		sigBytes      []byte
		readmeBytes   []byte
	)

	isZip := len(pkgBytes) >= 4 && string(pkgBytes[:4]) == "PK\x03\x04"
	if isZip {
		manifestBytes, wasmBytes, sigBytes, readmeBytes, err = ExtractPluginPackage(pkgBytes)
		if err != nil {
			return nil, fmt.Errorf("unpacking .actonpkg bundle: %w", err)
		}
	} else {
		// Standalone .wasm binary fallback
		if len(pkgBytes) < 8 || string(pkgBytes[:4]) != "\x00asm" {
			return nil, errors.New("downloaded payload is neither a valid .actonpkg zip nor WebAssembly bytecode")
		}
		wasmBytes = pkgBytes

		// Construct synthetic manifest if standalone .wasm was provided
		var synthCaps []string
		if target != nil && len(target.Capabilities) > 0 {
			synthCaps = append(synthCaps, target.Capabilities...)
		} else {
			synthCaps = []string{string(CapabilityTool)}
		}

		synthManifest := PluginManifest{
			ID:           cleanID,
			Name:         displayName,
			Version:      "1.0.0",
			Author:       "ActonOS Registry",
			Capabilities: synthCaps,
		}
		if target != nil && target.Permissions != nil {
			synthManifest.Permissions = *target.Permissions
		}
		mJSON, _ := json.MarshalIndent(synthManifest, "", "  ")
		manifestBytes = mJSON
	}

	// Parse manifest to determine canonical destination folder
	var parsedManifest PluginManifest
	if err := json.Unmarshal(manifestBytes, &parsedManifest); err != nil {
		return nil, fmt.Errorf("parsing manifest.json: %w", err)
	}
	if parsedManifest.ID != "" {
		cleanID = parsedManifest.ID
	}

	if err := VerifyRemotePluginPackage(wasmBytes, sigBytes); err != nil {
		return nil, fmt.Errorf("verifying plugin signature: %w", err)
	}

	targetDir := filepath.Join(prm.pluginsDir, cleanID)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return nil, fmt.Errorf("creating plugin directory %s: %w", targetDir, err)
	}

	// Write files into target directory
	if err := os.WriteFile(filepath.Join(targetDir, "manifest.json"), manifestBytes, 0644); err != nil {
		return nil, fmt.Errorf("writing manifest.json: %w", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "plugin.wasm"), wasmBytes, 0644); err != nil {
		return nil, fmt.Errorf("writing plugin.wasm: %w", err)
	}
	if len(sigBytes) > 0 {
		_ = os.WriteFile(filepath.Join(targetDir, "signature.sig"), sigBytes, 0644)
	}
	if len(readmeBytes) > 0 {
		_ = os.WriteFile(filepath.Join(targetDir, "README.md"), readmeBytes, 0644)
	}

	prm.publishProgress(cleanID, "activating", "Hot-reloading plugin into Wazero runtime", 85, map[string]any{
		"plugin_id": cleanID,
		"name":      parsedManifest.Name,
	})

	// Hot-reload into Wazero runtime
	if prm.pluginMgr != nil {
		if err := prm.pluginMgr.ScanAndLoadAll(ctx); err != nil {
			slog.Warn("failed to scan plugins after installation", "error", err)
		}
	}

	// Check final plugin status
	var installedPlugin *PluginInfo
	if prm.pluginMgr != nil {
		if pInfo, found := prm.pluginMgr.GetPlugin(cleanID); found {
			installedPlugin = &pInfo
		}
	}

	if installedPlugin == nil {
		installedPlugin = &PluginInfo{
			Manifest: parsedManifest,
			Enabled:  true,
			Status:   StatusRunning,
			Path:     targetDir,
			LoadedAt: time.Now(),
		}
	}

	prm.publishProgress(cleanID, "completed", fmt.Sprintf("Successfully installed %s", parsedManifest.Name), 100, map[string]any{
		"plugin_id": cleanID,
		"name":      parsedManifest.Name,
		"version":   parsedManifest.Version,
		"status":    string(installedPlugin.Status),
	})

	// Emit EventPluginInstalled
	prm.mu.RLock()
	eb := prm.eventBus
	prm.mu.RUnlock()
	if eb != nil {
		eb.Publish(bus.NewEvent(bus.EventPluginInstalled, cleanID, map[string]any{
			"plugin_id": cleanID,
			"name":      parsedManifest.Name,
			"version":   parsedManifest.Version,
		}))
	}

	slog.Info("plugin installed from official registry", "id", cleanID, "name", parsedManifest.Name, "version", parsedManifest.Version)
	return installedPlugin, nil
}

// ExtractPluginPackage extracts the contents of a .actonpkg zip package into individual files.
func ExtractPluginPackage(pkgData []byte) ([]byte, []byte, []byte, []byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(pkgData), int64(len(pkgData)))
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("reading zip archive: %w", err)
	}

	var (
		manifestBytes []byte
		wasmBytes     []byte
		sigBytes      []byte
		readmeBytes   []byte
	)

	for _, file := range reader.File {
		cleanName := filepath.Base(file.Name)
		if file.FileInfo().IsDir() {
			continue
		}

		rc, err := file.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(rc, 64<<20))
		rc.Close()
		if err != nil {
			continue
		}

		switch cleanName {
		case "manifest.json":
			manifestBytes = data
		case "plugin.wasm":
			wasmBytes = data
		case "signature.sig":
			sigBytes = data
		case "README.md":
			readmeBytes = data
		}
	}

	if len(manifestBytes) == 0 {
		return nil, nil, nil, nil, errors.New("package is missing manifest.json")
	}
	if len(wasmBytes) == 0 {
		return nil, nil, nil, nil, errors.New("package is missing plugin.wasm")
	}

	return manifestBytes, wasmBytes, sigBytes, readmeBytes, nil
}

func containsStr(slice []string, val string) bool {
	for _, s := range slice {
		if strings.EqualFold(s, val) {
			return true
		}
	}
	return false
}

func generatePluginTags(id string, caps []string) []string {
	tags := []string{}
	clean := strings.ToLower(id)
	clean = strings.TrimPrefix(clean, "channel-")
	clean = strings.TrimPrefix(clean, "connector-")
	clean = strings.TrimPrefix(clean, "tool-")
	tags = append(tags, clean)

	for _, c := range caps {
		tags = append(tags, strings.ToLower(c))
	}
	return tags
}

func getDefaultPluginStars(id string) int {
	starsMap := map[string]int{
		"channel-discord":            342,
		"channel-slack":              289,
		"channel-telegram":           415,
		"channel-whatsapp":           260,
		"channel-zalo":               380,
		"connector-figma":            210,
		"connector-github":           540,
		"connector-google-workspace": 470,
		"connector-jira":             195,
		"connector-linear":           310,
		"connector-notion":           430,
		"connector-slack":            275,
	}
	if s, ok := starsMap[id]; ok {
		return s
	}
	return 150
}

func getDefaultPluginPermissions(id string) *PluginPermissions {
	permMap := map[string]*PluginPermissions{
		"channel-discord": {
			NetOutbound: []string{"discord.com", "gateway.discord.gg"},
			Secrets:     []string{"DISCORD_BOT_TOKEN"},
			Storage:     true,
		},
		"channel-slack": {
			NetOutbound: []string{"slack.com", "wss-primary.slack.com", "wss-backup.slack.com"},
			Secrets:     []string{"SLACK_BOT_TOKEN", "SLACK_APP_TOKEN"},
			Storage:     true,
		},
		"channel-telegram": {
			NetOutbound: []string{"api.telegram.org"},
			Secrets:     []string{"TELEGRAM_BOT_TOKEN"},
			Storage:     true,
		},
		"channel-whatsapp": {
			NetOutbound: []string{"graph.facebook.com"},
			Secrets:     []string{"WHATSAPP_ACCESS_TOKEN", "WHATSAPP_PHONE_NUMBER_ID"},
			Storage:     true,
		},
		"channel-zalo": {
			NetOutbound: []string{"bot.zapps.me", "openapi.zalo.me"},
			Secrets:     []string{"ZALO_BOT_TOKEN"},
			Storage:     true,
		},
		"connector-figma": {
			NetOutbound: []string{"api.figma.com"},
			Secrets:     []string{"FIGMA_PERSONAL_ACCESS_TOKEN"},
			Storage:     true,
		},
		"connector-github": {
			NetOutbound: []string{"api.github.com"},
			Secrets:     []string{"GITHUB_TOKEN"},
			Storage:     true,
		},
		"connector-google-workspace": {
			NetOutbound: []string{"gmail.googleapis.com", "calendar.googleapis.com", "drive.googleapis.com", "docs.googleapis.com", "oauth2.googleapis.com"},
			Secrets:     []string{"GOOGLE_WORKSPACE_CLIENT_SECRET"},
			Storage:     true,
		},
		"connector-jira": {
			NetOutbound: []string{"atlassian.net", "api.atlassian.com"},
			Secrets:     []string{"JIRA_API_TOKEN", "JIRA_USER_EMAIL"},
			Storage:     true,
		},
		"connector-linear": {
			NetOutbound: []string{"api.linear.app"},
			Secrets:     []string{"LINEAR_API_KEY"},
			Storage:     true,
		},
		"connector-notion": {
			NetOutbound: []string{"api.notion.com"},
			Secrets:     []string{"NOTION_INTEGRATION_TOKEN"},
			Storage:     true,
		},
		"connector-slack": {
			NetOutbound: []string{"slack.com"},
			Secrets:     []string{"SLACK_USER_TOKEN"},
			Storage:     true,
		},
	}
	if p, ok := permMap[id]; ok {
		return p
	}
	return &PluginPermissions{Storage: true}
}

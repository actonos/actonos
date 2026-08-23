package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/tools"
)

var (
	ErrPluginNotFound      = errors.New("plugin not found")
	ErrPluginAlreadyLoaded = errors.New("plugin already loaded")
	ErrInvalidManifest     = errors.New("invalid plugin manifest")
)

type loadedPluginState struct {
	manifest        PluginManifest
	info            PluginInfo
	inst            *PluginInstance
	hostCtx         *HostContext
	toolBridges     []*WasmToolBridge
	channelBridge   *WasmChannelBridge
	connectorBridge *WasmConnectorBridge
	dir             string
}

// Manager orchestrates lifecycle, registration, and discovery of WASM plugins.
type Manager struct {
	mu          sync.RWMutex
	loader      *WasmLoader
	toolReg     *tools.ToolRegistry
	channelMgr  *channels.ChannelManager
	eventBus    *bus.EventBus
	kvStore     KVStore
	secrets     SecretProvider
	pluginsDir  string
	plugins     map[string]*loadedPluginState
	disabledIDs map[string]bool
}

// NewManager creates a new WASM Plugin Manager.
func NewManager(
	loader *WasmLoader,
	toolReg *tools.ToolRegistry,
	channelMgr *channels.ChannelManager,
	eventBus *bus.EventBus,
	kvStore KVStore,
	secrets SecretProvider,
	pluginsDir string,
) *Manager {
	return &Manager{
		loader:      loader,
		toolReg:     toolReg,
		channelMgr:  channelMgr,
		eventBus:    eventBus,
		kvStore:     kvStore,
		secrets:     secrets,
		pluginsDir:  pluginsDir,
		plugins:     make(map[string]*loadedPluginState),
		disabledIDs: make(map[string]bool),
	}
}

// ScanAndLoadAll scans the plugins directory and loads all valid plugins.
func (m *Manager) ScanAndLoadAll(ctx context.Context) error {
	if m.pluginsDir == "" {
		return nil
	}

	if err := os.MkdirAll(m.pluginsDir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(m.pluginsDir)
	if err != nil {
		return fmt.Errorf("reading plugins dir: %w", err)
	}

	for _, entry := range entries {
		pluginPath := filepath.Join(m.pluginsDir, entry.Name())
		if entry.IsDir() {
			// Subdirectory format: /data/plugins/<id>/manifest.json + plugin.wasm
			manifestPath := filepath.Join(pluginPath, "manifest.json")
			wasmPath := filepath.Join(pluginPath, "plugin.wasm")
			if fileExists(manifestPath) && fileExists(wasmPath) {
				if err := m.loadFromFolder(ctx, entry.Name(), pluginPath); err != nil {
					slog.Warn("failed to load plugin folder", "id", entry.Name(), "error", err)
				}
			}
		} else if strings.HasSuffix(entry.Name(), ".wasm") {
			// Single-file format: /data/plugins/<name>.wasm + <name>.json
			pluginID := strings.TrimSuffix(entry.Name(), ".wasm")
			metaPath := filepath.Join(m.pluginsDir, pluginID+".json")
			if err := m.loadSingleFile(ctx, pluginID, pluginPath, metaPath); err != nil {
				slog.Warn("failed to load single-file plugin", "file", entry.Name(), "error", err)
			}
		}
	}

	return nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (m *Manager) loadFromFolder(ctx context.Context, id, dir string) error {
	manifestBytes, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return fmt.Errorf("reading manifest: %w", err)
	}

	var manifest PluginManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidManifest, err)
	}
	if manifest.ID == "" {
		manifest.ID = id
	}

	wasmBytes, err := os.ReadFile(filepath.Join(dir, "plugin.wasm"))
	if err != nil {
		return fmt.Errorf("reading plugin.wasm: %w", err)
	}

	disabledMarker := filepath.Join(dir, ".disabled")
	if !fileExists(disabledMarker) {
		disabledMarker = filepath.Join(dir, id+".disabled")
	}
	if !fileExists(disabledMarker) {
		disabledMarker = filepath.Join(m.pluginsDir, id+".disabled")
	}
	enabled := !fileExists(disabledMarker)

	return m.activatePlugin(ctx, manifest, wasmBytes, dir, enabled)
}

func (m *Manager) loadSingleFile(ctx context.Context, id, wasmPath, metaPath string) error {
	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("reading wasm file: %w", err)
	}

	manifest := PluginManifest{
		ID:           id,
		Name:         id,
		Version:      "1.0.0",
		Capabilities: []string{string(CapabilityTool)},
		Tools: []PluginToolDef{
			{
				Name:        "wasm_" + id,
				Description: fmt.Sprintf("WASM plugin tool (%s)", id),
				Parameters:  json.RawMessage(`{"type": "object", "properties": {}}`),
			},
		},
	}

	if metaBytes, err := os.ReadFile(metaPath); err == nil {
		_ = json.Unmarshal(metaBytes, &manifest)
		if manifest.ID == "" {
			manifest.ID = id
		}
	}

	dir := filepath.Dir(wasmPath)
	disabledMarker := filepath.Join(dir, id+".disabled")
	enabled := !fileExists(disabledMarker)

	return m.activatePlugin(ctx, manifest, wasmBytes, dir, enabled)
}

func (m *Manager) activatePlugin(ctx context.Context, manifest PluginManifest, wasmBytes []byte, dir string, enabled bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	id := manifest.ID
	if old, exists := m.plugins[id]; exists {
		m.deactivatePluginLocked(ctx, old)
	}

	info := PluginInfo{
		Manifest: manifest,
		Enabled:  enabled,
		Status:   StatusStopped,
		Path:     dir,
		LoadedAt: time.Now(),
	}

	state := &loadedPluginState{
		manifest: manifest,
		info:     info,
		dir:      dir,
	}

	if !enabled {
		info.Status = StatusDisabled
		state.info = info
		m.plugins[id] = state
		return nil
	}

	// Compile module
	cm, err := m.loader.Compile(ctx, id, wasmBytes)
	if err != nil {
		info.Status = StatusError
		info.Error = fmt.Sprintf("compilation failed: %v", err)
		state.info = info
		m.plugins[id] = state
		return err
	}

	gate := NewSecurityGate(manifest)
	hostCtx := &HostContext{
		PluginID: id,
		Manifest: manifest,
		Gate:     gate,
		KV:       m.kvStore,
		Secrets:  m.secrets,
		EventBus: m.eventBus,
	}

	inst, err := m.loader.Instantiate(ctx, id, manifest, hostCtx)
	if err != nil {
		info.Status = StatusError
		info.Error = fmt.Sprintf("instantiation failed: %v", err)
		state.info = info
		m.plugins[id] = state
		return err
	}

	state.inst = inst
	state.hostCtx = hostCtx
	state.info.Status = StatusRunning

	// Register Tools
	if m.toolReg != nil {
		for _, toolDef := range manifest.Tools {
			bridge := NewWasmToolBridge(id, toolDef, inst)
			m.toolReg.RegisterOrReplace(bridge)
			state.toolBridges = append(state.toolBridges, bridge)
		}
	}

	// Register Channel Adapter if capability declared
	for _, capStr := range manifest.Capabilities {
		if PluginCapability(capStr) == CapabilityChannel && m.channelMgr != nil {
			bridge := NewWasmChannelBridge(id, manifest, inst, m.eventBus)
			if err := m.channelMgr.RegisterDynamicAdapter(bridge); err == nil {
				state.channelBridge = bridge
				_ = bridge.Start(ctx)
			} else {
				slog.Warn("failed to register wasm channel adapter", "plugin_id", id, "error", err)
			}
		}
		if PluginCapability(capStr) == CapabilityConnector {
			state.connectorBridge = NewWasmConnectorBridge(id, manifest, inst)
		}
	}

	_ = cm
	m.plugins[id] = state
	slog.Info("activated wasm plugin", "id", id, "capabilities", manifest.Capabilities, "tools_count", len(state.toolBridges))
	return nil
}

func (m *Manager) deactivatePluginLocked(ctx context.Context, state *loadedPluginState) {
	if m.toolReg != nil {
		for _, tb := range state.toolBridges {
			m.toolReg.Unregister(tb.Name())
		}
	}

	if state.channelBridge != nil && m.channelMgr != nil {
		_ = state.channelBridge.Stop()
		_ = m.channelMgr.UnregisterDynamicAdapter(state.channelBridge.Name())
	}

	if state.inst != nil {
		_ = state.inst.Close(ctx)
	}

	m.loader.Uncompile(ctx, state.manifest.ID)
}

// EnablePlugin activates and registers a disabled plugin.
func (m *Manager) EnablePlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	state, exists := m.plugins[id]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}
	dir := state.dir
	manifest := state.manifest
	m.mu.Unlock()

	_ = os.Remove(filepath.Join(dir, ".disabled"))
	_ = os.Remove(filepath.Join(dir, id+".disabled"))
	_ = os.Remove(filepath.Join(m.pluginsDir, id+".disabled"))
	_ = os.Remove(filepath.Join(m.pluginsDir, id, ".disabled"))

	wasmPath := filepath.Join(dir, "plugin.wasm")
	if !fileExists(wasmPath) {
		wasmPath = filepath.Join(dir, id+".wasm")
	}

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("reading plugin wasm: %w", err)
	}

	return m.activatePlugin(ctx, manifest, wasmBytes, dir, true)
}

// DisablePlugin unregisters and suspends an active plugin.
func (m *Manager) DisablePlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.plugins[id]
	if !exists {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}

	m.deactivatePluginLocked(ctx, state)
	state.info.Status = StatusDisabled
	state.info.Enabled = false

	if state.dir != "" {
		_ = os.WriteFile(filepath.Join(state.dir, ".disabled"), []byte("disabled"), 0644)
		_ = os.WriteFile(filepath.Join(state.dir, id+".disabled"), []byte("disabled"), 0644)
	}
	_ = os.WriteFile(filepath.Join(m.pluginsDir, id+".disabled"), []byte("disabled"), 0644)
	return nil
}

// UninstallPlugin removes a plugin completely from memory and disk.
func (m *Manager) UninstallPlugin(ctx context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	state, exists := m.plugins[id]
	if !exists {
		return fmt.Errorf("%w: %s", ErrPluginNotFound, id)
	}

	m.deactivatePluginLocked(ctx, state)
	delete(m.plugins, id)

	if state.dir != "" {
		_ = os.RemoveAll(state.dir)
	}
	_ = os.RemoveAll(filepath.Join(m.pluginsDir, id))
	_ = os.Remove(filepath.Join(m.pluginsDir, id+".wasm"))
	_ = os.Remove(filepath.Join(m.pluginsDir, id+".json"))
	_ = os.Remove(filepath.Join(m.pluginsDir, id+".disabled"))
	_ = os.Remove(filepath.Join(m.pluginsDir, id+".actonpkg"))

	slog.Info("uninstalled wasm plugin", "id", id)
	return nil
}

// ListPlugins returns a summary list of all installed plugins.
func (m *Manager) ListPlugins() []PluginInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	res := make([]PluginInfo, 0, len(m.plugins))
	for _, p := range m.plugins {
		res = append(res, p.info)
	}
	return res
}

// GetPlugin returns the info for a single plugin.
func (m *Manager) GetPlugin(id string) (PluginInfo, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.plugins[id]
	if !exists {
		return PluginInfo{}, false
	}
	return p.info, true
}

// GetPluginLogs returns buffered runtime execution logs for a plugin.
func (m *Manager) GetPluginLogs(id string) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	p, exists := m.plugins[id]
	if !exists || p.hostCtx == nil {
		return []string{}
	}
	return p.hostCtx.GetLogs()
}

// Close gracefully closes all active plugin instances and runtime.
func (m *Manager) Close(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.plugins {
		m.deactivatePluginLocked(ctx, p)
	}
	return m.loader.Close(ctx)
}

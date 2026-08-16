package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var (
	ErrWASMExecution = errors.New("wasm plugin execution failed")
)

// WASMTool represents an executable WebAssembly plugin tool.
type WASMTool struct {
	toolName    string
	description string
	schema      json.RawMessage
	wasmBytes   []byte
	runtime     wazero.Runtime
}

// NewWASMTool creates a WASMTool instance from raw WebAssembly bytecode.
func NewWASMTool(ctx context.Context, name, description string, schema json.RawMessage, wasmBytes []byte) (*WASMTool, error) {
	r := wazero.NewRuntime(ctx)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	return &WASMTool{
		toolName:    "wasm_" + name,
		description: description,
		schema:      schema,
		wasmBytes:   wasmBytes,
		runtime:     r,
	}, nil
}

func (t *WASMTool) Name() string                     { return t.toolName }
func (t *WASMTool) Description() string              { return t.description }
func (t *WASMTool) Category() string                 { return "wasm" }
func (t *WASMTool) ParametersSchema() json.RawMessage { return t.schema }

func (t *WASMTool) Execute(ctx context.Context, inputJSON json.RawMessage) (*ToolResult, error) {
	stdinBuf := bytes.NewReader(inputJSON)
	var stdoutBuf, stderrBuf bytes.Buffer

	modConfig := wazero.NewModuleConfig().
		WithStdin(stdinBuf).
		WithStdout(&stdoutBuf).
		WithStderr(&stderrBuf).
		WithName("")

	mod, err := t.runtime.InstantiateWithConfig(ctx, t.wasmBytes, modConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %v (stderr: %s)", ErrWASMExecution, err, stderrBuf.String())
	}
	defer mod.Close(ctx)

	outputStr := strings.TrimSpace(stdoutBuf.String())
	if outputStr == "" && stderrBuf.Len() > 0 {
		return nil, fmt.Errorf("%w: %s", ErrWASMExecution, stderrBuf.String())
	}

	return &ToolResult{
		Content: outputStr,
	}, nil
}

func (t *WASMTool) Close(ctx context.Context) error {
	return t.runtime.Close(ctx)
}

// WASMPluginManager manages loading, scanning, and execution of WebAssembly plugins.
type WASMPluginManager struct {
	mu       sync.RWMutex
	registry *ToolRegistry
	plugins  map[string]*WASMTool
	dir      string
}

// NewWASMPluginManager creates a WASMPluginManager instance.
func NewWASMPluginManager(registry *ToolRegistry, pluginsDir string) *WASMPluginManager {
	return &WASMPluginManager{
		registry: registry,
		plugins:  make(map[string]*WASMTool),
		dir:      pluginsDir,
	}
}

// ScanAndRegisterPlugins scans the plugin directory for .wasm files and registers them.
func (m *WASMPluginManager) ScanAndRegisterPlugins(ctx context.Context) error {
	if m.dir == "" {
		return nil
	}

	if err := os.MkdirAll(m.dir, 0755); err != nil {
		return err
	}

	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".wasm") {
			pluginPath := filepath.Join(m.dir, entry.Name())
			pluginName := strings.TrimSuffix(entry.Name(), ".wasm")
			if err := m.LoadPlugin(ctx, pluginName, pluginPath); err != nil {
				slog.Warn("failed to load wasm plugin", "file", entry.Name(), "error", err)
			}
		}
	}

	return nil
}

// LoadPlugin reads and compiles a .wasm file into a registered tool.
func (m *WASMPluginManager) LoadPlugin(ctx context.Context, name, wasmPath string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	wasmBytes, err := os.ReadFile(wasmPath)
	if err != nil {
		return fmt.Errorf("reading wasm file: %w", err)
	}

	metaPath := strings.TrimSuffix(wasmPath, ".wasm") + ".json"
	description := fmt.Sprintf("Sandboxed WebAssembly plugin (%s)", name)
	schema := json.RawMessage(`{"type": "object", "properties": {}}`)

	if metaBytes, err := os.ReadFile(metaPath); err == nil {
		var meta struct {
			Description string          `json:"description"`
			Schema      json.RawMessage `json:"schema"`
		}
		if err := json.Unmarshal(metaBytes, &meta); err == nil {
			if meta.Description != "" {
				description = meta.Description
			}
			if len(meta.Schema) > 0 {
				schema = meta.Schema
			}
		}
	}

	wasmTool, err := NewWASMTool(ctx, name, description, schema, wasmBytes)
	if err != nil {
		return fmt.Errorf("instantiating wasm runtime: %w", err)
	}

	if oldTool, exists := m.plugins[name]; exists {
		_ = oldTool.Close(ctx)
		m.registry.Unregister(oldTool.Name())
	}

	m.plugins[name] = wasmTool
	if err := m.registry.Register(wasmTool); err != nil {
		return err
	}

	slog.Info("loaded wasm plugin tool", "name", wasmTool.Name(), "path", wasmPath)
	return nil
}

// UnloadPlugin unregisters and frees resources for a WASM plugin.
func (m *WASMPluginManager) UnloadPlugin(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	tool, exists := m.plugins[name]
	if !exists {
		return fmt.Errorf("wasm plugin %s not loaded", name)
	}

	_ = tool.Close(ctx)
	m.registry.Unregister(tool.Name())
	delete(m.plugins, name)

	return nil
}

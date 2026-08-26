package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
)

var (
	ErrPluginInitFailed    = errors.New("wasm plugin initialization failed")
	ErrFunctionNotFound    = errors.New("wasm export function not found")
	ErrExecutionTimeout    = errors.New("wasm execution exceeded timeout")
	ErrMemoryAllocation    = errors.New("wasm memory allocation failed")
	ErrModuleInstantiation = errors.New("wasm module instantiation failed")
)

const pluginToolTimeout = 300 * time.Second

// WasmLoader manages compilation and sandboxed execution of WASM plugins using Wazero.
type WasmLoader struct {
	runtime wazero.Runtime
	mu      sync.RWMutex
	cache   map[string]wazero.CompiledModule
}

// NewWasmLoader creates and initializes a WasmLoader instance.
func NewWasmLoader(ctx context.Context) (*WasmLoader, error) {
	config := wazero.NewRuntimeConfig().
		WithMemoryLimitPages(4096) // 4096 pages = 256 MB max memory cap

	r := wazero.NewRuntimeWithConfig(ctx, config)
	wasi_snapshot_preview1.MustInstantiate(ctx, r)

	if err := RegisterHostModule(ctx, r); err != nil {
		_ = r.Close(ctx)
		return nil, fmt.Errorf("registering host module: %w", err)
	}

	return &WasmLoader{
		runtime: r,
		cache:   make(map[string]wazero.CompiledModule),
	}, nil
}

// Compile compiles and caches a WASM bytecode module.
func (l *WasmLoader) Compile(ctx context.Context, pluginID string, wasmBytes []byte) (wazero.CompiledModule, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if cm, exists := l.cache[pluginID]; exists {
		return cm, nil
	}

	cm, err := l.runtime.CompileModule(ctx, wasmBytes)
	if err != nil {
		return nil, fmt.Errorf("compiling wasm module %s: %w", pluginID, err)
	}

	l.cache[pluginID] = cm
	return cm, nil
}

// Uncompile frees the cached compiled module.
func (l *WasmLoader) Uncompile(ctx context.Context, pluginID string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if cm, exists := l.cache[pluginID]; exists {
		_ = cm.Close(ctx)
		delete(l.cache, pluginID)
	}
}

// Close terminates the Wazero runtime and frees all compiled modules.
func (l *WasmLoader) Close(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	for id, cm := range l.cache {
		_ = cm.Close(ctx)
		delete(l.cache, id)
	}
	return l.runtime.Close(ctx)
}

// PluginInstance represents an active, isolated runtime instance of a WASM plugin.
type PluginInstance struct {
	pluginID string
	manifest PluginManifest
	mod      api.Module
	hostCtx  *HostContext
	mu       sync.Mutex
}

// Instantiate creates an isolated PluginInstance from a compiled module.
func (l *WasmLoader) Instantiate(ctx context.Context, pluginID string, manifest PluginManifest, hostCtx *HostContext) (*PluginInstance, error) {
	cm, err := l.Compile(ctx, pluginID, nil)
	if err != nil && cm == nil {
		return nil, fmt.Errorf("compiled module for %s not found: %w", pluginID, err)
	}

	modConfig := wazero.NewModuleConfig().
		WithName(""). // anonymous instance to allow concurrent instances
		WithStdin(bytes.NewReader(nil)).
		WithStdout(&pluginStdioWriter{host: hostCtx, level: "INFO"}).
		WithStderr(&pluginStdioWriter{host: hostCtx, level: "ERROR"})

	mod, err := l.runtime.InstantiateModule(ctx, cm, modConfig)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrModuleInstantiation, err)
	}

	allocFn := mod.ExportedFunction("acton_alloc")
	freeFn := mod.ExportedFunction("acton_free")

	hostCtx.AllocFn = allocFn
	hostCtx.FreeFn = freeFn
	hostCtx.Memory = mod.Memory()

	inst := &PluginInstance{
		pluginID: pluginID,
		manifest: manifest,
		mod:      mod,
		hostCtx:  hostCtx,
	}

	// 1. Initialize WASI reactor if _initialize is exported (required by TinyGo and C reactors)
	if initWasi := mod.ExportedFunction("_initialize"); initWasi != nil {
		if _, err := initWasi.Call(ctx); err != nil {
			_ = mod.Close(ctx)
			return nil, fmt.Errorf("wasi _initialize failed: %w", err)
		}
	}

	// 2. Initialize plugin if it exports acton_plugin_init
	if initFn := mod.ExportedFunction("acton_plugin_init"); initFn != nil {
		execCtx := WithHostContext(ctx, hostCtx)
		var res []uint64
		var initErr error

		cfgBytes, _ := json.Marshal(manifest.Config)
		hostCtx.mu.Lock()
		hostCtx.LastResponse = cfgBytes
		hostCtx.mu.Unlock()

		if len(initFn.Definition().ParamTypes()) == 0 {
			res, initErr = initFn.Call(execCtx)
		} else {
			if len(cfgBytes) > 0 && allocFn != nil {
				ptrLen, err := writeBufferToGuest(execCtx, hostCtx, cfgBytes)
				if err == nil {
					ptr := uint32(ptrLen >> 32)
					length := uint32(ptrLen)
					res, initErr = initFn.Call(execCtx, uint64(ptr), uint64(length))
				} else {
					initErr = err
				}
			} else {
				res, initErr = initFn.Call(execCtx, 0, 0)
			}
		}

		if initErr != nil || (len(res) > 0 && res[0] != 0) {
			_ = mod.Close(ctx)
			return nil, fmt.Errorf("%w: code %v, err: %v", ErrPluginInitFailed, res, initErr)
		}
	}

	return inst, nil
}

// ExecuteTool calls acton_tool_execute on the plugin instance.
func (inst *PluginInstance) ExecuteTool(ctx context.Context, toolName string, argsJSON json.RawMessage) (string, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	execFn := inst.mod.ExportedFunction("acton_tool_execute")
	if execFn == nil {
		return "", fmt.Errorf("%w: acton_tool_execute", ErrFunctionNotFound)
	}

	execCtx, cancel := context.WithTimeout(ctx, pluginToolTimeout)
	defer cancel()

	execCtx = WithHostContext(execCtx, inst.hostCtx)

	namePtrLen, err := writeBufferToGuest(execCtx, inst.hostCtx, []byte(toolName))
	if err != nil {
		return "", fmt.Errorf("%w: allocating tool name: %v", ErrMemoryAllocation, err)
	}
	namePtr := uint32(namePtrLen >> 32)
	nameLen := uint32(namePtrLen)

	argsPtrLen, err := writeBufferToGuest(execCtx, inst.hostCtx, argsJSON)
	if err != nil {
		return "", fmt.Errorf("%w: allocating args json: %v", ErrMemoryAllocation, err)
	}
	argsPtr := uint32(argsPtrLen >> 32)
	argsLen := uint32(argsPtrLen)

	results, err := execFn.Call(execCtx, uint64(namePtr), uint64(nameLen), uint64(argsPtr), uint64(argsLen))
	if err != nil {
		return "", fmt.Errorf("tool execution failed: %w", err)
	}

	if len(results) == 0 || results[0] == 0 {
		return "", nil
	}

	packed := results[0]
	resPtr := uint32(packed >> 32)
	resLen := uint32(packed)

	resBytes, err := readBufferFromMemory(inst.mod.Memory(), resPtr, resLen)
	if err != nil {
		return "", fmt.Errorf("reading tool result: %w", err)
	}

	return string(resBytes), nil
}

// SendChannelMessage calls acton_channel_send or acton_channel_send_message on the plugin instance.
func (inst *PluginInstance) SendChannelMessage(ctx context.Context, msgJSON []byte) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	sendFn := inst.mod.ExportedFunction("acton_channel_send")
	if sendFn == nil {
		sendFn = inst.mod.ExportedFunction("acton_channel_send_message")
	}
	if sendFn == nil {
		return fmt.Errorf("%w: acton_channel_send or acton_channel_send_message", ErrFunctionNotFound)
	}

	execCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	execCtx = WithHostContext(execCtx, inst.hostCtx)

	msgPtrLen, err := writeBufferToGuest(execCtx, inst.hostCtx, msgJSON)
	if err != nil {
		return fmt.Errorf("%w: allocating message json: %v", ErrMemoryAllocation, err)
	}
	msgPtr := uint32(msgPtrLen >> 32)
	msgLen := uint32(msgPtrLen)

	res, err := sendFn.Call(execCtx, uint64(msgPtr), uint64(msgLen))
	if err != nil {
		return fmt.Errorf("channel send failed: %w", err)
	}

	if len(res) > 0 && res[0] != 0 {
		return fmt.Errorf("channel send error code: %d", res[0])
	}

	return nil
}

// PollChannel calls acton_channel_poll on the plugin instance and returns any inbound message payload bytes.
func (inst *PluginInstance) PollChannel(ctx context.Context) ([]byte, error) {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	pollFn := inst.mod.ExportedFunction("acton_channel_poll")
	if pollFn == nil {
		return nil, nil
	}

	execCtx, cancel := context.WithTimeout(ctx, pluginToolTimeout)
	defer cancel()

	execCtx = WithHostContext(execCtx, inst.hostCtx)

	res, err := pollFn.Call(execCtx)
	if err != nil {
		return nil, fmt.Errorf("channel poll failed: %w", err)
	}

	if len(res) == 0 || res[0] == 0 {
		return nil, nil
	}

	packed := res[0]
	resPtr := uint32(packed >> 32)
	resLen := uint32(packed)

	if resLen == 0 {
		return nil, nil
	}

	resBytes, err := readBufferFromMemory(inst.mod.Memory(), resPtr, resLen)
	if err != nil {
		return nil, fmt.Errorf("reading poll result: %w", err)
	}

	return resBytes, nil
}

// Close terminates and unloads the plugin instance.
func (inst *PluginInstance) Close(ctx context.Context) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.hostCtx != nil {
		inst.hostCtx.CloseAllWS()
	}

	if shutdownFn := inst.mod.ExportedFunction("acton_plugin_shutdown"); shutdownFn != nil {
		execCtx := WithHostContext(ctx, inst.hostCtx)
		_, _ = shutdownFn.Call(execCtx)
	}

	return inst.mod.Close(ctx)
}

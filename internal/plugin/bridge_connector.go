package plugin

import (
	"context"
	"encoding/json"
	"fmt"
)

// WasmConnectorBridge coordinates SaaS connector hooks and sync routines for a WASM plugin.
type WasmConnectorBridge struct {
	pluginID      string
	connectorName string
	manifest      PluginManifest
	inst          *PluginInstance
}

// NewWasmConnectorBridge creates a new WasmConnectorBridge.
func NewWasmConnectorBridge(pluginID string, manifest PluginManifest, inst *PluginInstance) *WasmConnectorBridge {
	name := pluginID
	if len(manifest.Connectors) > 0 && manifest.Connectors[0].Name != "" {
		name = manifest.Connectors[0].Name
	} else if n, ok := manifest.Config["connector_name"].(string); ok && n != "" {
		name = n
	}
	return &WasmConnectorBridge{
		pluginID:      pluginID,
		connectorName: name,
		manifest:      manifest,
		inst:          inst,
	}
}

func (c *WasmConnectorBridge) Name() string {
	return c.connectorName
}

// HandleWebhook passes an incoming webhook payload into the WASM connector.
func (c *WasmConnectorBridge) HandleWebhook(ctx context.Context, payload []byte) ([]byte, error) {
	if c.inst == nil {
		return nil, fmt.Errorf("plugin %s not loaded", c.pluginID)
	}

	execFn := c.inst.mod.ExportedFunction("acton_connector_handle_webhook")
	if execFn == nil {
		if c.inst.hostCtx != nil {
			c.inst.hostCtx.Record("DEBUG", "connector webhook export is not implemented")
		}
		return nil, nil
	}

	execCtx := WithHostContext(ctx, c.inst.hostCtx)
	ptrLen, err := writeBufferToGuest(execCtx, c.inst.hostCtx, payload)
	if err != nil {
		return nil, err
	}

	res, err := execFn.Call(execCtx, uint64(uint32(ptrLen>>32)), uint64(uint32(ptrLen)))
	if err != nil {
		return nil, fmt.Errorf("connector webhook handler failed: %w", err)
	}

	if len(res) == 0 || res[0] == 0 {
		return nil, nil
	}

	packed := res[0]
	resPtr := uint32(packed >> 32)
	resLen := uint32(packed)

	return readBufferFromMemory(c.inst.mod.Memory(), resPtr, resLen)
}

// GetAuthURLSchema returns OAuth or credential config schema for the connector.
func (c *WasmConnectorBridge) GetConfigSchema() json.RawMessage {
	if len(c.manifest.ConfigSchema) > 0 {
		return c.manifest.ConfigSchema
	}
	if schemaStr, ok := c.manifest.Config["auth_schema"].(string); ok {
		return json.RawMessage(schemaStr)
	}
	return json.RawMessage(`{"type": "object", "properties": {}}`)
}

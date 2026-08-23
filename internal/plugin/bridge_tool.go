package plugin

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/actonos/actonos/internal/tools"
)

// WasmToolBridge wraps a WASM plugin tool definition into a callable tools.Tool.
type WasmToolBridge struct {
	pluginID string
	toolDef  PluginToolDef
	inst     *PluginInstance
}

// NewWasmToolBridge constructs a WasmToolBridge instance.
func NewWasmToolBridge(pluginID string, toolDef PluginToolDef, inst *PluginInstance) *WasmToolBridge {
	return &WasmToolBridge{
		pluginID: pluginID,
		toolDef:  toolDef,
		inst:     inst,
	}
}

func (t *WasmToolBridge) Name() string {
	return t.toolDef.Name
}

func (t *WasmToolBridge) Description() string {
	return t.toolDef.Description
}

func (t *WasmToolBridge) Category() string {
	return "wasm"
}

func (t *WasmToolBridge) ParametersSchema() json.RawMessage {
	if len(t.toolDef.Parameters) == 0 {
		return json.RawMessage(`{"type": "object", "properties": {}}`)
	}
	return t.toolDef.Parameters
}

func (t *WasmToolBridge) Execute(ctx context.Context, inputJSON json.RawMessage) (*tools.ToolResult, error) {
	if t.inst == nil {
		return nil, fmt.Errorf("plugin instance %s is not active", t.pluginID)
	}

	result, err := t.inst.ExecuteTool(ctx, t.toolDef.Name, inputJSON)
	if err != nil {
		return &tools.ToolResult{
			Error: err.Error(),
		}, nil
	}

	return &tools.ToolResult{
		Content: result,
	}, nil
}

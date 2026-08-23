package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"

	"github.com/actonos/actonos/internal/channels"
)

// WasmChannelBridge adapts a WASM Plugin with CapabilityChannel to channels.ChannelAdapter.
type WasmChannelBridge struct {
	pluginID    string
	channelName string
	inst        *PluginInstance
	ctx         context.Context
	cancel      context.CancelFunc
	mu          sync.Mutex
}

// NewWasmChannelBridge creates a new WasmChannelBridge.
func NewWasmChannelBridge(pluginID string, manifest PluginManifest, inst *PluginInstance) *WasmChannelBridge {
	channelName := pluginID
	if name, ok := manifest.Config["channel_name"]; ok && name != "" {
		channelName = name
	}

	return &WasmChannelBridge{
		pluginID:    pluginID,
		channelName: channelName,
		inst:        inst,
	}
}

func (b *WasmChannelBridge) Name() string {
	return b.channelName
}

func (b *WasmChannelBridge) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ctx, b.cancel = context.WithCancel(ctx)
	slog.Info("started wasm channel adapter", "plugin_id", b.pluginID, "channel", b.channelName)
	return nil
}

func (b *WasmChannelBridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cancel != nil {
		b.cancel()
	}
	slog.Info("stopped wasm channel adapter", "plugin_id", b.pluginID, "channel", b.channelName)
	return nil
}

func (b *WasmChannelBridge) SendMessage(ctx context.Context, msg channels.OutboundMessage) error {
	if b.inst == nil {
		return fmt.Errorf("plugin instance %s is inactive", b.pluginID)
	}

	msgBytes, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshaling outbound message: %w", err)
	}

	return b.inst.SendChannelMessage(ctx, msgBytes)
}

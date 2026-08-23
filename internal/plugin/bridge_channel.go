package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/actonos/actonos/internal/bus"
	"github.com/actonos/actonos/internal/channels"
)

// WasmChannelBridge adapts a WASM Plugin with CapabilityChannel to channels.ChannelAdapter.
type WasmChannelBridge struct {
	pluginID     string
	channelName  string
	manifest     PluginManifest
	inst         *PluginInstance
	pollInterval time.Duration
	eventBus     *bus.EventBus
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
}

// NewWasmChannelBridge creates a new WasmChannelBridge.
func NewWasmChannelBridge(pluginID string, manifest PluginManifest, inst *PluginInstance, eventBus *bus.EventBus) *WasmChannelBridge {
	channelName := pluginID
	if len(manifest.Channels) > 0 && manifest.Channels[0].Name != "" {
		channelName = manifest.Channels[0].Name
	} else if name, ok := manifest.Config["channel_name"].(string); ok && name != "" {
		channelName = name
	}

	pollInterval := 3 * time.Second
	if intervalSec, ok := manifest.Config["poll_interval_seconds"].(float64); ok && intervalSec > 0 {
		pollInterval = time.Duration(intervalSec) * time.Second
	} else if intervalSec, ok := manifest.Config["poll_interval"].(float64); ok && intervalSec > 0 {
		pollInterval = time.Duration(intervalSec) * time.Second
	}

	return &WasmChannelBridge{
		pluginID:     pluginID,
		channelName:  channelName,
		manifest:     manifest,
		inst:         inst,
		pollInterval: pollInterval,
		eventBus:     eventBus,
	}
}

func (b *WasmChannelBridge) Name() string {
	return b.channelName
}

func (b *WasmChannelBridge) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ctx, b.cancel = context.WithCancel(ctx)
	slog.Info("started wasm channel adapter", "plugin_id", b.pluginID, "channel", b.channelName, "interval", b.pollInterval)

	// Start background polling loop
	go b.pollLoop(b.ctx)

	return nil
}

func (b *WasmChannelBridge) pollLoop(ctx context.Context) {
	ticker := time.NewTicker(b.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.pollOnce(ctx)
		}
	}
}

func (b *WasmChannelBridge) pollOnce(ctx context.Context) {
	b.mu.Lock()
	inst := b.inst
	b.mu.Unlock()

	if inst == nil || ctx.Err() != nil {
		return
	}

	msgBytes, err := inst.PollChannel(ctx)
	if err != nil {
		slog.Debug("wasm channel poll error", "plugin_id", b.pluginID, "error", err)
		return
	}

	if len(msgBytes) == 0 {
		return
	}

	// Try unmarshaling as array of InboundMessage
	var msgs []channels.InboundMessage
	if err := json.Unmarshal(msgBytes, &msgs); err == nil && len(msgs) > 0 {
		for _, msg := range msgs {
			if msg.ChannelID == "" {
				msg.ChannelID = b.channelName
			}
			if b.eventBus != nil {
				b.eventBus.Publish(bus.NewEvent(bus.EventChannelMessage, b.channelName, msg))
			}
		}
		return
	}

	// Try unmarshaling as single InboundMessage
	var single channels.InboundMessage
	if err := json.Unmarshal(msgBytes, &single); err == nil && (single.Content != "" || single.SenderID != "") {
		if single.ChannelID == "" {
			single.ChannelID = b.channelName
		}
		if b.eventBus != nil {
			b.eventBus.Publish(bus.NewEvent(bus.EventChannelMessage, b.channelName, single))
		}
	}
}

func (b *WasmChannelBridge) Stop() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.cancel != nil {
		b.cancel()
		b.cancel = nil
	}
	b.inst = nil
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

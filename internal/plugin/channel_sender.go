package plugin

import (
	"context"

	"github.com/actonos/actonos/internal/channels"
	"github.com/actonos/actonos/internal/tools"
)

// ChannelToolSender adapts ChannelManager to the native_channel_notify tool,
// including typing / reaction / quote envelope fields.
func ChannelToolSender(mgr *channels.ChannelManager) tools.ChannelMessageSender {
	if mgr == nil {
		return nil
	}
	return channelToolSender{mgr: mgr}
}

type channelToolSender struct {
	mgr *channels.ChannelManager
}

func (s channelToolSender) Send(ctx context.Context, channelID, accountID, recipient, content string) error {
	return s.mgr.Send(ctx, channelID, accountID, recipient, content)
}

func (s channelToolSender) SendEnvelope(ctx context.Context, env tools.ChannelEnvelope) error {
	return s.mgr.SendMessage(ctx, channels.OutboundMessage{
		ChannelID: env.ChannelID,
		AccountID: env.AccountID,
		Recipient: env.Recipient,
		Content:   env.Content,
		Kind:      channels.MessageKind(env.Kind),
		ChatID:    env.ChatID,
		ReplyToID: env.ReplyToID,
		ThreadID:  env.ThreadID,
		Reaction:  env.Reaction,
		Action:    env.Action,
		Typing:    env.Typing,
		Metadata:  env.Metadata,
	})
}

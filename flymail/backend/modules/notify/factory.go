package notify

import (
	"fmt"
)

// DefaultChannelFactory creates channel instances based on type
func DefaultChannelFactory(channel *NotifyChannel) (Channel, error) {
	// Create base channel with common properties
	_ = &BaseChannel{
		ID:         channel.ID,
		UserID:     channel.UserID,
		Name:       channel.Name,
		Type:       channel.Type,
		Config:     channel.Config,
		Status:     channel.Status,
		TimeRanges: channel.TimeRanges,
		Events:     channel.Events,
	}

	// Create specific channel implementation based on type
	switch channel.Type {
	case ChannelTypeFeishu:
		// TODO: Implement Feishu channel
		return nil, fmt.Errorf("Feishu channel not implemented yet")

	case ChannelTypeWecom:
		// TODO: Implement Wecom channel
		return nil, fmt.Errorf("Wecom channel not implemented yet")

	case ChannelTypeTelegram:
		// TODO: Implement Telegram channel
		return nil, fmt.Errorf("Telegram channel not implemented yet")

	case ChannelTypeEmail:
		// TODO: Implement Email channel
		return nil, fmt.Errorf("Email channel not implemented yet")

	case ChannelTypeWebhook:
		// TODO: Implement Webhook channel
		return nil, fmt.Errorf("Webhook channel not implemented yet")

	case ChannelTypeSSE:
		// SSE is handled as an internal channel, not created from database
		return nil, fmt.Errorf("SSE channel should not be created from database")

	default:
		return nil, fmt.Errorf("unknown channel type: %s", channel.Type)
	}
}

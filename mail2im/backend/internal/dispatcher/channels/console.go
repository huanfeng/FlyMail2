package channels

import (
	"mail2im/internal/core"

	"flymail-core/logger"
	"go.uber.org/zap"
)

type ConsoleChannel struct {
	minPriority core.EventPriority
}

func NewConsoleChannel(minPriority core.EventPriority) *ConsoleChannel {
	return &ConsoleChannel{minPriority: minPriority}
}

func (c *ConsoleChannel) Name() string {
	return "Console"
}

func (c *ConsoleChannel) Send(event core.Event) error {
	logger.Debug("ConsoleChannel received event",
		zap.String("type", string(event.Type)),
		zap.Int("priority", int(event.Priority)),
		zap.String("source", event.Source))
	return nil
}

func (c *ConsoleChannel) SendRendered(rendered string, event core.Event) error {
	logger.Debug("ConsoleChannel rendered",
		zap.String("type", string(event.Type)),
		zap.String("content", rendered))
	return nil
}

func (c *ConsoleChannel) MinPriority() core.EventPriority {
	return c.minPriority
}

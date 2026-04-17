package channels

import (
	"log"
	"mail2im/internal/core"
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
	log.Printf("[ConsoleChannel] Received event: Type=%s, Priority=%d, Source=%s, Payload=%v",
		event.Type, event.Priority, event.Source, event.Payload)
	return nil
}

func (c *ConsoleChannel) SendRendered(rendered string, event core.Event) error {
	log.Printf("[ConsoleChannel] %s\n%s", event.Type, rendered)
	return nil
}

func (c *ConsoleChannel) MinPriority() core.EventPriority {
	return c.minPriority
}

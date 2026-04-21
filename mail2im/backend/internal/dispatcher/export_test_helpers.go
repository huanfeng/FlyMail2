package dispatcher

import (
	"mail2im/internal/core"
	"time"
)

// NewTestDispatcher creates a Dispatcher with default strategy for testing.
func NewTestDispatcher() *Dispatcher {
	return &Dispatcher{
		channels: make([]channelEntry, 0),
		strategy: NewStrategyEngine(StrategyConfig{}),
		loc:      time.UTC,
	}
}

// NewDispatcherWithStrategy creates a Dispatcher with a custom strategy config.
func NewDispatcherWithStrategy(cfg StrategyConfig) *Dispatcher {
	return &Dispatcher{
		channels: make([]channelEntry, 0),
		strategy: NewStrategyEngine(cfg),
		loc:      time.UTC,
		globalQuiet: quietConfig{
			Enabled: cfg.QuietEnabled,
			Start:   cfg.QuietHoursStart,
			End:     cfg.QuietHoursEnd,
		},
	}
}

// RegisterWithID registers a channel with a specific DB ID for testing channel routing.
func (d *Dispatcher) RegisterWithID(id uint, name, channelType string, c core.NotificationChannel, tmpl string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	idCopy := id
	d.channels = append(d.channels, channelEntry{
		id:              &idCopy,
		name:            name,
		channelType:     channelType,
		sender:          c,
		quiet:           channelQuiet{Mode: "global"},
		templateContent: tmpl,
	})
}

// RegisterWithQuiet registers a channel with a specific quiet mode for testing.
func (d *Dispatcher) RegisterWithQuiet(c core.NotificationChannel, quietMode string, quietEnable bool, quietStart, quietEnd string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.channels = append(d.channels, channelEntry{
		id:     nil,
		name:   c.Name(),
		sender: c,
		quiet: channelQuiet{
			Mode:    quietMode,
			Enabled: quietEnable,
			Start:   quietStart,
			End:     quietEnd,
		},
	})
}

// HandleEventForTest is an exported wrapper around handleEvent for integration tests.
func (d *Dispatcher) HandleEventForTest(event core.Event) {
	d.handleEvent(event)
}

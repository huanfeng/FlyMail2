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

// HandleEventForTest is an exported wrapper around handleEvent for integration tests.
func (d *Dispatcher) HandleEventForTest(event core.Event) {
	d.handleEvent(event)
}

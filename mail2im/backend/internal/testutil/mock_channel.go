package testutil

import (
	"mail2im/internal/core"
	"sync"
	"time"
)

// MockChannel records all events sent to it for verification in tests.
type MockChannel struct {
	name        string
	minPriority core.EventPriority
	mu          sync.Mutex
	events      []core.Event
	notify      chan struct{}
}

func NewMockChannel(name string, minPriority core.EventPriority) *MockChannel {
	return &MockChannel{
		name:        name,
		minPriority: minPriority,
		notify:      make(chan struct{}, 100),
	}
}

func (m *MockChannel) Name() string              { return m.name }
func (m *MockChannel) MinPriority() core.EventPriority { return m.minPriority }

func (m *MockChannel) Send(event core.Event) error {
	m.mu.Lock()
	m.events = append(m.events, event)
	m.mu.Unlock()

	select {
	case m.notify <- struct{}{}:
	default:
	}
	return nil
}

func (m *MockChannel) SendRendered(rendered string, event core.Event) error {
	return m.Send(event)
}

// Events returns all recorded events.
func (m *MockChannel) Events() []core.Event {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]core.Event, len(m.events))
	copy(result, m.events)
	return result
}

// WaitForEvent waits up to timeout for at least one event.
// Returns true if an event was received, false on timeout.
func (m *MockChannel) WaitForEvent(timeout time.Duration) bool {
	select {
	case <-m.notify:
		return true
	case <-time.After(timeout):
		return false
	}
}

// Reset clears all recorded events.
func (m *MockChannel) Reset() {
	m.mu.Lock()
	m.events = nil
	m.mu.Unlock()
	// Drain notify channel
	for {
		select {
		case <-m.notify:
		default:
			return
		}
	}
}

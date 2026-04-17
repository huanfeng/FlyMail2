package core

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestEventBus_PublishSubscribe(t *testing.T) {
	bus := &EventBus{
		subscribers: make(map[EventType][]EventHandler),
		eventChan:   make(chan Event, 100),
	}
	go bus.dispatcher()

	var received atomic.Int32

	bus.Subscribe(EventEmailReceived, func(event Event) {
		received.Add(1)
	})

	bus.Publish(Event{
		Type:     EventEmailReceived,
		Priority: PriorityNormal,
		Source:   "test",
		Payload:  map[string]any{"subject": "test email"},
	})

	// Wait for async delivery
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if received.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if received.Load() != 1 {
		t.Errorf("expected 1 event received, got %d", received.Load())
	}
}

func TestEventBus_MultipleSubscribers(t *testing.T) {
	bus := &EventBus{
		subscribers: make(map[EventType][]EventHandler),
		eventChan:   make(chan Event, 100),
	}
	go bus.dispatcher()

	var count1, count2 atomic.Int32

	bus.Subscribe(EventEmailReceived, func(event Event) { count1.Add(1) })
	bus.Subscribe(EventEmailReceived, func(event Event) { count2.Add(1) })

	bus.Publish(Event{Type: EventEmailReceived, Priority: PriorityNormal, Source: "test"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if count1.Load() > 0 && count2.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if count1.Load() != 1 || count2.Load() != 1 {
		t.Errorf("expected both subscribers to receive 1 event, got %d and %d", count1.Load(), count2.Load())
	}
}

func TestEventBus_DifferentEventTypes(t *testing.T) {
	bus := &EventBus{
		subscribers: make(map[EventType][]EventHandler),
		eventChan:   make(chan Event, 100),
	}
	go bus.dispatcher()

	var emailCount, authCount atomic.Int32

	bus.Subscribe(EventEmailReceived, func(event Event) { emailCount.Add(1) })
	bus.Subscribe(EventAuthFailed, func(event Event) { authCount.Add(1) })

	bus.Publish(Event{Type: EventEmailReceived, Priority: PriorityNormal, Source: "test"})
	bus.Publish(Event{Type: EventAuthFailed, Priority: PriorityCritical, Source: "test"})

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if emailCount.Load() > 0 && authCount.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if emailCount.Load() != 1 {
		t.Errorf("expected 1 email event, got %d", emailCount.Load())
	}
	if authCount.Load() != 1 {
		t.Errorf("expected 1 auth event, got %d", authCount.Load())
	}
}

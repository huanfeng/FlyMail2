package core

import (
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
)

type EventType string
type EventPriority int

const (
	PriorityLow      EventPriority = 0
	PriorityNormal   EventPriority = 1
	PriorityHigh     EventPriority = 2
	PriorityCritical EventPriority = 3
)

const (
	EventEmailReceived    EventType = "email_received"
	EventAuthFailed       EventType = "auth_failed"
	EventConnectionLost   EventType = "connection_lost"
	EventPasswordExpiring EventType = "password_expiring"
	EventSystemError      EventType = "system_error"
)

type Event struct {
	ID        string
	Type      EventType
	Priority  EventPriority
	Source    string      // e.g., "account:1"
	Payload   interface{} // Arbitrary data
	CreatedAt time.Time
}

type EventHandler func(event Event)

type EventBus struct {
	subscribers map[EventType][]EventHandler
	mu          sync.RWMutex
	eventChan   chan Event
}

var Bus *EventBus

func InitEventBus() {
	Bus = &EventBus{
		subscribers: make(map[EventType][]EventHandler),
		eventChan:   make(chan Event, 1000),
	}
	go Bus.dispatcher()
	log.Println("EventBus initialized")
}

func (b *EventBus) Subscribe(eventType EventType, handler EventHandler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers[eventType] = append(b.subscribers[eventType], handler)
}

func (b *EventBus) Publish(event Event) {
	if event.ID == "" {
		event.ID = uuid.New().String()
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}

	select {
	case b.eventChan <- event:
	default:
		log.Printf("ERROR: EventBus queue full, dropping event: %v", event.Type)
	}
}

func (b *EventBus) dispatcher() {
	for event := range b.eventChan {
		b.mu.RLock()
		handlers := b.subscribers[event.Type]
		b.mu.RUnlock()

		for _, handler := range handlers {
			go func(h EventHandler, e Event) {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("Panic in event handler: %v", r)
					}
				}()
				h(e)
			}(handler, event)
		}
	}
}

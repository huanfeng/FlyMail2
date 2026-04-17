package realtime

import (
	"strings"
	"sync"
)

// subscriptionManager implements SubscriptionManager interface
type subscriptionManager struct {
	subscriptions map[string]*Subscription
	mu            sync.RWMutex
}

// NewSubscriptionManager creates a new subscription manager
func NewSubscriptionManager() SubscriptionManager {
	return &subscriptionManager{
		subscriptions: make(map[string]*Subscription),
	}
}

// Subscribe adds a subscription for a client
func (sm *subscriptionManager) Subscribe(clientID string, subscription *Subscription) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	if subscription == nil {
		subscription = &Subscription{
			Topics:  []string{},
			Filters: make(map[string]string),
		}
	}

	sm.subscriptions[clientID] = subscription
	return nil
}

// Unsubscribe removes a subscription for a client
func (sm *subscriptionManager) Unsubscribe(clientID string, topic string) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sub, exists := sm.subscriptions[clientID]
	if !exists {
		return nil
	}

	// Remove topic from subscription
	newTopics := make([]string, 0, len(sub.Topics))
	for _, t := range sub.Topics {
		if t != topic {
			newTopics = append(newTopics, t)
		}
	}
	sub.Topics = newTopics

	return nil
}

// GetSubscriptions returns all subscriptions for a client
func (sm *subscriptionManager) GetSubscriptions(clientID string) (*Subscription, error) {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sub, exists := sm.subscriptions[clientID]
	if !exists {
		return nil, nil
	}

	// Return a copy
	return &Subscription{
		Topics:     append([]string{}, sub.Topics...),
		EventTypes: append([]string{}, sub.EventTypes...),
		Filters:    copyMap(sub.Filters),
	}, nil
}

// ShouldReceive checks if a client should receive an event
func (sm *subscriptionManager) ShouldReceive(clientID string, topic string, event *Event) bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	sub, exists := sm.subscriptions[clientID]
	if !exists {
		// No specific subscription, allow all
		return true
	}

	// Check event type filter
	if len(sub.EventTypes) > 0 {
		eventTypeAllowed := false
		for _, et := range sub.EventTypes {
			if string(event.Type) == et {
				eventTypeAllowed = true
				break
			}
		}
		if !eventTypeAllowed {
			return false
		}
	}

	// Check custom filters
	for key, value := range sub.Filters {
		// Special filter: severity
		if key == "severity" && event.Data != nil {
			if severity, ok := event.Data["severity"].(string); ok {
				if !matchesFilter(severity, value) {
					return false
				}
			}
		}

		// Special filter: account_id
		if key == "account_id" && event.Data != nil {
			if accountID, ok := event.Data["account_id"]; ok {
				accountIDStr := ""
				switch v := accountID.(type) {
				case uint:
					accountIDStr = string(v)
				case float64:
					accountIDStr = string(uint(v))
				case string:
					accountIDStr = v
				}
				if !matchesFilter(accountIDStr, value) {
					return false
				}
			}
		}
	}

	return true
}

// Clear removes all subscriptions for a client
func (sm *subscriptionManager) Clear(clientID string) {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	delete(sm.subscriptions, clientID)
}

// Helper functions

func copyMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	result := make(map[string]string)
	for k, v := range m {
		result[k] = v
	}
	return result
}

func matchesFilter(value, filter string) bool {
	// Support comma-separated values in filter
	filters := strings.Split(filter, ",")
	for _, f := range filters {
		f = strings.TrimSpace(f)
		if f == value {
			return true
		}
		// Support wildcard
		if strings.HasSuffix(f, "*") {
			prefix := strings.TrimSuffix(f, "*")
			if strings.HasPrefix(value, prefix) {
				return true
			}
		}
	}
	return false
}

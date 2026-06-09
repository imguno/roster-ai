package event

import (
	"context"
	"sync"

	"github.com/roster-io/roster/pkg/types"
)

// Handler is called when an event is delivered to a subscriber.
type Handler func(ctx context.Context, ev types.Event) error

// Subscription binds a handler to one or more event types.
type Subscription struct {
	ID         string   // subscriber ID (desk or group)
	EventTypes []string // event types to match (empty = all)
	Handler    Handler
}

// Bus is the central event routing system.
// Desks subscribe to events. Desks emit events. Resources emit events.
// The Organization's routing rules are just subscriptions created at startup.
type Bus struct {
	mu          sync.RWMutex
	subscribers []*Subscription
	history     []types.Event
	maxHistory  int
}

// NewBus creates an event bus with the given history buffer size.
func NewBus(maxHistory int) *Bus {
	if maxHistory <= 0 {
		maxHistory = 1000
	}
	return &Bus{
		maxHistory: maxHistory,
	}
}

// Subscribe registers a handler for the given event types.
// If eventTypes is empty, the handler receives all events.
func (b *Bus) Subscribe(id string, eventTypes []string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subscribers = append(b.subscribers, &Subscription{
		ID:         id,
		EventTypes: eventTypes,
		Handler:    handler,
	})
}

// Unsubscribe removes all subscriptions for the given subscriber ID.
func (b *Bus) Unsubscribe(id string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	filtered := b.subscribers[:0]
	for _, s := range b.subscribers {
		if s.ID != id {
			filtered = append(filtered, s)
		}
	}
	b.subscribers = filtered
}

// Publish emits an event to all matching subscribers.
// Delivery is synchronous in order; errors are collected but do not stop delivery.
func (b *Bus) Publish(ctx context.Context, ev types.Event) []error {
	b.mu.Lock()
	b.history = append(b.history, ev)
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}
	// Copy subscribers to release the lock during dispatch.
	subs := make([]*Subscription, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.Unlock()

	var errs []error
	for _, sub := range subs {
		if !matches(sub.EventTypes, ev.Type) {
			continue
		}
		if err := sub.Handler(ctx, ev); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// PublishAsync emits an event asynchronously — each subscriber is invoked in its own goroutine.
func (b *Bus) PublishAsync(ctx context.Context, ev types.Event) {
	b.mu.Lock()
	b.history = append(b.history, ev)
	if len(b.history) > b.maxHistory {
		b.history = b.history[len(b.history)-b.maxHistory:]
	}
	subs := make([]*Subscription, len(b.subscribers))
	copy(subs, b.subscribers)
	b.mu.Unlock()

	for _, sub := range subs {
		if !matches(sub.EventTypes, ev.Type) {
			continue
		}
		go sub.Handler(ctx, ev) //nolint:errcheck
	}
}

// History returns the recent event history.
func (b *Bus) History() []types.Event {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]types.Event, len(b.history))
	copy(out, b.history)
	return out
}

// matches returns true if eventTypes is empty (wildcard) or contains the given type.
func matches(eventTypes []string, eventType string) bool {
	if len(eventTypes) == 0 {
		return true
	}
	for _, t := range eventTypes {
		if t == eventType {
			return true
		}
	}
	return false
}

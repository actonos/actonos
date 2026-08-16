package bus

import (
	"sync"
)

// DefaultChannelBufferSize is the default buffer size for subscriber channels.
const DefaultChannelBufferSize = 64

// EventBus is an asynchronous, thread-safe in-memory event bus.
type EventBus struct {
	mu          sync.RWMutex
	subscribers map[string][]chan Event
	closed      bool
}

// NewEventBus instantiates a new EventBus.
func NewEventBus() *EventBus {
	return &EventBus{
		subscribers: make(map[string][]chan Event),
	}
}

// Publish broadcasts an event to all subscribers registered for the event's type or wildcard "*".
// If a subscriber's channel is full, the event is dropped for that subscriber to avoid blocking the publisher.
func (b *EventBus) Publish(event Event) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.closed {
		return
	}

	// Dispatch to specific topic subscribers
	if subs, ok := b.subscribers[event.Type]; ok {
		for _, ch := range subs {
			select {
			case ch <- event:
			default:
				// Buffer full; drop to prevent system deadlock
			}
		}
	}

	// Dispatch to wildcard subscribers
	if subs, ok := b.subscribers["*"]; ok {
		for _, ch := range subs {
			select {
			case ch <- event:
			default:
			}
		}
	}
}

// Subscribe creates and returns a read-only channel listening for a specific event type or "*" (all events).
func (b *EventBus) Subscribe(eventType string) <-chan Event {
	b.mu.Lock()
	defer b.mu.Unlock()

	ch := make(chan Event, DefaultChannelBufferSize)
	if b.closed {
		close(ch)
		return ch
	}

	b.subscribers[eventType] = append(b.subscribers[eventType], ch)
	return ch
}

// Unsubscribe removes a channel from the subscriber list and closes the channel.
func (b *EventBus) Unsubscribe(eventType string, ch <-chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs, ok := b.subscribers[eventType]
	if !ok {
		return
	}

	for i, sub := range subs {
		if sub == ch {
			b.subscribers[eventType] = append(subs[:i], subs[i+1:]...)
			close(sub)
			break
		}
	}
}

// Close closes all subscriber channels and shuts down the EventBus.
func (b *EventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return
	}

	b.closed = true
	for _, subs := range b.subscribers {
		for _, ch := range subs {
			close(ch)
		}
	}
	b.subscribers = make(map[string][]chan Event)
}

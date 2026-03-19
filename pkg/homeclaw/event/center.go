package event

import (
	"context"
	"sync"
	"sync/atomic"
)

// Center is the central event dispatcher (singleton pattern like Spring's ApplicationEventPublisher)
type Center struct {
	listeners         map[EventType][]Listener
	wildcardListeners []Listener
	mu                sync.RWMutex
	async             bool
	buffer            chan Event
	bufferSize        int
	closed            atomic.Bool
	ctx               context.Context
	cancel            context.CancelFunc
}

// defaultBufferSize is the default size of the async event buffer
const defaultBufferSize = 256

var (
	instance *Center
	once     sync.Once
)

// GetCenter returns the singleton instance of Center
func GetCenter() *Center {
	once.Do(func() {
		instance = NewCenter()
	})
	return instance
}

// NewCenter creates a new Center instance (for testing or custom use)
func NewCenter() *Center {
	ctx, cancel := context.WithCancel(context.Background())
	return &Center{
		listeners:         make(map[EventType][]Listener),
		wildcardListeners: make([]Listener, 0),
		bufferSize:        defaultBufferSize,
		ctx:               ctx,
		cancel:            cancel,
	}
}

// SetAsync enables or disables async event dispatch
func (c *Center) SetAsync(async bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.async = async
	if async && c.buffer == nil {
		c.buffer = make(chan Event, c.bufferSize)
		go c.dispatchLoop()
	}
}

// SetBufferSize sets the buffer size for async mode (must be called before SetAsync)
func (c *Center) SetBufferSize(size int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bufferSize = size
}

// Subscribe registers a listener for the given event type
func (c *Center) Subscribe(eventType EventType, listener Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.listeners[eventType] = append(c.listeners[eventType], listener)
}

// SubscribeTypes registers a listener for multiple event types
func (c *Center) SubscribeTypes(listener Listener, eventTypes ...EventType) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, eventType := range eventTypes {
		c.listeners[eventType] = append(c.listeners[eventType], listener)
	}
}

// SubscribeAll registers a listener for all event types (wildcard)
func (c *Center) SubscribeAll(listener Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.wildcardListeners = append(c.wildcardListeners, listener)
}

// Unsubscribe removes a listener from the given event type
func (c *Center) Unsubscribe(eventType EventType, listener Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()
	listeners := c.listeners[eventType]
	for i, l := range listeners {
		if l == listener {
			c.listeners[eventType] = append(listeners[:i], listeners[i+1:]...)
			break
		}
	}
}

// UnsubscribeAll removes a listener from all event types including wildcard
func (c *Center) UnsubscribeAll(listener Listener) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Remove from specific types
	for eventType, listeners := range c.listeners {
		for i, l := range listeners {
			if l == listener {
				c.listeners[eventType] = append(listeners[:i], listeners[i+1:]...)
				break
			}
		}
	}

	// Remove from wildcard listeners
	for i, l := range c.wildcardListeners {
		if l == listener {
			c.wildcardListeners = append(c.wildcardListeners[:i], c.wildcardListeners[i+1:]...)
			break
		}
	}
}

// Publish publishes an event to all subscribed listeners
func (c *Center) Publish(event Event) {
	if c.closed.Load() {
		return
	}

	if c.async && c.buffer != nil {
		select {
		case c.buffer <- event:
		default:
			// Buffer full, drop event or handle overflow
		}
	} else {
		c.dispatch(event)
	}
}

// PublishWithData creates and publishes an event with the given data
func (c *Center) PublishWithData(eventType EventType, source string, data map[string]any) {
	event := NewEventWithData(eventType, source, data)
	c.Publish(event)
}

// dispatch sends the event to all matching listeners synchronously
func (c *Center) dispatch(event Event) {
	c.mu.RLock()
	listeners := c.listeners[event.Type]
	wildcards := c.wildcardListeners
	c.mu.RUnlock()

	// Notify type-specific listeners
	for _, listener := range listeners {
		if listener.Supports(event.Type) {
			go listener.OnEvent(event)
		}
	}

	// Notify wildcard listeners
	for _, listener := range wildcards {
		go listener.OnEvent(event)
	}
}

// dispatchLoop runs in a goroutine for async mode
func (c *Center) dispatchLoop() {
	for {
		select {
		case event := <-c.buffer:
			c.dispatch(event)
		case <-c.ctx.Done():
			return
		}
	}
}

// Close stops the event center and cleans up resources
func (c *Center) Close() {
	if c.closed.CompareAndSwap(false, true) {
		c.cancel()

		c.mu.Lock()
		defer c.mu.Unlock()

		// Clear all listeners
		c.listeners = make(map[EventType][]Listener)
		c.wildcardListeners = c.wildcardListeners[:0]

		// Drain buffer
		if c.buffer != nil {
			close(c.buffer)
			for range c.buffer {
				// Drain remaining events
			}
		}
	}
}

// GetListenerCount returns the number of listeners for the given event type
func (c *Center) GetListenerCount(eventType EventType) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.listeners[eventType])
}

// GetTotalListenerCount returns the total number of all listeners
func (c *Center) GetTotalListenerCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	count := len(c.wildcardListeners)
	for _, listeners := range c.listeners {
		count += len(listeners)
	}
	return count
}

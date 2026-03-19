package event

// Listener is the interface for event subscribers
type Listener interface {
	// OnEvent is called when an event is received
	OnEvent(event Event)
	// Supports returns true if this listener supports the given event type
	Supports(eventType EventType) bool
}

// ListenerFunc is a function type that implements Listener interface
type ListenerFunc struct {
	eventType EventType
	handler   func(Event)
}

// OnEvent implements Listener interface
func (f *ListenerFunc) OnEvent(event Event) {
	if f.handler != nil {
		f.handler(event)
	}
}

// Supports implements Listener interface
func (f *ListenerFunc) Supports(eventType EventType) bool {
	return f.eventType == eventType
}

// NewListener creates a new ListenerFunc for the given event type
func NewListener(eventType EventType, handler func(Event)) *ListenerFunc {
	return &ListenerFunc{
		eventType: eventType,
		handler:   handler,
	}
}

// WildcardListener is a listener that receives all events
type WildcardListener struct {
	handler func(Event)
}

// OnEvent implements Listener interface
func (w *WildcardListener) OnEvent(event Event) {
	if w.handler != nil {
		w.handler(event)
	}
}

// Supports implements Listener interface - always returns true for wildcard
func (w *WildcardListener) Supports(eventType EventType) bool {
	return true
}

// NewWildcardListener creates a new listener that receives all events
func NewWildcardListener(handler func(Event)) *WildcardListener {
	return &WildcardListener{handler: handler}
}

// MultiTypeListener listens to multiple event types
type MultiTypeListener struct {
	eventTypes []EventType
	handler    func(Event)
}

// OnEvent implements Listener interface
func (m *MultiTypeListener) OnEvent(event Event) {
	if m.handler != nil {
		m.handler(event)
	}
}

// Supports implements Listener interface
func (m *MultiTypeListener) Supports(eventType EventType) bool {
	for _, t := range m.eventTypes {
		if t == eventType {
			return true
		}
	}
	return false
}

// NewMultiTypeListener creates a listener for multiple event types
func NewMultiTypeListener(handler func(Event), eventTypes ...EventType) *MultiTypeListener {
	return &MultiTypeListener{
		eventTypes: eventTypes,
		handler:    handler,
	}
}

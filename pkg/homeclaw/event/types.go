package event

import "time"

// EventType represents the type of event
type EventType string

// Event type constants
const (
	EventTypeDevice    EventType = "device"
	EventTypeRoom      EventType = "room"
	EventTypeProp      EventType = "prop"
	EventTypeDeviceMsg EventType = "device_msg"
	EventTypeToken     EventType = "token"
	EventTypeNet       EventType = "net"
)

// Event represents a unified event structure
type Event struct {
	Type      EventType      // Event type enum
	Source    string         // Who published the event
	Timestamp time.Time      // Event timestamp
	Data      map[string]any // Flexible data payload
}

// NewEvent creates a new Event with the given type and source
func NewEvent(eventType EventType, source string) Event {
	return Event{
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now(),
		Data:      make(map[string]any),
	}
}

// NewEventWithData creates a new Event with the given type, source and data
func NewEventWithData(eventType EventType, source string, data map[string]any) Event {
	return Event{
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// Set adds a key-value pair to the event data
func (e *Event) Set(key string, value any) {
	if e.Data == nil {
		e.Data = make(map[string]any)
	}
	e.Data[key] = value
}

// Get retrieves a value from the event data by key
func (e *Event) Get(key string) (any, bool) {
	if e.Data == nil {
		return nil, false
	}
	val, ok := e.Data[key]
	return val, ok
}

// GetString retrieves a string value from the event data
func (e *Event) GetString(key string) (string, bool) {
	val, ok := e.Get(key)
	if !ok {
		return "", false
	}
	str, ok := val.(string)
	return str, ok
}

// IsType checks if the event is of the given type
func (e *Event) IsType(eventType EventType) bool {
	return e.Type == eventType
}

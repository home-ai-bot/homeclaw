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

// ==================== Event Data Types ====================

// TokenData represents token update event payload
type TokenData struct {
	AccessToken    string    `json:"access_token"`
	RefreshToken   string    `json:"refresh_token,omitempty"`
	TokenExpiresAt time.Time `json:"token_expires_at"`
}

// NetData represents network status event payload
type NetData struct {
	Kind    string `json:"kind"` // "status" or "interface"
	Online  bool   `json:"online,omitempty"`
	Status  int    `json:"status,omitempty"`
	Name    string `json:"name,omitempty"`
	IP      string `json:"ip,omitempty"`
	Netmask string `json:"netmask,omitempty"`
	NetSeg  string `json:"netseg,omitempty"`
}

// MDNSData represents mDNS discovery event payload
type MDNSData struct {
	State   string         `json:"state"`
	GroupID string         `json:"group_id"`
	Service map[string]any `json:"service"`
}

// ==================== Event Structure ====================

// Event represents a unified event structure
type Event struct {
	Type      EventType // Event type enum
	Source    string    // Who published the event
	Timestamp time.Time // Event timestamp
	Data      any       // Payload: Device, Space, TokenData, NetData, etc.
}

// NewEvent creates a new Event with the given type, source, and data payload
func NewEvent(eventType EventType, source string, data any) Event {
	return Event{
		Type:      eventType,
		Source:    source,
		Timestamp: time.Now(),
		Data:      data,
	}
}

// IsType checks if the event is of the given type
func (e *Event) IsType(eventType EventType) bool {
	return e.Type == eventType
}

// ==================== Type-Safe Accessors ====================

// TokenData returns the payload as *TokenData, or nil if type mismatch
func (e *Event) TokenData() *TokenData {
	if d, ok := e.Data.(*TokenData); ok {
		return d
	}
	return nil
}

// NetData returns the payload as *NetData, or nil if type mismatch
func (e *Event) NetData() *NetData {
	if d, ok := e.Data.(*NetData); ok {
		return d
	}
	return nil
}

// MDNSData returns the payload as *MDNSData, or nil if type mismatch
func (e *Event) MDNSData() *MDNSData {
	if d, ok := e.Data.(*MDNSData); ok {
		return d
	}
	return nil
}

// MapData returns the payload as map[string]any for backward compatibility
func (e *Event) MapData() map[string]any {
	if d, ok := e.Data.(map[string]any); ok {
		return d
	}
	return nil
}

package event

import (
	"sync"
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	e := NewEvent(EventTypeDevice, "test-source", nil)

	if e.Type != EventTypeDevice {
		t.Errorf("expected type %s, got %s", EventTypeDevice, e.Type)
	}
	if e.Source != "test-source" {
		t.Errorf("expected source test-source, got %s", e.Source)
	}
}

func TestEventDataTypes(t *testing.T) {
	// Test TokenData
	tokenData := &TokenData{
		AccessToken:    "test-token",
		RefreshToken:   "refresh-token",
		TokenExpiresAt: time.Now().Add(time.Hour),
	}
	e := NewEvent(EventTypeToken, "test", tokenData)

	if got := e.TokenData(); got == nil {
		t.Error("expected TokenData to be returned")
	} else if got.AccessToken != "test-token" {
		t.Errorf("expected access_token test-token, got %s", got.AccessToken)
	}

	// Test NetData
	netData := &NetData{
		Kind:   "status",
		Online: true,
	}
	e2 := NewEvent(EventTypeNet, "test", netData)
	if got := e2.NetData(); got == nil {
		t.Error("expected NetData to be returned")
	} else if got.Kind != "status" {
		t.Errorf("expected kind status, got %s", got.Kind)
	}

	// Test type mismatch returns nil
	if e.NetData() != nil {
		t.Error("expected nil for wrong type")
	}
}

func TestEventMapData(t *testing.T) {
	mapData := map[string]any{"key": "value"}
	e := NewEvent(EventTypeDevice, "test", mapData)

	got := e.MapData()
	if got == nil {
		t.Error("expected map data")
	}
	if got["key"] != "value" {
		t.Errorf("expected value, got %v", got["key"])
	}
}

func TestEventIsType(t *testing.T) {
	e := NewEvent(EventTypeDevice, "test", nil)

	if !e.IsType(EventTypeDevice) {
		t.Error("expected IsType to return true for Device")
	}

	if e.IsType(EventTypeRoom) {
		t.Error("expected IsType to return false for Room")
	}
}

func TestCenterSubscribeAndPublish(t *testing.T) {
	center := NewCenter()
	defer center.Close()

	var received Event
	var mu sync.Mutex

	listener := NewListener(EventTypeDevice, func(e Event) {
		mu.Lock()
		received = e
		mu.Unlock()
	})

	center.Subscribe(EventTypeDevice, listener)

	eventData := map[string]any{"device_id": "123"}
	event := NewEvent(EventTypeDevice, "test-source", eventData)
	center.Publish(event)

	// Wait for async dispatch
	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if received.Type != EventTypeDevice {
		t.Errorf("expected type %s, got %s", EventTypeDevice, received.Type)
	}
	if received.Source != "test-source" {
		t.Errorf("expected source test-source, got %s", received.Source)
	}
	mu.Unlock()
}

func TestCenterWildcardListener(t *testing.T) {
	center := NewCenter()
	defer center.Close()

	var count int
	var mu sync.Mutex

	listener := NewWildcardListener(func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	center.SubscribeAll(listener)

	center.Publish(NewEvent(EventTypeDevice, "test", nil))
	center.Publish(NewEvent(EventTypeRoom, "test", nil))
	center.Publish(NewEvent(EventTypeToken, "test", nil))

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if count != 3 {
		t.Errorf("expected 3 events, got %d", count)
	}
	mu.Unlock()
}

func TestCenterMultiTypeListener(t *testing.T) {
	center := NewCenter()
	defer center.Close()

	var count int
	var mu sync.Mutex

	listener := NewMultiTypeListener(func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	}, EventTypeDevice, EventTypeRoom)

	center.SubscribeTypes(listener, EventTypeDevice, EventTypeRoom)

	center.Publish(NewEvent(EventTypeDevice, "test", nil))
	center.Publish(NewEvent(EventTypeRoom, "test", nil))
	center.Publish(NewEvent(EventTypeToken, "test", nil))

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if count != 2 {
		t.Errorf("expected 2 events, got %d", count)
	}
	mu.Unlock()
}

func TestCenterUnsubscribe(t *testing.T) {
	center := NewCenter()
	defer center.Close()

	var count int
	var mu sync.Mutex

	listener := NewListener(EventTypeDevice, func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	center.Subscribe(EventTypeDevice, listener)
	center.Publish(NewEvent(EventTypeDevice, "test", nil))

	time.Sleep(50 * time.Millisecond)

	center.Unsubscribe(EventTypeDevice, listener)
	center.Publish(NewEvent(EventTypeDevice, "test", nil))

	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if count != 1 {
		t.Errorf("expected 1 event, got %d", count)
	}
	mu.Unlock()
}

func TestCenterAsyncMode(t *testing.T) {
	center := NewCenter()
	defer center.Close()

	center.SetAsync(true)

	var count int
	var mu sync.Mutex

	listener := NewListener(EventTypeDevice, func(e Event) {
		mu.Lock()
		count++
		mu.Unlock()
	})

	center.Subscribe(EventTypeDevice, listener)

	center.Publish(NewEvent(EventTypeDevice, "test", nil))

	time.Sleep(100 * time.Millisecond)

	mu.Lock()
	if count != 1 {
		t.Errorf("expected 1 event in async mode, got %d", count)
	}
	mu.Unlock()
}

func TestGetCenterSingleton(t *testing.T) {
	c1 := GetCenter()
	c2 := GetCenter()

	if c1 != c2 {
		t.Error("expected GetCenter to return the same instance")
	}
}

func TestCenterListenerCount(t *testing.T) {
	center := NewCenter()
	defer center.Close()

	listener1 := NewListener(EventTypeDevice, func(e Event) {})
	listener2 := NewListener(EventTypeDevice, func(e Event) {})
	listener3 := NewListener(EventTypeRoom, func(e Event) {})

	center.Subscribe(EventTypeDevice, listener1)
	center.Subscribe(EventTypeDevice, listener2)
	center.Subscribe(EventTypeRoom, listener3)

	if count := center.GetListenerCount(EventTypeDevice); count != 2 {
		t.Errorf("expected 2 device listeners, got %d", count)
	}

	if count := center.GetListenerCount(EventTypeRoom); count != 1 {
		t.Errorf("expected 1 room listener, got %d", count)
	}

	if count := center.GetTotalListenerCount(); count != 3 {
		t.Errorf("expected 3 total listeners, got %d", count)
	}
}

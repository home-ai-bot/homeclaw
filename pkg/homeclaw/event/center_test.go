package event

import (
	"sync"
	"testing"
	"time"
)

func TestNewEvent(t *testing.T) {
	e := NewEvent(EventTypeDevice, "test-source")

	if e.Type != EventTypeDevice {
		t.Errorf("expected type %s, got %s", EventTypeDevice, e.Type)
	}
	if e.Source != "test-source" {
		t.Errorf("expected source test-source, got %s", e.Source)
	}
	if e.Data == nil {
		t.Error("expected Data to be initialized")
	}
}

func TestEventSetGet(t *testing.T) {
	e := NewEvent(EventTypeRoom, "test")
	e.Set("key1", "value1")
	e.Set("key2", 123)

	val1, ok := e.Get("key1")
	if !ok || val1 != "value1" {
		t.Errorf("expected value1, got %v", val1)
	}

	val2, ok := e.Get("key2")
	if !ok || val2 != 123 {
		t.Errorf("expected 123, got %v", val2)
	}

	_, ok = e.Get("nonexistent")
	if ok {
		t.Error("expected nonexistent key to return false")
	}
}

func TestEventGetString(t *testing.T) {
	e := NewEvent(EventTypeDevice, "test")
	e.Set("str", "value")
	e.Set("int", 123)

	str, ok := e.GetString("str")
	if !ok || str != "value" {
		t.Errorf("expected value, got %s", str)
	}

	_, ok = e.GetString("int")
	if ok {
		t.Error("expected int to not be convertible to string")
	}
}

func TestEventIsType(t *testing.T) {
	e := NewEvent(EventTypeDevice, "test")

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

	event := NewEvent(EventTypeDevice, "test-source")
	event.Set("device_id", "123")
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

	center.Publish(NewEvent(EventTypeDevice, "test"))
	center.Publish(NewEvent(EventTypeRoom, "test"))
	center.Publish(NewEvent(EventTypeToken, "test"))

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

	center.Publish(NewEvent(EventTypeDevice, "test"))
	center.Publish(NewEvent(EventTypeRoom, "test"))
	center.Publish(NewEvent(EventTypeToken, "test"))

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
	center.Publish(NewEvent(EventTypeDevice, "test"))

	time.Sleep(50 * time.Millisecond)

	center.Unsubscribe(EventTypeDevice, listener)
	center.Publish(NewEvent(EventTypeDevice, "test"))

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

	center.Publish(NewEvent(EventTypeDevice, "test"))

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

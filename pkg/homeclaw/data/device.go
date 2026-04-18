// Package data provides data access layer for HomeClaw.
package data

import "sync"

// DeviceStore defines the interface for device data operations
type DeviceStore interface {
	GetAll() ([]Device, error)
	Save(devices ...Device) error
	Delete(fromID, from string) error
}

// deviceStore implements DeviceStore using JSONStore.
// All read operations reload from disk to ensure fresh data after gateway syncs.
type deviceStore struct {
	store *JSONStore
	mu    sync.Mutex
	data  DevicesData
}

// NewDeviceStore creates a new DeviceStore
func NewDeviceStore(store *JSONStore) (DeviceStore, error) {
	s := &deviceStore{store: store}
	// Don't fail if file doesn't exist or is corrupted - just initialize with empty data
	_ = s.load()
	return s, nil
}

// load reads data from file. Caller must hold mu.
func (s *deviceStore) load() error {
	s.data = DevicesData{Version: "1", Devices: []Device{}}
	return s.store.Read("devices", &s.data)
}

// save writes data to file. Caller must hold mu.
func (s *deviceStore) save() error {
	return s.store.Write("devices", s.data)
}

// GetAll returns all devices, always reloading from disk to reflect latest changes.
func (s *deviceStore) GetAll() ([]Device, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	return s.data.Devices, nil
}

// Save saves devices (insert or update), reloading from disk first to avoid overwriting concurrent changes.
func (s *deviceStore) Save(devices ...Device) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	for _, device := range devices {
		found := false
		for i := range s.data.Devices {
			if s.data.Devices[i].FromID == device.FromID && s.data.Devices[i].From == device.From {
				s.data.Devices[i] = device
				found = true
				break
			}
		}
		if !found {
			s.data.Devices = append(s.data.Devices, device)
		}
	}
	return s.save()
}

// Delete deletes a device by FromID and From, reloading from disk first.
func (s *deviceStore) Delete(fromID, from string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	for i := range s.data.Devices {
		if s.data.Devices[i].FromID == fromID && s.data.Devices[i].From == from {
			s.data.Devices = append(s.data.Devices[:i], s.data.Devices[i+1:]...)
			return s.save()
		}
	}
	return ErrRecordNotFound
}

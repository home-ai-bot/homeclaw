// Package data provides data access layer for HomeClaw.
package data

// DeviceStore defines the interface for device data operations
type DeviceStore interface {
	GetAll() ([]Device, error)
	Save(devices ...Device) error
	Delete(fromID, from string) error
}

// deviceStore implements DeviceStore using JSONStore
type deviceStore struct {
	store *JSONStore
	data  DevicesData
}

// NewDeviceStore creates a new DeviceStore
func NewDeviceStore(store *JSONStore) (DeviceStore, error) {
	s := &deviceStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads data from file
func (s *deviceStore) load() error {
	s.data = DevicesData{Version: "1", Devices: []Device{}}
	return s.store.Read("devices", &s.data)
}

// save writes data to file
func (s *deviceStore) save() error {
	return s.store.Write("devices", s.data)
}

// GetAll returns all devices
func (s *deviceStore) GetAll() ([]Device, error) {
	return s.data.Devices, nil
}

// Save saves devices (insert or update)
func (s *deviceStore) Save(devices ...Device) error {
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

// Delete deletes a device by FromID and From
func (s *deviceStore) Delete(fromID, from string) error {
	for i := range s.data.Devices {
		if s.data.Devices[i].FromID == fromID && s.data.Devices[i].From == from {
			s.data.Devices = append(s.data.Devices[:i], s.data.Devices[i+1:]...)
			return s.save()
		}
	}
	return ErrRecordNotFound
}

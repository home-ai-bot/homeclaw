// Package data provides data access layer for HomeClaw.
package data

// DeviceStore defines the interface for device data operations
type DeviceStore interface {
	GetAll() ([]Device, error)
	GetByID(id string) (*Device, error)
	GetBySpace(spaceID string) ([]Device, error)
	Save(device Device) error
	Delete(id string) error
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

// GetByID finds a device by ID
func (s *deviceStore) GetByID(id string) (*Device, error) {
	for i := range s.data.Devices {
		if s.data.Devices[i].ID == id {
			return &s.data.Devices[i], nil
		}
	}
	return nil, ErrRecordNotFound
}

// GetBySpace returns all devices in a specific space
func (s *deviceStore) GetBySpace(spaceID string) ([]Device, error) {
	var result []Device
	for _, d := range s.data.Devices {
		if d.SpaceID == spaceID {
			result = append(result, d)
		}
	}
	return result, nil
}

// Save saves a device (insert or update)
func (s *deviceStore) Save(device Device) error {
	for i := range s.data.Devices {
		if s.data.Devices[i].ID == device.ID {
			s.data.Devices[i] = device
			return s.save()
		}
	}
	s.data.Devices = append(s.data.Devices, device)
	return s.save()
}

// Delete deletes a device by ID
func (s *deviceStore) Delete(id string) error {
	for i := range s.data.Devices {
		if s.data.Devices[i].ID == id {
			s.data.Devices = append(s.data.Devices[:i], s.data.Devices[i+1:]...)
			return s.save()
		}
	}
	return ErrRecordNotFound
}

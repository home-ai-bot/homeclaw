// Package data provides data access layer for HomeClaw.
package data

import "sync"

// DeviceOpStore defines the interface for device operation data operations
type DeviceOpStore interface {
	GetAll() ([]DeviceOp, error)
	GetOpsByDevice(fromID, from string) ([]string, error)
	GetOpsCommand(fromID, from, ops string) (DeviceOp, error)
	Save(ops ...DeviceOp) error
	Delete(fromID, from, ops string) error
}

// deviceOpStore implements DeviceOpStore using JSONStore.
// All read operations reload from disk to ensure fresh data after gateway syncs.
type deviceOpStore struct {
	store       *JSONStore
	deviceStore DeviceStore
	mu          sync.Mutex
	data        DeviceOpsData
}

// NewDeviceOpStore creates a new DeviceOpStore
func NewDeviceOpStore(store *JSONStore, deviceStore DeviceStore) (DeviceOpStore, error) {
	s := &deviceOpStore{store: store, deviceStore: deviceStore}
	// Don't fail if file doesn't exist - just initialize with empty data
	_ = s.load()
	return s, nil
}

// load reads data from file. Caller must hold mu.
func (s *deviceOpStore) load() error {
	s.data = DeviceOpsData{Version: "1", DeviceOps: []DeviceOp{}}
	return s.store.Read("device_ops", &s.data)
}

// save writes data to file. Caller must hold mu.
func (s *deviceOpStore) save() error {
	return s.store.Write("device_ops", s.data)
}

// getOpsByDeviceLocked searches in-memory data for operations. Caller must hold mu.
func (s *deviceOpStore) getOpsByDeviceLocked(fromID, from string) []string {
	var ops []string
	if s.data.DeviceOps == nil {
		return ops
	}
	for _, op := range s.data.DeviceOps {
		if op.FromID == fromID && op.From == from {
			ops = append(ops, op.Ops)
		}
	}
	return ops
}

// GetAll returns all device operations, always reloading from disk.
func (s *deviceOpStore) GetAll() ([]DeviceOp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	if s.data.DeviceOps == nil {
		return []DeviceOp{}, nil
	}
	return s.data.DeviceOps, nil
}

// GetOpsByDevice returns all operation names for a specific device, always reloading from disk.
func (s *deviceOpStore) GetOpsByDevice(fromID, from string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	return s.getOpsByDeviceLocked(fromID, from), nil
}

// GetOpsCommand returns the command for a specific operation, always reloading from disk.
func (s *deviceOpStore) GetOpsCommand(fromID, from, ops string) (DeviceOp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	if s.data.DeviceOps == nil {
		return DeviceOp{}, ErrRecordNotFound
	}
	for _, op := range s.data.DeviceOps {
		if op.FromID == fromID && op.From == from && op.Ops == ops {
			return op, nil
		}
	}
	return DeviceOp{}, ErrRecordNotFound
}

// Save saves device operations (insert or update)
// Primary key: fromID, from, ops
func (s *deviceOpStore) Save(ops ...DeviceOp) error {
	s.mu.Lock()
	_ = s.load()
	for _, op := range ops {
		found := false
		for i := range s.data.DeviceOps {
			if s.data.DeviceOps[i].FromID == op.FromID &&
				s.data.DeviceOps[i].From == op.From &&
				s.data.DeviceOps[i].Ops == op.Ops {
				s.data.DeviceOps[i] = op
				found = true
				break
			}
		}
		if !found {
			s.data.DeviceOps = append(s.data.DeviceOps, op)
		}
	}
	saveErr := s.save()
	s.mu.Unlock()
	if saveErr != nil {
		return saveErr
	}
	// enrichDevicesWithOps calls back into GetOpsByDevice which acquires mu,
	// so it must be called outside the lock.
	return s.enrichDevicesWithOps()
}

// Delete deletes a device operation by FromID, From, and Ops
func (s *deviceOpStore) Delete(fromID, from, ops string) error {
	s.mu.Lock()
	_ = s.load()
	deleted := false
	var saveErr error
	for i := range s.data.DeviceOps {
		if s.data.DeviceOps[i].FromID == fromID &&
			s.data.DeviceOps[i].From == from &&
			s.data.DeviceOps[i].Ops == ops {
			s.data.DeviceOps = append(s.data.DeviceOps[:i], s.data.DeviceOps[i+1:]...)
			saveErr = s.save()
			deleted = true
			break
		}
	}
	s.mu.Unlock()
	if !deleted {
		return ErrRecordNotFound
	}
	if saveErr != nil {
		return saveErr
	}
	// enrichDevicesWithOps calls back into GetOpsByDevice which acquires mu,
	// so it must be called outside the lock.
	return s.enrichDevicesWithOps()
}

// enrichDevicesWithOps updates all devices with their current operations
func (s *deviceOpStore) enrichDevicesWithOps() error {
	if s.deviceStore == nil {
		return nil
	}

	// Get all devices
	devices, err := s.deviceStore.GetAll()
	if err != nil {
		return err
	}

	// Update each device with its ops
	for i := range devices {
		ops, err := s.GetOpsByDevice(devices[i].FromID, devices[i].From)
		if err != nil {
			return err
		}
		devices[i].Ops = ops
	}

	// Save updated devices
	return s.deviceStore.Save(devices...)
}

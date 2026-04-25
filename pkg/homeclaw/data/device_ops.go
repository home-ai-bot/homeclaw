// Package data provides data access layer for HomeClaw.
package data

import "sync"

// DeviceOpStore defines the interface for device operation data operations
type DeviceOpStore interface {
	GetAll() ([]DeviceOp, error)
	GetOpsByURN(urn, from string) ([]string, error)
	GetOpsCommand(urn, from, ops string) (DeviceOp, error)
	Save(ops ...DeviceOp) error
	Delete(urn, from, ops string) error
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

// getOpsByURNLocked searches in-memory data for operations by URN. Caller must hold mu.
// Skips entries with empty URN (legacy data).
func (s *deviceOpStore) getOpsByURNLocked(urn, from string) []string {
	var ops []string
	if s.data.DeviceOps == nil {
		return ops
	}
	for _, op := range s.data.DeviceOps {
		if op.URN != "" && op.URN == urn && op.From == from {
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

// GetOpsByURN returns all operation names for a specific device type (URN), always reloading from disk.
func (s *deviceOpStore) GetOpsByURN(urn, from string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	return s.getOpsByURNLocked(urn, from), nil
}

// GetOpsCommand returns the command for a specific operation, always reloading from disk.
func (s *deviceOpStore) GetOpsCommand(urn, from, ops string) (DeviceOp, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.load()
	if s.data.DeviceOps == nil {
		return DeviceOp{}, ErrRecordNotFound
	}
	for _, op := range s.data.DeviceOps {
		if op.URN == urn && op.From == from && op.Ops == ops {
			return op, nil
		}
	}
	return DeviceOp{}, ErrRecordNotFound
}

// Save saves device operations (insert or update)
// Primary key: urn, from, ops
func (s *deviceOpStore) Save(ops ...DeviceOp) error {
	s.mu.Lock()
	_ = s.load()
	for _, op := range ops {
		found := false
		for i := range s.data.DeviceOps {
			if s.data.DeviceOps[i].URN == op.URN &&
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
	// enrichDevicesWithOps calls back into GetOpsByURN which acquires mu,
	// so it must be called outside the lock.
	return s.enrichDevicesWithOps()
}

// Delete deletes a device operation by URN, From, and Ops
func (s *deviceOpStore) Delete(urn, from, ops string) error {
	s.mu.Lock()
	_ = s.load()
	deleted := false
	var saveErr error
	for i := range s.data.DeviceOps {
		if s.data.DeviceOps[i].URN == urn &&
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
	// enrichDevicesWithOps calls back into GetOpsByURN which acquires mu,
	// so it must be called outside the lock.
	return s.enrichDevicesWithOps()
}

// enrichDevicesWithOps updates all devices with their current operations.
// Optimized to O(N): builds a URN→Ops map once, then updates all devices.
// IMPORTANT: Only updates the Ops field to avoid overwriting other device fields (like URN).
func (s *deviceOpStore) enrichDevicesWithOps() error {
	if s.deviceStore == nil {
		return nil
	}

	// Build URN→Ops map from all DeviceOps
	opsMap := make(map[string][]string)
	for _, op := range s.data.DeviceOps {
		if op.URN != "" {
			key := op.URN + "|" + op.From
			opsMap[key] = append(opsMap[key], op.Ops)
		}
	}

	// Get all devices
	devices, err := s.deviceStore.GetAll()
	if err != nil {
		return err
	}

	// Only update devices whose Ops actually changed
	devicesToUpdate := make([]Device, 0)
	for _, device := range devices {
		if device.URN == "" {
			continue
		}

		key := device.URN + "|" + device.From
		newOps := opsMap[key]

		// Check if Ops actually changed to avoid unnecessary saves
		if !opsEqual(device.Ops, newOps) {
			device.Ops = newOps
			devicesToUpdate = append(devicesToUpdate, device)
		}
	}

	// Only save if there are changes
	if len(devicesToUpdate) > 0 {
		return s.deviceStore.Save(devicesToUpdate...)
	}
	return nil
}

// opsEqual checks if two ops slices are equal.
func opsEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

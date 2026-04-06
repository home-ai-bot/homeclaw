// Package data provides data access layer for HomeClaw.
package data

// DeviceOpStore defines the interface for device operation data operations
type DeviceOpStore interface {
	GetAll() ([]DeviceOp, error)
	GetOpsByDevice(fromID, from string) ([]string, error)
	GetOpsCommand(fromID, from, ops string) (string, error)
	Save(ops ...DeviceOp) error
	Delete(fromID, from, ops string) error
}

// deviceOpStore implements DeviceOpStore using JSONStore
type deviceOpStore struct {
	store *JSONStore
	data  DeviceOpsData
}

// NewDeviceOpStore creates a new DeviceOpStore
func NewDeviceOpStore(store *JSONStore) (DeviceOpStore, error) {
	s := &deviceOpStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads data from file
func (s *deviceOpStore) load() error {
	s.data = DeviceOpsData{Version: "1", DeviceOps: []DeviceOp{}}
	return s.store.Read("device_ops", &s.data)
}

// save writes data to file
func (s *deviceOpStore) save() error {
	return s.store.Write("device_ops", s.data)
}

// GetAll returns all device operations
func (s *deviceOpStore) GetAll() ([]DeviceOp, error) {
	return s.data.DeviceOps, nil
}

// GetOpsByDevice returns all operation names for a specific device
func (s *deviceOpStore) GetOpsByDevice(fromID, from string) ([]string, error) {
	var ops []string
	for _, op := range s.data.DeviceOps {
		if op.FromID == fromID && op.From == from {
			ops = append(ops, op.Ops)
		}
	}
	return ops, nil
}

// GetOpsCommand returns the command for a specific operation
func (s *deviceOpStore) GetOpsCommand(fromID, from, ops string) (string, error) {
	for _, op := range s.data.DeviceOps {
		if op.FromID == fromID && op.From == from && op.Ops == ops {
			return op.Command, nil
		}
	}
	return "", ErrRecordNotFound
}

// Save saves device operations (insert or update)
// Primary key: fromID, from, ops
func (s *deviceOpStore) Save(ops ...DeviceOp) error {
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
	return s.save()
}

// Delete deletes a device operation by FromID, From, and Ops
func (s *deviceOpStore) Delete(fromID, from, ops string) error {
	for i := range s.data.DeviceOps {
		if s.data.DeviceOps[i].FromID == fromID &&
			s.data.DeviceOps[i].From == from &&
			s.data.DeviceOps[i].Ops == ops {
			s.data.DeviceOps = append(s.data.DeviceOps[:i], s.data.DeviceOps[i+1:]...)
			return s.save()
		}
	}
	return ErrRecordNotFound
}

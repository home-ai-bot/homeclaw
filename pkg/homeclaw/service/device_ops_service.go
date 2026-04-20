// Package service provides business logic services for HomeClaw.
package service

import (
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// DeviceOpsService provides device operations querying functionality
type DeviceOpsService struct {
	deviceStore   data.DeviceStore
	deviceOpStore data.DeviceOpStore
}

// NewDeviceOpsService creates a new DeviceOpsService
func NewDeviceOpsService(deviceStore data.DeviceStore, deviceOpStore data.DeviceOpStore) *DeviceOpsService {
	return &DeviceOpsService{
		deviceStore:   deviceStore,
		deviceOpStore: deviceOpStore,
	}
}

// GetDeviceOpsCommand returns the command for a specific device operation
func (s *DeviceOpsService) GetDeviceOpsCommand(fromID, from, opsName string) (data.DeviceOp, error) {
	return s.deviceOpStore.GetOpsCommand(fromID, from, opsName)
}

// MarkDeviceAsNoAction marks a device as non-operable (e.g., IR devices, gateways)
// by setting its Ops to ["NoAction"]. This prevents the device from being
// shared with LLM for spec analysis since it cannot be controlled.
func (s *DeviceOpsService) MarkDeviceAsNoAction(fromID, from string) error {
	// Get all devices to find the target device
	devices, err := s.deviceStore.GetAll()
	if err != nil {
		return err
	}

	// Find and update the target device
	for _, device := range devices {
		if device.FromID == fromID && device.From == from {
			device.Ops = []string{"NoAction"}
			return s.deviceStore.Save(device)
		}
	}

	return fmt.Errorf("device not found: from_id=%s, from=%s", fromID, from)
}

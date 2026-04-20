// Package service provides business logic services for HomeClaw.
package service

import (
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

// Package service provides business logic services for HomeClaw.
package service

import (
	"fmt"
	"sort"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// DeviceOpsByRoom represents devices grouped by room/space
type DeviceOpsByRoom struct {
	RoomName string        `json:"room_name"`
	Devices  []data.Device `json:"devices"`
}

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

// GetAllDevicesWithOps returns all devices with their operations, grouped by room
func (s *DeviceOpsService) GetAllDevicesWithOps() ([]DeviceOpsByRoom, error) {
	// Get all devices
	devices, err := s.deviceStore.GetAll()
	if err != nil {
		return nil, err
	}

	// Enrich devices with their operations
	for i := range devices {
		ops, err := s.deviceOpStore.GetOpsByDevice(devices[i].FromID, devices[i].From)
		if err != nil {
			// Log error but continue processing
			continue
		}
		devices[i].Ops = ops
	}

	// Group devices by room - show ALL devices including those without ops
	roomMap := make(map[string]*DeviceOpsByRoom)
	for _, device := range devices {
		roomName := device.SpaceName
		if roomName == "" {
			roomName = "Unassigned"
		}

		if _, exists := roomMap[roomName]; !exists {
			roomMap[roomName] = &DeviceOpsByRoom{
				RoomName: roomName,
				Devices:  []data.Device{},
			}
		}

		roomMap[roomName].Devices = append(roomMap[roomName].Devices, device)
	}

	// Convert map to slice and sort by room name for stable ordering
	result := make([]DeviceOpsByRoom, 0, len(roomMap))
	for _, room := range roomMap {
		result = append(result, *room)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].RoomName < result[j].RoomName
	})

	return result, nil
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

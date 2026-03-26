// Package data provides data access layer for HomeClaw.
package data

import (
	rootdata "github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// DeviceInfo 设备信息
type DeviceInfo struct {
	DID        string                 `json:"did"`
	UID        any                    `json:"uid"`
	Name       string                 `json:"name"`
	Model      string                 `json:"model"`
	Token      string                 `json:"token"`
	LocalIP    string                 `json:"localip"`
	MAC        string                 `json:"mac"`
	SSID       string                 `json:"ssid"`
	BSSID      string                 `json:"bssid"`
	RSSI       any                    `json:"rssi"`
	PID        any                    `json:"pid"`
	ParentID   string                 `json:"parent_id"`
	IsOnline   bool                   `json:"isOnline"`
	SpecType   string                 `json:"spec_type"`
	VoiceCtrl  any                    `json:"voice_ctrl"`
	OrderTime  int64                  `json:"orderTime"`
	Extra      map[string]any         `json:"extra"`
	SubDevices map[string]*DeviceInfo `json:"sub_devices,omitempty"`

	// 家庭/房间信息（由 GetDevices 填充）
	HomeID   string `json:"home_id,omitempty"`
	HomeName string `json:"home_name,omitempty"`
	RoomID   string `json:"room_id,omitempty"`
	RoomName string `json:"room_name,omitempty"`
	GroupID  string `json:"group_id,omitempty"`
}

// MiDevicesData is the root structure for mi-devices.json
type MiDevicesData struct {
	Version string        `json:"version"`
	Devices []*DeviceInfo `json:"devices"`
}

// MiDeviceStore defines the interface for MiDevice data operations
type MiDeviceStore interface {
	GetAll() ([]*DeviceInfo, error)
	GetByDID(did string) (*DeviceInfo, error)
	Save(device *DeviceInfo) error
	Delete(did string) error
}

// miDeviceStore implements MiDeviceStore using JSONStore
type miDeviceStore struct {
	store *rootdata.JSONStore
	data  MiDevicesData
}

// NewMiDeviceStore creates a new MiDeviceStore
func NewMiDeviceStore(store *rootdata.JSONStore) (MiDeviceStore, error) {
	s := &miDeviceStore{store: store}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

// load reads data from file
func (s *miDeviceStore) load() error {
	s.data = MiDevicesData{Version: "1", Devices: []*DeviceInfo{}}
	return s.store.Read("mi-devices", &s.data)
}

// save writes data to file
func (s *miDeviceStore) save() error {
	return s.store.Write("mi-devices", s.data)
}

// GetAll returns all mi devices
func (s *miDeviceStore) GetAll() ([]*DeviceInfo, error) {
	return s.data.Devices, nil
}

// GetByDID returns a device by DID
func (s *miDeviceStore) GetByDID(did string) (*DeviceInfo, error) {
	for _, d := range s.data.Devices {
		if d.DID == did {
			return d, nil
		}
	}
	return nil, rootdata.ErrRecordNotFound
}

// Save saves a device (insert or update by DID)
func (s *miDeviceStore) Save(device *DeviceInfo) error {
	for i, d := range s.data.Devices {
		if d.DID == device.DID {
			s.data.Devices[i] = device
			return s.save()
		}
	}
	s.data.Devices = append(s.data.Devices, device)
	return s.save()
}

// Delete deletes a device by DID
func (s *miDeviceStore) Delete(did string) error {
	for i, d := range s.data.Devices {
		if d.DID == did {
			s.data.Devices = append(s.data.Devices[:i], s.data.Devices[i+1:]...)
			return s.save()
		}
	}
	return rootdata.ErrRecordNotFound
}

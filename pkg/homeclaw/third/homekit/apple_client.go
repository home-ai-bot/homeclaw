// Package homekit provides HomeKit device management for HomeClaw.
package homekit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third"
)

const (
	// BrandHomeKit is the brand identifier for HomeKit platform.
	BrandHomeKit = "homekit"

	// HomeKit spec cache configuration
	homekitSpecCacheDir = "homekit-spec"
	homekitSpecTTL      = -1
)

// HomeKitClient handles communication with HomeKit devices
type HomeKitClient struct {
	deviceStore data.DeviceStore
	specCache   *data.FileCache // Cache for device specs
}

// NewHomeKitClient creates a new HomeKitClient instance
func NewHomeKitClient(deviceStore data.DeviceStore, workspace string) *HomeKitClient {
	client := &HomeKitClient{
		deviceStore: deviceStore,
	}

	// Initialize spec cache if workspace is provided
	if workspace != "" {
		cacheDir := filepath.Join(workspace, "third", homekitSpecCacheDir)
		specCache, err := data.NewFileCache(data.FileCacheConfig{
			CacheDir:  cacheDir,
			TTL:       homekitSpecTTL,
			EncodeKey: true, // Device IDs contain colons
		})
		if err == nil {
			client.specCache = specCache
		}
	}

	return client
}

// Brand returns the brand identifier.
func (c *HomeKitClient) Brand() string {
	return BrandHomeKit
}

// GetHomes returns all homes visible to the authenticated user.
// HomeKit doesn't have a home concept like cloud platforms.
func (c *HomeKitClient) GetHomes() ([]*third.HomeInfo, error) {
	return []*third.HomeInfo{}, nil
}

// GetRooms returns all rooms for the given homeID.
// HomeKit doesn't provide a direct room listing API.
func (c *HomeKitClient) GetRooms(homeID string) ([]*data.Space, error) {
	return []*data.Space{}, nil
}

// GetDevices returns all devices for the given homeID.
// For HomeKit, this returns devices from the DeviceStore.
func (c *HomeKitClient) GetDevices(homeID string) ([]*data.Device, error) {
	return []*data.Device{}, nil
}

// GetSpec fetches the capability specification for deviceID.
// HomeKit uses characteristic types instead of MIoT specs.
func (c *HomeKitClient) GetSpec(deviceID string) (*third.SpecInfo, error) {
	// 1. Try file cache first (populated during PairDevice)
	if c.specCache != nil {
		if cached, err := c.specCache.GetAsString(deviceID); err == nil && cached != "" {
			var cachedData map[string]any
			if json.Unmarshal([]byte(cached), &cachedData) == nil {
				return &third.SpecInfo{
					DeviceID: deviceID,
					Model:    fmt.Sprintf("%v", cachedData["model"]),
					Raw:      "",
					Extra:    cachedData["extra"].(map[string]any),
				}, nil
			}
		}
	}

	// 2. Find device in store
	targetDevice, err := c.findDevice(deviceID)
	if err != nil {
		return nil, err
	}

	// 3. Connect to device
	hapClient, err := c.connectToDevice(targetDevice)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to device: %w", err)
	}
	defer hapClient.Close()

	// 4. Fetch accessories
	accessories, err := hapClient.GetAccessories()
	if err != nil {
		return nil, fmt.Errorf("failed to get accessories: %w", err)
	}

	// 5. Build and return SpecInfo
	return c.buildSpecInfo(deviceID, accessories), nil
}

// connectToDevice creates a HAP client connection to the device
func (c *HomeKitClient) connectToDevice(device *data.Device) (*hap.Client, error) {
	creds, err := parsePairingToken(device.Token)
	if err != nil {
		return nil, fmt.Errorf("failed to parse pairing token: %w", err)
	}

	rawURL := fmt.Sprintf(
		"homekit://%s?device_id=%s&client_id=%s&client_private=%s&device_public=%s",
		device.IP,
		device.FromID,
		creds["client_id"],
		creds["client_private"],
		creds["device_public"],
	)

	return hap.Pair(rawURL)
}

// buildSpecInfo constructs SpecInfo with operation instructions from accessories
func (c *HomeKitClient) buildSpecInfo(deviceID string, accessories []*hap.Accessory) *third.SpecInfo {
	// Extract operations from accessories
	operations := c.extractOperations(deviceID, accessories)

	return &third.SpecInfo{
		DeviceID: deviceID,
		Model:    c.guessModel(accessories),
		Raw:      "", // No longer storing raw HAP JSON
		Extra: map[string]any{
			"platform":   BrandHomeKit,
			"operations": operations,
		},
	}
}

// extractOperations parses accessories to build operation guide
func (c *HomeKitClient) extractOperations(deviceID string, accessories []*hap.Accessory) []map[string]any {
	var operations []map[string]any

	for _, acc := range accessories {
		for _, service := range acc.Services {
			serviceName := c.getServiceName(service.Type)

			for _, char := range service.Characters {
				charName := c.getCharacteristicName(char.Type)
				canRead := c.hasPerm(char.Perms, "pr")
				canWrite := c.hasPerm(char.Perms, "pw")

				// Add read operation
				if canRead {
					operations = append(operations, map[string]any{
						"type":           "getProps",
						"description":    fmt.Sprintf("Read %s %s", serviceName, charName),
						"service":        serviceName,
						"characteristic": charName,
						"params": map[string]any{
							"device_id": deviceID,
							"aid":       acc.AID,
							"iid":       char.IID,
						},
					})
				}

				// Add write operation
				if canWrite {
					op := map[string]any{
						"type":           "setProps",
						"description":    fmt.Sprintf("Set %s %s", serviceName, charName),
						"service":        serviceName,
						"characteristic": charName,
						"format":         char.Format,
						"params": map[string]any{
							"device_id": deviceID,
							"aid":       acc.AID,
							"iid":       char.IID,
							"value":     c.getDefaultValue(char.Format),
						},
					}

					// Add value constraints if available
					if char.Format == "uint8" || char.Format == "float" {
						// Could add min/max/step if needed
					}

					operations = append(operations, op)
				}
			}
		}
	}

	return operations
}

// getServiceName maps HomeKit service type to human-readable name
func (c *HomeKitClient) getServiceName(typeID string) string {
	names := map[string]string{
		"3E": "Accessory Information",
		"40": "Fan",
		"43": "Lightbulb",
		"47": "Outlet",
		"49": "Switch",
		"4A": "Thermostat",
		"7E": "Security System",
		"80": "Contact Sensor",
		"82": "Humidity Sensor",
		"85": "Motion Sensor",
		"8A": "Temperature Sensor",
		"8D": "Air Quality Sensor",
		"96": "Battery Service",
	}
	if name, ok := names[typeID]; ok {
		return name
	}
	return "Service(" + typeID + ")"
}

// getCharacteristicName maps HomeKit characteristic type to human-readable name
func (c *HomeKitClient) getCharacteristicName(typeID string) string {
	names := map[string]string{
		"08": "Brightness",
		"0E": "Configured Name",
		"14": "Identify",
		"23": "Name",
		"25": "On",
		"2F": "Saturation",
		"30": "Serial Number",
		"37": "Version",
		"52": "Firmware Revision",
		"B0": "Active",
		"B3": "Current Temperature",
		"B4": "Target Temperature",
		"B5": "Temperature Display Units",
		"B6": "Cooling Threshold Temperature",
		"B7": "Heating Threshold Temperature",
		"B8": "Relative Humidity Dehumidifier Threshold",
		"B9": "Current Relative Humidity",
		"BA": "Target Relative Humidity",
		"BF": "Rotation Speed",
		"D8": "Current Horizontal Tilt Angle",
		"D9": "Target Horizontal Tilt Angle",
		"DC": "Current Vertical Tilt Angle",
		"DD": "Target Vertical Tilt Angle",
		"E7": "Active Identifier",
		"E8": "In Use",
		"E9": "Is Configured",
		"EA": "Remaining Duration",
		"EB": "Set Duration",
	}
	if name, ok := names[typeID]; ok {
		return name
	}
	return "Characteristic(" + typeID + ")"
}

// hasPerm checks if permissions list contains a specific permission
func (c *HomeKitClient) hasPerm(perms []string, perm string) bool {
	for _, p := range perms {
		if p == perm {
			return true
		}
	}
	return false
}

// getDefaultValue returns a default value for a characteristic format
func (c *HomeKitClient) getDefaultValue(format string) any {
	switch format {
	case "bool":
		return false
	case "uint8", "uint16", "uint32", "int32":
		return 0
	case "float":
		return 0.0
	case "string":
		return ""
	default:
		return nil
	}
}

// cacheSpecFromAccessories caches spec data from accessories to avoid re-reading
func (c *HomeKitClient) cacheSpecFromAccessories(deviceID string, accessories []*hap.Accessory) {
	if c.specCache == nil || accessories == nil {
		return
	}

	specInfo := c.buildSpecInfo(deviceID, accessories)
	cacheData := map[string]any{
		"model": specInfo.Model,
		"extra": specInfo.Extra,
	}
	if cacheJSON, err := json.Marshal(cacheData); err == nil {
		_ = c.specCache.SetString(deviceID, string(cacheJSON))
	}
}

// guessModel extracts model name from accessories
func (c *HomeKitClient) guessModel(accessories []*hap.Accessory) string {
	for _, acc := range accessories {
		for _, service := range acc.Services {
			if service.Type == "43" {
				return "Lightbulb"
			} else if service.Type == "49" {
				return "Switch"
			} else if service.Type == "47" {
				return "Outlet"
			}
		}
	}
	return "HomeKit Device"
}

// Execute sends an action command to a device.
// For HomeKit, this writes to a characteristic.
func (c *HomeKitClient) Execute(params map[string]any) (map[string]any, error) {
	deviceID, ok := params["device_id"].(string)
	if !ok || deviceID == "" {
		return nil, fmt.Errorf("device_id is required")
	}

	targetDevice, err := c.findDevice(deviceID)
	if err != nil {
		return nil, err
	}

	hapClient, err := c.connectToDevice(targetDevice)
	if err != nil {
		return nil, err
	}
	defer hapClient.Close()

	aid, _ := params["aid"].(float64)
	iid, _ := params["iid"].(float64)
	if aid == 0 || iid == 0 {
		return nil, fmt.Errorf("aid and iid are required")
	}

	value := params["value"]
	charData := map[string]any{
		"aid":   int(aid),
		"iid":   uint64(iid),
		"value": value,
	}

	bodyBytes, _ := json.Marshal(map[string]any{
		"characteristics": []map[string]any{charData},
	})

	if _, err := hapClient.Put(hap.PathCharacteristics, hap.MimeJSON, bytes.NewReader(bodyBytes)); err != nil {
		return nil, fmt.Errorf("failed to execute action: %w", err)
	}

	return map[string]any{"success": true}, nil
}

// GetProps reads property values from a device.
func (c *HomeKitClient) GetProps(params map[string]any) (any, error) {
	deviceID, ok := params["device_id"].(string)
	if !ok || deviceID == "" {
		return nil, fmt.Errorf("device_id is required")
	}

	targetDevice, err := c.findDevice(deviceID)
	if err != nil {
		return nil, err
	}

	hapClient, err := c.connectToDevice(targetDevice)
	if err != nil {
		return nil, err
	}
	defer hapClient.Close()

	aid, hasAid := params["aid"].(float64)
	iid, hasIid := params["iid"].(float64)

	if hasAid && hasIid {
		query := fmt.Sprintf("%d.%d", int(aid), int(iid))
		chars, err := hapClient.GetCharacters(query)
		if err != nil {
			return nil, fmt.Errorf("failed to read characteristic: %w", err)
		}
		return chars, nil
	}

	// Read all accessories
	accessories, err := hapClient.GetAccessories()
	if err != nil {
		return nil, fmt.Errorf("failed to get accessories: %w", err)
	}

	return accessories, nil
}

// SetProps writes property values to a device.
func (c *HomeKitClient) SetProps(params map[string]any) (any, error) {
	deviceID, ok := params["device_id"].(string)
	if !ok || deviceID == "" {
		return nil, fmt.Errorf("device_id is required")
	}

	targetDevice, err := c.findDevice(deviceID)
	if err != nil {
		return nil, err
	}

	hapClient, err := c.connectToDevice(targetDevice)
	if err != nil {
		return nil, err
	}
	defer hapClient.Close()

	aid, _ := params["aid"].(float64)
	iid, _ := params["iid"].(float64)
	if aid == 0 || iid == 0 {
		return nil, fmt.Errorf("aid and iid are required")
	}

	value, ok := params["value"]
	if !ok {
		return nil, fmt.Errorf("value is required")
	}

	charData := map[string]any{
		"aid":   int(aid),
		"iid":   uint64(iid),
		"value": value,
	}

	bodyBytes, _ := json.Marshal(map[string]any{
		"characteristics": []map[string]any{charData},
	})

	if _, err := hapClient.Put(hap.PathCharacteristics, hap.MimeJSON, bytes.NewReader(bodyBytes)); err != nil {
		return nil, fmt.Errorf("failed to write characteristic: %w", err)
	}

	return map[string]any{"success": true}, nil
}

// EnableEvent starts event subscription for a device.
// HomeKit supports event notifications for characteristics.
func (c *HomeKitClient) EnableEvent(params map[string]any) error {
	deviceID, ok := params["device_id"].(string)
	if !ok || deviceID == "" {
		return fmt.Errorf("device_id is required")
	}

	_, err := c.findDevice(deviceID)
	return err
}

// DisableEvent stops event subscription for a device.
// Idempotent: returns nil even if device is not subscribed.
func (c *HomeKitClient) DisableEvent(params map[string]any) error {
	return nil
}

// GetRtspStr returns the RTSP stream URL for a device.
// HomeKit cameras use SRTP, not direct RTSP URLs.
func (c *HomeKitClient) GetRtspStr(deviceID string) (string, error) {
	return "", nil
}

// GetDeviceStore returns the DeviceStore instance
func (c *HomeKitClient) GetDeviceStore() data.DeviceStore {
	return c.deviceStore
}

// PairDevice performs HomeKit pairing with a device, saves it to DeviceStore, and returns the device
func (c *HomeKitClient) PairDevice(ip string, port uint16, deviceID string, pin string) (*data.Device, error) {
	// Construct the HomeKit URL
	rawURL := fmt.Sprintf("homekit://%s:%d?device_id=%s&pin=%s", ip, port, deviceID, pin)

	// Perform pairing using HAP protocol
	hapClient, err := hap.Pair(rawURL)
	if err != nil {
		return nil, fmt.Errorf("pairing failed: %w", err)
	}
	defer hapClient.Close()

	// Fetch accessories to get device name
	accessories, err := hapClient.GetAccessories()
	if err != nil {
		// If we can't get accessories, use deviceID as fallback
		accessories = nil
	}

	// Extract device name from accessories
	deviceName := c.extractDeviceName(deviceID, accessories)

	// Cache spec data from accessories (avoid re-reading in GetSpec)
	c.cacheSpecFromAccessories(deviceID, accessories)

	// Parse the URL to extract query parameters (client credentials)
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	query := u.Query()

	// Create device record with all necessary pairing credentials
	device := &data.Device{
		FromID: deviceID,
		From:   "homekit",
		Name:   deviceName,
		Type:   "homekit",
		IP:     ip,
		// Store critical pairing information in Token field
		// This includes client_id, client_private, device_public needed for future connections
		Token: fmt.Sprintf(
			"client_id=%s&client_private=%s&device_public=%s",
			query.Get("client_id"),
			query.Get("client_private"),
			query.Get("device_public"),
		),
	}

	// Save device to DeviceStore
	if c.deviceStore != nil {
		if err := c.deviceStore.Save(*device); err != nil {
			return nil, fmt.Errorf("failed to save device: %w", err)
		}
	}

	return device, nil
}

// UnpairDevice removes pairing from a HomeKit device and deletes it from DeviceStore
func (c *HomeKitClient) UnpairDevice(deviceID string) error {
	if c.deviceStore == nil {
		return fmt.Errorf("deviceStore is nil")
	}

	// Get device from DeviceStore
	devices, err := c.deviceStore.GetAll()
	if err != nil {
		return fmt.Errorf("failed to get devices: %w", err)
	}

	// Find the device
	var targetDevice *data.Device
	for _, device := range devices {
		if device.FromID == deviceID && device.From == "homekit" {
			targetDevice = &device
			break
		}
	}

	if targetDevice == nil {
		return fmt.Errorf("device_not_in_store: This device was not paired through HomeClaw. Please remove the pairing from the place where you originally added it")
	}

	// Parse the token to extract pairing credentials
	creds, err := parsePairingToken(targetDevice.Token)
	if err != nil {
		return fmt.Errorf("failed to parse pairing token: %w", err)
	}

	// Construct the URL with pairing credentials
	rawURL := fmt.Sprintf(
		"homekit://%s?device_id=%s&client_id=%s&client_private=%s&device_public=%s",
		targetDevice.IP,
		targetDevice.FromID,
		creds["client_id"],
		creds["client_private"],
		creds["device_public"],
	)

	// Perform unpairing
	if err := hap.Unpair(rawURL); err != nil {
		return fmt.Errorf("unpairing failed: %w", err)
	}

	// Delete device from DeviceStore
	if err := c.deviceStore.Delete(deviceID, "homekit"); err != nil {
		return fmt.Errorf("failed to delete device from store: %w", err)
	}

	return nil
}

// extractDeviceName extracts device name from accessories
func (c *HomeKitClient) extractDeviceName(deviceID string, accessories []*hap.Accessory) string {
	if accessories == nil {
		return deviceID
	}

	// Look for the Accessory Information service (type 3E) and Name characteristic (type 23)
	for _, acc := range accessories {
		for _, service := range acc.Services {
			if service.Type == "3E" { // Accessory Information service
				for _, char := range service.Characters {
					if char.Type == "23" && char.Value != nil { // Name characteristic
						if name, ok := char.Value.(string); ok && name != "" {
							return name
						}
					}
				}
			}
		}
	}

	return deviceID
}

// findDevice finds a HomeKit device by ID from DeviceStore.
func (c *HomeKitClient) findDevice(deviceID string) (*data.Device, error) {
	if c.deviceStore == nil {
		return nil, fmt.Errorf("deviceStore is not initialized")
	}

	devices, err := c.deviceStore.GetAll()
	if err != nil {
		return nil, fmt.Errorf("failed to get devices: %w", err)
	}

	for i := range devices {
		if devices[i].FromID == deviceID && devices[i].From == BrandHomeKit {
			return &devices[i], nil
		}
	}

	return nil, fmt.Errorf("device not found: %s", deviceID)
}

// parsePairingToken parses the pairing token string into a map
func parsePairingToken(token string) (map[string]string, error) {
	result := make(map[string]string)

	// Token format: client_id=xxx&client_private=yyy&device_public=zzz
	values, err := url.ParseQuery(token)
	if err != nil {
		return nil, err
	}

	for key, vals := range values {
		if len(vals) > 0 {
			result[key] = vals[0]
		}
	}

	// Validate required fields
	requiredFields := []string{"client_id", "client_private", "device_public"}
	for _, field := range requiredFields {
		if _, ok := result[field]; !ok {
			return nil, fmt.Errorf("missing required field: %s", field)
		}
	}

	return result, nil
}

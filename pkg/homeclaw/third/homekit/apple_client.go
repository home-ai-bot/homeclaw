// Package homekit provides HomeKit device management for HomeClaw.
package homekit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"

	"github.com/AlexxIT/go2rtc/pkg/hap"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
)

// HomeKitClient handles communication with HomeKit devices
type HomeKitClient struct {
	deviceStore data.DeviceStore
}

// NewHomeKitClient creates a new HomeKitClient instance
func NewHomeKitClient(deviceStore data.DeviceStore) *HomeKitClient {
	return &HomeKitClient{
		deviceStore: deviceStore,
	}
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

	// Get device name from accessory information
	deviceName, err := GetDeviceName(hapClient, deviceID)
	if err != nil {
		// If we can't get the name, use deviceID as fallback
		deviceName = deviceID
	}

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

// GetDeviceName retrieves the device name from a HomeKit accessory
func GetDeviceName(client *hap.Client, fallbackName string) (string, error) {
	if client == nil {
		return fallbackName, fmt.Errorf("client is nil")
	}

	// Request accessory information from the device
	resp, err := client.Get(hap.PathAccessories)
	if err != nil {
		return fallbackName, fmt.Errorf("failed to get accessories: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fallbackName, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse JSON response to extract device name
	var result struct {
		Accessories []struct {
			Services []struct {
				Type            string `json:"type"`
				Characteristics []struct {
					Type  string      `json:"type"`
					Value interface{} `json:"value"`
				} `json:"characteristics"`
			} `json:"services"`
		} `json:"accessories"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return fallbackName, fmt.Errorf("failed to parse accessories: %w", err)
	}

	// Look for the Accessory Information service (type 3E) and Name characteristic (type 23)
	if len(result.Accessories) > 0 {
		for _, service := range result.Accessories[0].Services {
			if service.Type == "3E" { // Accessory Information service
				for _, char := range service.Characteristics {
					if char.Type == "23" && char.Value != nil { // Name characteristic
						if name, ok := char.Value.(string); ok && name != "" {
							return name, nil
						}
					}
				}
			}
		}
	}

	return fallbackName, nil
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

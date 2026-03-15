// Package tool provides HomeClaw LLM tools for Mi Home device token extraction.
// This file contains the hc_mijia_extract_tokens tool which extracts device tokens
// from Xiaomi cloud using username/password authentication.
//
// Based on: https://github.com/PiotrMachowski/Xiaomi-cloud-tokens-extractor
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/miio"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// MijiaTokenExtractorTool extracts device tokens from Xiaomi cloud.
// It logs in with username/password, fetches homes and devices from all regions,
// and saves discovered devices to the device store.
type MijiaTokenExtractorTool struct {
	store data.DeviceStore
}

// NewMijiaTokenExtractorTool creates a MijiaTokenExtractorTool.
func NewMijiaTokenExtractorTool(store data.DeviceStore) *MijiaTokenExtractorTool {
	return &MijiaTokenExtractorTool{store: store}
}

func (t *MijiaTokenExtractorTool) Name() string { return "hc_mijia_extract_tokens" }

func (t *MijiaTokenExtractorTool) Description() string {
	return "Extract device tokens from Xiaomi Mi Home cloud using username and password. " +
		"This tool logs into the Xiaomi cloud, discovers all homes and devices across all server regions " +
		"(cn, de, us, ru, tw, sg, in, i2), extracts device tokens, and saves them to the HomeClaw device store. " +
		"For BLE devices, it also retrieves beacon keys.\n\n" +
		"Note: If 2FA is enabled on the account, use hc_mijia_login instead."
}

func (t *MijiaTokenExtractorTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"username": map[string]any{
				"type":        "string",
				"description": "Mi Home account username (email or phone number)",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "Mi Home account password",
			},
			"server": map[string]any{
				"type":        "string",
				"description": "Server region to check (cn, de, us, ru, tw, sg, in, i2). Leave empty to check all servers.",
				"enum":        []string{"", "cn", "de", "us", "ru", "tw", "sg", "in", "i2"},
			},
		},
		"required": []string{"username", "password"},
	}
}

// ExtractedDevice represents a device with its token and optional BLE key.
type ExtractedDevice struct {
	Name     string `json:"name"`
	Did      string `json:"did"`
	Token    string `json:"token"`
	Model    string `json:"model"`
	IP       string `json:"ip"`
	MAC      string `json:"mac"`
	Server   string `json:"server"`
	HomeID   int64  `json:"home_id"`
	BLEKey   string `json:"ble_key,omitempty"`
	IsOnline bool   `json:"is_online"`
}

// ExtractionResult holds the complete extraction results.
type ExtractionResult struct {
	Servers   map[string]*ServerResult `json:"servers"`
	Total     int                      `json:"total_devices"`
	Saved     int                      `json:"saved_to_store"`
	Skipped   int                      `json:"skipped_no_token"`
	Errors    []string                 `json:"errors,omitempty"`
	Timestamp time.Time                `json:"timestamp"`
}

// ServerResult holds results for a single server region.
type ServerResult struct {
	Homes    []HomeResult `json:"homes"`
	Devices  int          `json:"device_count"`
	HasError bool         `json:"has_error,omitempty"`
	Error    string       `json:"error,omitempty"`
}

// HomeResult holds results for a single home.
type HomeResult struct {
	HomeID   int64             `json:"home_id"`
	HomeName string            `json:"home_name"`
	Devices  []ExtractedDevice `json:"devices"`
}

func (t *MijiaTokenExtractorTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	username, _ := args["username"].(string)
	password, _ := args["password"].(string)
	server, _ := args["server"].(string)

	if username == "" || password == "" {
		return &tools.ToolResult{ForLLM: "username and password are required", IsError: true}
	}

	// Determine which servers to check
	var serversToCheck []miio.ServerRegion
	if server != "" {
		serversToCheck = []miio.ServerRegion{miio.ServerRegion(server)}
	} else {
		serversToCheck = miio.AllServerRegions()
	}

	// First, login to the main server (cn) to get credentials
	client := miio.NewCloudClient("cn")
	res, err := client.LoginWithResult(username, password)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("Mi Home login failed: %v", err), IsError: true}
	}

	if res.NeedVerify {
		return &tools.ToolResult{
			ForLLM: fmt.Sprintf(
				"Login requires 2FA verification. Please use hc_mijia_login instead.\n"+
					"Verification URL: %s", res.NotifyURL),
			IsError: true,
		}
	}

	result := &ExtractionResult{
		Servers:   make(map[string]*ServerResult),
		Timestamp: time.Now(),
	}

	// Extract session for reuse across servers
	session := client.ExportSession()

	// Check each server
	for _, srv := range serversToCheck {
		srvResult := t.extractFromServer(srv, session, username, password)
		result.Servers[srv.String()] = srvResult
		result.Total += srvResult.Devices
	}

	// Save devices to store
	saved, skipped, saveErrors := t.saveDevicesToStore(result)
	result.Saved = saved
	result.Skipped = skipped
	result.Errors = append(result.Errors, saveErrors...)

	// Format result for LLM
	output, _ := json.MarshalIndent(result, "", "  ")
	return tools.NewToolResult(string(output))
}

func (t *MijiaTokenExtractorTool) extractFromServer(
	server miio.ServerRegion,
	session miio.Session,
	username, password string,
) *ServerResult {
	srvResult := &ServerResult{
		Homes: []HomeResult{},
	}

	// Create client for this server
	client := miio.NewCloudClient(server.String())

	// Try to use existing session, fallback to fresh login
	client.ImportSession(session)

	// Verify session works by getting homes
	homes, err := client.GetHomes()
	if err != nil {
		// Session might be server-specific, try fresh login
		res, loginErr := client.LoginWithResult(username, password)
		if loginErr != nil {
			srvResult.HasError = true
			srvResult.Error = fmt.Sprintf("login failed: %v", loginErr)
			return srvResult
		}
		if res.NeedVerify {
			srvResult.HasError = true
			srvResult.Error = "2FA required"
			return srvResult
		}
		// Retry getting homes
		homes, err = client.GetHomes()
		if err != nil {
			srvResult.HasError = true
			srvResult.Error = fmt.Sprintf("get homes failed: %v", err)
			return srvResult
		}
	}

	// Build list of homes to check
	type homeRef struct {
		id    int64
		owner int64
		name  string
	}
	var homesToCheck []homeRef

	// Add user's own homes
	for _, h := range homes {
		// Use client.UserID as owner if OwnerID is not set
		ownerID := h.OwnerID
		if ownerID == 0 {
			// Parse UserID from client
			ownerID = parseUserID(client.UserID)
		}
		homesToCheck = append(homesToCheck, homeRef{
			id:    h.ID,
			owner: ownerID,
			name:  h.Name,
		})
	}

	// Get shared families from device count API
	devCount, err := client.GetDeviceCount()
	if err == nil && devCount != nil {
		for _, sf := range devCount.Share.ShareFamilies {
			// Check if not already in list
			found := false
			for _, existing := range homesToCheck {
				if existing.id == sf.HomeID {
					found = true
					break
				}
			}
			if !found {
				homesToCheck = append(homesToCheck, homeRef{
					id:    sf.HomeID,
					owner: sf.HomeOwner,
					name:  fmt.Sprintf("Shared Home %d", sf.HomeID),
				})
			}
		}
	}

	// Extract devices from each home
	for _, home := range homesToCheck {
		homeResult := HomeResult{
			HomeID:   home.id,
			HomeName: home.name,
			Devices:  []ExtractedDevice{},
		}

		devices, err := client.GetHomeDevices(home.id, home.owner)
		if err != nil {
			// Log error but continue with other homes
			continue
		}

		for _, dev := range devices {
			extracted := ExtractedDevice{
				Name:     dev.Name,
				Did:      dev.Did,
				Token:    dev.Token,
				Model:    dev.Model,
				IP:       dev.IP,
				MAC:      dev.MAC,
				Server:   server.String(),
				HomeID:   home.id,
				IsOnline: dev.IsOnline,
			}

			// For BLE devices, try to get beacon key
			if strings.HasPrefix(dev.Did, "blt") {
				if beaconKey, err := client.GetBeaconKey(dev.Did); err == nil && beaconKey != nil {
					extracted.BLEKey = beaconKey.BeaconKey
				}
			}

			homeResult.Devices = append(homeResult.Devices, extracted)
		}

		srvResult.Homes = append(srvResult.Homes, homeResult)
		srvResult.Devices += len(devices)
	}

	return srvResult
}

// parseUserID parses user ID string to int64.
func parseUserID(userID string) int64 {
	var id int64
	fmt.Sscanf(userID, "%d", &id)
	return id
}

func (t *MijiaTokenExtractorTool) saveDevicesToStore(result *ExtractionResult) (saved, skipped int, errors []string) {
	now := time.Now()

	for _, srvResult := range result.Servers {
		for _, home := range srvResult.Homes {
			for _, dev := range home.Devices {
				if dev.Token == "" {
					skipped++
					continue
				}

				device := data.Device{
					ID:           dev.Did,
					Name:         dev.Name,
					Brand:        "mijia",
					Protocol:     "miio",
					Model:        dev.Model,
					IP:           dev.IP,
					Token:        dev.Token,
					Capabilities: []string{},
					State: map[string]interface{}{
						"server":    dev.Server,
						"home_id":   dev.HomeID,
						"mac":       dev.MAC,
						"is_online": dev.IsOnline,
					},
					AddedAt:  now,
					LastSeen: now,
				}

				// Add BLE key to state if present
				if dev.BLEKey != "" {
					device.State["ble_key"] = dev.BLEKey
				}

				if err := t.store.Save(device); err != nil {
					errors = append(errors, fmt.Sprintf("failed to save device %s: %v", dev.Did, err))
					skipped++
				} else {
					saved++
				}
			}
		}
	}

	return saved, skipped, errors
}

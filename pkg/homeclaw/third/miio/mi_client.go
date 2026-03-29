// Package miio provides a Xiaomi MIoT cloud client implementation.
package miio

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/AlexxIT/go2rtc/pkg/xiaomi"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	midata "github.com/sipeed/picoclaw/pkg/homeclaw/third/miio/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/std"
)

const (
	// BrandXiaomi is the brand identifier for Xiaomi/Mi Home platform.
	BrandXiaomi = "xiaomi"

	// API endpoints
	apiHomeRoomList   = "/homeroom/gethome"
	apiHomeDeviceList = "/v2/home/device_list_page"
	apiMiotspecProp   = "/miotspec/prop/get"
	apiMiotspecSet    = "/miotspec/prop/set"
	apiMiotspecAct    = "/miotspec/action"

	// Pagination settings
	homeDeviceLimit = 300
)

// getBaseURL returns the API base URL for the given country/region.
// For CN, it returns "https://api.io.mi.com/app"
// For other regions, it returns "https://{country}.api.io.mi.com/app"
func getBaseURL(country string) string {
	if country == "" || country == "cn" {
		return "https://api.io.mi.com/app"
	}
	return fmt.Sprintf("https://%s.api.io.mi.com/app", country)
}

// MiClient implements std.Client for Xiaomi/Mi Home platform.
type MiClient struct {
	cloud       *xiaomi.Cloud
	specFetcher *SpecFetcher
	deviceStore midata.MiDeviceStore
	baseURL     string
	country     string // region code (cn, de, ru, sg, i2, us, etc.)

	// in-memory cache for quick access
	deviceCache map[string]*midata.DeviceInfo // deviceID -> DeviceInfo
}

// NewMiClient creates a new MiClient instance.
//
// Parameters:
//   - cloud: authenticated xiaomi.Cloud instance
//   - country: region code (cn, de, ru, sg, i2, us, etc.)
//   - workspace: data root directory for caching
//   - deviceStore: optional MiDeviceStore for persisting device info (can be nil)
func NewMiClient(cloud *xiaomi.Cloud, country, workspace string, deviceStore midata.MiDeviceStore) *MiClient {
	if country == "" {
		country = "cn"
	}
	return &MiClient{
		cloud:       cloud,
		specFetcher: NewSpecFetcher(workspace),
		deviceStore: deviceStore,
		baseURL:     getBaseURL(country),
		country:     country,
		deviceCache: make(map[string]*midata.DeviceInfo),
	}
}

// GetUserAndRegion returns the authenticated user ID and region code.
func (c *MiClient) GetUserAndRegion() (userID string, region string) {
	userID, _ = c.cloud.UserToken()
	return userID, c.country
}

// Brand returns the brand identifier for Xiaomi platform.
func (c *MiClient) Brand() string {
	return BrandXiaomi
}

// ────────────────────────────────────────────────────────────────────────────────
// Query methods
// ────────────────────────────────────────────────────────────────────────────────

// homeRoomResponse represents the response structure from homeroom API.
type homeRoomResponse struct {
	HomeName string   `json:"name"`
	HomeID   string   `json:"id"`
	DIDs     []string `json:"dids"`
	Rooms    []struct {
		ID   string   `json:"id"`
		Name string   `json:"name"`
		DIDs []string `json:"dids"`
	} `json:"roomlist"`
}

// GetHomes returns all homes visible to the authenticated user.
func (c *MiClient) GetHomes() ([]*std.HomeInfo, error) {
	params := `{"fg":true,"fetch_share":true,"fetch_share_dev":true,"limit":300,"app_ver":7}`
	result, err := c.cloud.Request(c.baseURL, apiHomeRoomList, params, nil)
	if err != nil {
		return nil, fmt.Errorf("get homes: %w", err)
	}

	var resp struct {
		Homelist []homeRoomResponse `json:"homelist"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("parse homes response: %w", err)
	}

	homes := make([]*std.HomeInfo, 0, len(resp.Homelist))
	for _, h := range resp.Homelist {
		homes = append(homes, &std.HomeInfo{
			ID:   h.HomeID,
			Name: h.HomeName,
		})
	}
	return homes, nil
}

// GetRooms returns all rooms for the given homeID.
func (c *MiClient) GetRooms(homeID string) ([]*data.Space, error) {
	params := `{"fg":true,"fetch_share":true,"fetch_share_dev":true,"limit":300,"app_ver":7}`
	result, err := c.cloud.Request(c.baseURL, apiHomeRoomList, params, nil)
	if err != nil {
		return nil, fmt.Errorf("get rooms: %w", err)
	}

	var resp struct {
		Homelist []homeRoomResponse `json:"homelist"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("parse rooms response: %w", err)
	}

	var rooms []*data.Space
	for _, home := range resp.Homelist {
		// If homeID is specified, filter by it
		if homeID != "" && home.HomeID != homeID {
			continue
		}
		for _, r := range home.Rooms {
			rooms = append(rooms, &data.Space{
				Name: r.Name,
				From: map[string]string{
					BrandXiaomi: r.ID,
				},
			})
		}
	}
	return rooms, nil
}

// homeDeviceListResponse represents the response from device_list_page API.
type homeDeviceListResponse struct {
	List    []midata.DeviceInfo `json:"list"`
	MaxDID  string              `json:"max_did"`
	HasMore bool                `json:"has_more"`
}

// GetDevices returns all devices for the given homeID.
// homeID is required; use GetHomes() to get available home IDs first.
func (c *MiClient) GetDevices(homeID string) ([]*data.Device, error) {
	if homeID == "" {
		return nil, fmt.Errorf("homeID is required")
	}

	// Fetch devices with pagination
	devices := make(map[string]*midata.DeviceInfo)
	startDID := ""
	hasMore := true

	for hasMore {
		userID, _ := c.cloud.UserToken()
		reqParams := map[string]any{
			"home_owner":         userID,
			"home_id":            homeID,
			"limit":              homeDeviceLimit,
			"start_did":          startDID,
			"get_split_device":   false,
			"support_smart_home": true,
			"get_cariot_device":  true,
			"get_third_device":   true,
		}
		reqJSON, err := json.Marshal(reqParams)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		result, err := c.cloud.Request(c.baseURL, apiHomeDeviceList, string(reqJSON), nil)
		if err != nil {
			return nil, fmt.Errorf("get devices failed: %w", err)
		}

		var resp homeDeviceListResponse
		if err := json.Unmarshal(result, &resp); err != nil {
			return nil, fmt.Errorf("parse response: %w", err)
		}

		// Collect devices
		for i := range resp.List {
			d := &resp.List[i]
			devices[d.DID] = d
		}

		// Pagination: continue if has_more and max_did is not empty
		startDID = resp.MaxDID
		hasMore = resp.HasMore && startDID != ""
	}

	// Convert to data.Device
	var result []*data.Device
	for _, d := range devices {
		c.deviceCache[d.DID] = d
		if c.deviceStore != nil {
			_ = c.deviceStore.Save(d)
		}
		result = append(result, &data.Device{
			FromID:    d.DID,
			From:      BrandXiaomi,
			Name:      d.Name,
			Type:      d.Model,
			IP:        d.LocalIP,
			Token:     d.Token,
			URN:       d.SpecType,
			SpaceName: d.RoomName,
			Online:    d.IsOnline,
		})
	}

	return result, nil
}

// GetSpec fetches the capability specification for deviceID.
func (c *MiClient) GetSpec(deviceID string) (*std.SpecInfo, error) {
	info, err := c.GetDeviceInfo(deviceID)
	if err != nil {
		return nil, fmt.Errorf("get spec: %w", err)
	}
	if info.SpecType == "" {
		return nil, fmt.Errorf("get spec: device %s has no spec URN", deviceID)
	}

	specJSON, err := c.specFetcher.GetSpec(info.SpecType)
	if err != nil {
		return nil, fmt.Errorf("get spec: %w", err)
	}

	return &std.SpecInfo{
		DeviceID: deviceID,
		Model:    info.Model,
		Raw:      specJSON,
		Extra: map[string]any{
			"urn": info.SpecType,
		},
	}, nil
}

// GetDeviceInfo returns the full device info for the given deviceID.
// This is a helper method for accessing detailed device information.
func (c *MiClient) GetDeviceInfo(deviceID string) (*midata.DeviceInfo, error) {
	// Try cache first
	if info, ok := c.deviceCache[deviceID]; ok {
		return info, nil
	}

	// Try store
	if c.deviceStore != nil {
		info, err := c.deviceStore.GetByDID(deviceID)
		if err == nil && info != nil {
			c.deviceCache[deviceID] = info
			return info, nil
		}
	}

	return nil, fmt.Errorf("device %s not found", deviceID)
}

// ────────────────────────────────────────────────────────────────────────────────
// Control methods
// ────────────────────────────────────────────────────────────────────────────────

// Execute sends an action command to a device.
//
// Expected params:
//   - did: device ID
//   - siid: service ID
//   - aiid: action ID
//   - in: input parameters (optional, array of values)
func (c *MiClient) Execute(params map[string]any) (map[string]any, error) {
	did, ok := params["did"].(string)
	if !ok {
		return nil, errors.New("execute: missing or invalid 'did' parameter")
	}

	siid, ok := getIntParam(params, "siid")
	if !ok {
		return nil, errors.New("execute: missing or invalid 'siid' parameter")
	}

	aiid, ok := getIntParam(params, "aiid")
	if !ok {
		return nil, errors.New("execute: missing or invalid 'aiid' parameter")
	}

	// Build action request
	actionParams := map[string]any{
		"did":  did,
		"siid": siid,
		"aiid": aiid,
	}
	if in, ok := params["in"]; ok {
		actionParams["in"] = in
	}

	reqData := map[string]any{
		"params": []map[string]any{actionParams},
	}
	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("execute: marshal request: %w", err)
	}

	result, err := c.cloud.Request(c.baseURL, apiMiotspecAct, string(reqJSON), nil)
	if err != nil {
		return nil, fmt.Errorf("execute: %w", err)
	}

	var resp []map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("execute: parse response: %w", err)
	}

	if len(resp) == 0 {
		return nil, errors.New("execute: empty response")
	}

	return resp[0], nil
}

// GetProp reads property values from a device.
//
// Expected params:
//   - did: device ID
//   - siid: service ID
//   - piid: property ID
//
// Or batch mode:
//   - props: array of {did, siid, piid} objects
func (c *MiClient) GetProps(params map[string]any) (any, error) {
	var propList []map[string]any

	// Check for batch mode
	if props, ok := params["props"].([]any); ok {
		for _, p := range props {
			if pm, ok := p.(map[string]any); ok {
				propList = append(propList, pm)
			}
		}
	} else {
		// Single property mode
		did, ok := params["did"].(string)
		if !ok {
			return nil, errors.New("get_prop: missing or invalid 'did' parameter")
		}

		siid, ok := getIntParam(params, "siid")
		if !ok {
			return nil, errors.New("get_prop: missing or invalid 'siid' parameter")
		}

		piid, ok := getIntParam(params, "piid")
		if !ok {
			return nil, errors.New("get_prop: missing or invalid 'piid' parameter")
		}

		propList = []map[string]any{
			{"did": did, "siid": siid, "piid": piid},
		}
	}

	reqData := map[string]any{
		"params": propList,
	}
	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("get_prop: marshal request: %w", err)
	}

	result, err := c.cloud.Request(c.baseURL, apiMiotspecProp, string(reqJSON), nil)
	if err != nil {
		return nil, fmt.Errorf("get_prop: %w", err)
	}

	var resp []map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("get_prop: parse response: %w", err)
	}

	// Return single value or array based on request mode
	if _, ok := params["props"]; ok {
		return resp, nil
	}
	if len(resp) > 0 {
		return resp[0]["value"], nil
	}
	return nil, nil
}

// SetProp sets property values on a device.
//
// Expected params:
//   - did: device ID
//   - siid: service ID
//   - piid: property ID
//   - value: value to set
//
// Or batch mode:
//   - props: array of {did, siid, piid, value} objects
func (c *MiClient) SetProps(params map[string]any) (any, error) {
	var propList []map[string]any

	// Check for batch mode
	if props, ok := params["props"].([]any); ok {
		for _, p := range props {
			if pm, ok := p.(map[string]any); ok {
				propList = append(propList, pm)
			}
		}
	} else {
		// Single property mode
		did, ok := params["did"].(string)
		if !ok {
			return nil, errors.New("set_prop: missing or invalid 'did' parameter")
		}

		siid, ok := getIntParam(params, "siid")
		if !ok {
			return nil, errors.New("set_prop: missing or invalid 'siid' parameter")
		}

		piid, ok := getIntParam(params, "piid")
		if !ok {
			return nil, errors.New("set_prop: missing or invalid 'piid' parameter")
		}

		value, ok := params["value"]
		if !ok {
			return nil, errors.New("set_prop: missing 'value' parameter")
		}

		propList = []map[string]any{
			{"did": did, "siid": siid, "piid": piid, "value": value},
		}
	}

	reqData := map[string]any{
		"params": propList,
	}
	reqJSON, err := json.Marshal(reqData)
	if err != nil {
		return nil, fmt.Errorf("set_prop: marshal request: %w", err)
	}

	result, err := c.cloud.Request(c.baseURL, apiMiotspecSet, string(reqJSON), nil)
	if err != nil {
		return nil, fmt.Errorf("set_prop: %w", err)
	}

	var resp []map[string]any
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("set_prop: parse response: %w", err)
	}

	return resp, nil
}

// ────────────────────────────────────────────────────────────────────────────────
// Event lifecycle methods
// ────────────────────────────────────────────────────────────────────────────────

// EnableEvent starts event subscription for the given device.
// Note: Xiaomi cloud does not support real-time push events via HTTP.
// This is a placeholder for future implementation (e.g., polling or MQTT).
func (c *MiClient) EnableEvent(params map[string]any) error {
	// TODO: Implement event subscription (polling or MQTT)
	return errors.New("enable_event: not implemented for Xiaomi cloud")
}

// DisableEvent stops event subscription for the given device.
func (c *MiClient) DisableEvent(params map[string]any) error {
	// TODO: Implement event unsubscription
	return errors.New("disable_event: not implemented for Xiaomi cloud")
}

// ────────────────────────────────────────────────────────────────────────────────
// Helper functions
// ────────────────────────────────────────────────────────────────────────────────

// getIntParam extracts an integer parameter from the map.
// It handles both int and float64 (JSON number) types.
func getIntParam(params map[string]any, key string) (int, bool) {
	v, ok := params[key]
	if !ok {
		return 0, false
	}
	switch val := v.(type) {
	case int:
		return val, true
	case int64:
		return int(val), true
	case float64:
		return int(val), true
	default:
		return 0, false
	}
}

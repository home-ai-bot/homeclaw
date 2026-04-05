// Package miio provides a Xiaomi MIoT cloud client implementation.
package miio

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"

	"github.com/AlexxIT/go2rtc/pkg/xiaomi"
	"github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio/util"
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
	deviceStore MiDeviceStore
	homeStore   MiHomeStore
	baseURL     string
	country     string // region code (cn, de, ru, sg, i2, us, etc.)
}

// NewMiClient creates a new MiClient instance.
//
// Parameters:
//   - cloud: authenticated xiaomi.Cloud instance
//   - country: region code (cn, de, ru, sg, i2, us, etc.)
//   - workspace: data root directory for caching
//   - deviceStore: optional MiDeviceStore for persisting device info (can be nil)
//   - homeStore: optional MiHomeStore for persisting home info (can be nil)
func NewMiClient(cloud *xiaomi.Cloud, country, workspace string, deviceStore MiDeviceStore, homeStore MiHomeStore, fetcher *SpecFetcher) *MiClient {
	if country == "" {
		country = "cn"
	}
	return &MiClient{
		cloud:       cloud,
		specFetcher: fetcher,
		deviceStore: deviceStore,
		homeStore:   homeStore,
		baseURL:     getBaseURL(country),
		country:     country,
	}
}

// GetUserAndRegion returns the authenticated user ID and region code.
func (c *MiClient) GetUserAndRegion() (userID string, region string) {
	userID, _ = c.cloud.UserToken()
	return userID, c.country
}

// checkLoadToken checks if the current token is empty and attempts to reload it from config.
// If the token is still empty after reloading, it returns an error with instructions.
func (c *MiClient) checkLoadToken() error {
	// Check current token
	_, token := c.cloud.UserToken()
	if token != "" {
		return nil
	}

	// Token is empty, try to reload from config
	var xiaomiCfg struct {
		Cfg map[string]string `yaml:"xiaomi"`
	}
	if err := config.LoadGo2RTCConfig(&xiaomiCfg); err != nil {
		return fmt.Errorf("failed to load xiaomi config: %w", err)
	}

	// Get first key-value pair: userId=key, token=value
	var userID, newToken string
	for k, v := range xiaomiCfg.Cfg {
		userID = k
		newToken = v
		break
	}

	if newToken == "" {
		// Get local IP for the error message
		localIP := getLocalIP()
		if localIP == "" {
			localIP = "<device_ip>"
		}
		return fmt.Errorf("Must Confirm!: xiaomi token is empty, please open http://%s:1984, login with your xiaomi account in the interface, then click 'Config Save & Restart'", localIP)
	}

	// Update cloud with new token
	c.cloud.LoginWithToken(userID, newToken)
	return nil
}

// getLocalIP returns the local IP address of the machine.
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		if ipnet, ok := a.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return ""
}

// isAuthError checks if the error is an authentication error (401 or token related).
func isAuthError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	// Check for common auth error indicators
	return contains(errStr, "401") ||
		contains(errStr, "unauthorized") ||
		contains(errStr, "unauthenticated") ||
		contains(errStr, "invalid token") ||
		contains(errStr, "token expired")
}

// contains checks if the string contains the substring (case-insensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && (containsAt(s, substr, 0) || containsHelper(s, substr, 1)))
}

func containsHelper(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if containsAt(s, substr, i) {
			return true
		}
	}
	return false
}

func containsAt(s, substr string, start int) bool {
	for i := 0; i < len(substr); i++ {
		if lower(s[start+i]) != lower(substr[i]) {
			return false
		}
	}
	return true
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// getAuthErrorMsg returns the standard authentication error message.
func getAuthErrorMsg() string {
	localIP := getLocalIP()
	if localIP == "" {
		localIP = "<device_ip>"
	}
	return fmt.Sprintf("Must Confirm!: xiaomi token is empty or invalid, please open http://%s:1984, login with your xiaomi account in the interface, then click 'Config Save & Restart'", localIP)
}

// request performs an authenticated request to the Xiaomi cloud API.
// It checks the token, makes the request, and handles authentication errors.
func (c *MiClient) request(api string, params string) ([]byte, error) {
	// Check token before request
	if err := c.checkLoadToken(); err != nil {
		return nil, err
	}

	result, err := c.cloud.Request(c.baseURL, api, params, nil)
	if err != nil {
		if isAuthError(err) {
			return nil, errors.New(getAuthErrorMsg())
		}
		return nil, err
	}
	return result, nil
}

// Brand returns the brand identifier for Xiaomi platform.
func (c *MiClient) Brand() string {
	return BrandXiaomi
}

// ────────────────────────────────────────────────────────────────────────────────
// Query methods
// ────────────────────────────────────────────────────────────────────────────────

// GetHomes returns all homes visible to the authenticated user.
func (c *MiClient) GetHomes() ([]*third.HomeInfo, error) {
	params := `{"fg":true,"fetch_share":true,"fetch_share_dev":true,"limit":300,"app_ver":7}`
	result, err := c.request(apiHomeRoomList, params)
	if err != nil {
		return nil, fmt.Errorf("get homes: %w", err)
	}

	var resp struct {
		Homelist []HomeRoomInfo `json:"homelist"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("parse homes response: %w", err)
	}

	homes := make([]*third.HomeInfo, 0, len(resp.Homelist))
	for _, h := range resp.Homelist {
		homes = append(homes, &third.HomeInfo{
			ID:   h.HomeID,
			Name: h.HomeName,
		})
		if c.homeStore != nil {
			_ = c.homeStore.Save(&h)
		}
	}

	return homes, nil
}

// GetRooms returns all rooms for the given homeID.
func (c *MiClient) GetRooms(homeID string) ([]*data.Space, error) {
	params := `{"fg":true,"fetch_share":true,"fetch_share_dev":true,"limit":300,"app_ver":7}`
	result, err := c.request(apiHomeRoomList, params)
	if err != nil {
		return nil, fmt.Errorf("get rooms: %w", err)
	}

	var resp struct {
		Homelist []HomeRoomInfo `json:"homelist"`
	}
	if err := json.Unmarshal(result, &resp); err != nil {
		return nil, fmt.Errorf("parse rooms response: %w", err)
	}

	var rooms []*data.Space
	for _, home := range resp.Homelist {
		if c.homeStore != nil {
			_ = c.homeStore.Save(&home)
		}
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
	List    []DeviceInfo `json:"list"`
	MaxDID  string       `json:"max_did"`
	HasMore bool         `json:"has_more"`
}

// GetDevices returns all devices for the given homeID.
// homeID is required; use GetHomes() to get available home IDs first.
func (c *MiClient) GetDevices(homeID string) ([]*data.Device, error) {
	if homeID == "" {
		return nil, fmt.Errorf("homeID is required")
	}

	// Fetch devices with pagination
	devices := make(map[string]*DeviceInfo)
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
		result, err := c.request(apiHomeDeviceList, string(reqJSON))
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

	// Build DID -> RoomName map from homeStore
	didRoomMap := make(map[string]string)
	if c.homeStore != nil {
		homes, _ := c.homeStore.GetAll()
		for _, home := range homes {
			for _, room := range home.Rooms {
				for _, did := range room.DIDs {
					didRoomMap[did] = room.Name
				}
			}
		}
	}

	// Convert to data.Device
	var result []*data.Device
	var specErrors []string
	for _, d := range devices {
		// Set RoomName from didRoomMap
		if roomName, ok := didRoomMap[d.DID]; ok {
			d.RoomName = roomName
		}
		if c.deviceStore != nil {
			_ = c.deviceStore.Save(d)
		}
		if !d.IsOnline {
			continue
		}
		if d == nil || d.SpecType == "" {
			continue
		}
		// Get spec for this device
		spec, err := c.GetSpec(d.DID)
		if err != nil {
			specErrors = append(specErrors, fmt.Sprintf("%s: %v", d.Name, err))
			continue
		}
		// Parse and process spec using spec_parser
		parsedSpec, err := util.ParseSpecJSON(spec.Raw)
		if err != nil {
			specErrors = append(specErrors, fmt.Sprintf("%s: parse error - %v", d.Name, err))
			continue
		}
		// Generate device commands (write operations: SetProp and Action)
		commands := parsedSpec.GenerateDeviceCommands(d.DID)

		// Extract actions (Method="Action")
		var actions []string
		for _, cmd := range commands {
			if cmd.Method == "Action" || cmd.Method == "SetProp" {
				actions = append(actions, cmd.Desc)
			}
		}

		// Generate status commands (GetProp) and extract descriptions (read operations)
		statusCommands := parsedSpec.GenerateStatusCommands(d.DID)
		var status []string
		for _, cmd := range statusCommands {
			status = append(status, cmd.Desc)
		}

		// Save processed specs for write and read modes
		if c.specFetcher != nil && d.SpecType != "" {
			writeJSON, _ := json.Marshal(commands)
			_ = c.specFetcher.SaveProcessedSpec(d.SpecType, "write", string(writeJSON))

			readJSON, _ := json.Marshal(statusCommands)
			_ = c.specFetcher.SaveProcessedSpec(d.SpecType, "read", string(readJSON))
		}

		result = append(result, &data.Device{
			FromID:    d.DID,
			From:      BrandXiaomi,
			Name:      d.Name,
			Type:      d.Model,
			SpaceName: d.RoomName,
			IP:        d.LocalIP,
			Token:     d.Token,
			URN:       d.SpecType,
		})
	}

	return result, nil
}

// GetSpec fetches the capability specification for deviceID.
func (c *MiClient) GetSpec(deviceID string) (*third.SpecInfo, error) {
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

	return &third.SpecInfo{
		DeviceID: deviceID,
		Model:    info.Model,
		Raw:      specJSON,
		Extra: map[string]any{
			"urn": info.SpecType,
		},
	}, nil
}

// GetRtspStr returns the go2rtc-compatible RTSP stream URL for the given deviceID.
// Returns an empty string and no error if the device is not a camera or lacks IP/token info.
func (c *MiClient) GetRtspStr(deviceID string) (string, error) {
	info, err := c.GetDeviceInfo(deviceID)
	if err != nil {
		return "", fmt.Errorf("get rtsp: %w", err)
	}
	// Only cameras support RTSP streaming
	if !hasCamera(info.Model) {
		return "", nil
	}
	if info.LocalIP == "" {
		return "", nil
	}
	user, region := c.GetUserAndRegion()
	if user == "" {
		return "", nil
	}
	// go2rtc xiaomi RTSP format: xiaomi://{user}:{region}@{ip}?did={did}&model={model}#video=copy#audio=pcmu
	return fmt.Sprintf("xiaomi://%s:%s@%s?did=%s&model=%s#video=copy#audio=pcmu",
		user, region, info.LocalIP, info.DID, info.Model), nil
}

// GetDeviceInfo returns the full device info for the given deviceID.
// This is a helper method for accessing detailed device information.
func (c *MiClient) GetDeviceInfo(deviceID string) (*DeviceInfo, error) {
	// Try store
	if c.deviceStore != nil {
		info, err := c.deviceStore.GetByDID(deviceID)
		if err == nil && info != nil {
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

	result, err := c.request(apiMiotspecAct, string(reqJSON))
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

	result, err := c.request(apiMiotspecProp, string(reqJSON))
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

	result, err := c.request(apiMiotspecSet, string(reqJSON))
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

// hasCamera checks if the device model indicates a camera device.
func hasCamera(model string) bool {
	return strings.Contains(model, ".camera.") ||
		strings.Contains(model, ".cateye.") ||
		strings.Contains(model, ".feeder.")
}

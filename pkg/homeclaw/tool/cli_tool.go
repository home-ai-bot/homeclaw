package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_cli
// ─────────────────────────────────────────────────────────────────────────────

// CLITool is a unified CLI-style tool that dispatches to the correct brand
// client based on the "brand" field, then routes to one of the supported
// methods: syncHomes, syncDevices, getProps, setProps, execute.
//
// commandJson schema:
//
//	{
//	  "brand":  "xiaomi" | "tuya" | …,
//	  "method": "syncHomes" | "syncDevices" | "getProps" | "setProps" | "execute",
//	  "params": { … }   // optional for syncHomes
//	}
//
// syncHomes   – fetch all homes from the brand cloud and persist them.
// syncDevices – fetch rooms + devices for a home; params: {"homeId":"xxx"}.
// getProps    – read device properties; params are brand-specific.
// setProps    – write device properties; params are brand-specific.
// execute     – send an action command to a device; params are brand-specific.
type CLITool struct {
	clients       map[string]third.Client
	homeStore     data.HomeStore
	spaceStore    data.SpaceStore
	deviceStore   data.DeviceStore
	deviceOpStore data.DeviceOpStore
}

// NewCLITool creates a CLITool with the given brand clients and data stores.
// clients maps brand name (e.g. "xiaomi", "tuya") to its third.Client.
func NewCLITool(
	clients map[string]third.Client,
	homeStore data.HomeStore,
	spaceStore data.SpaceStore,
	deviceStore data.DeviceStore,
	deviceOpStore data.DeviceOpStore,
) *CLITool {
	return &CLITool{
		clients:       clients,
		homeStore:     homeStore,
		spaceStore:    spaceStore,
		deviceStore:   deviceStore,
		deviceOpStore: deviceOpStore,
	}
}

// RegisterClient adds or replaces a brand client at runtime.
func (t *CLITool) RegisterClient(client third.Client) {
	if t.clients == nil {
		t.clients = make(map[string]third.Client)
	}
	t.clients[client.Brand()] = client
}

func (t *CLITool) Name() string { return "hc_cli" }

func (t *CLITool) Description() string {
	return "Do NOT use directly!"
}

func (t *CLITool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"commandJson": map[string]any{
				"type":        "string",
				"description": ``,
			},
		},
		"required": []string{"commandJson"},
	}
}

// cliCommandRequest is the parsed form of the commandJson argument.
type cliCommandRequest struct {
	Brand  string         `json:"brand"`
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func (t *CLITool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	commandJson, ok := args["commandJson"].(string)
	if !ok || commandJson == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'commandJson' parameter", IsError: true}
	}

	var req cliCommandRequest
	if err := json.Unmarshal([]byte(commandJson), &req); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse commandJson: %v", err), IsError: true}
	}

	if req.Method == "" {
		return &tools.ToolResult{ForLLM: "missing 'method' in commandJson", IsError: true}
	}

	switch req.Method {
	case "listCameras":
		return t.execListCameras()
	case "setCurrentHome":
		return t.execSetCurrentHome(req.Params)
	case "getCurrentHome":
		return t.execGetCurrentHome(req.Params)
	case "saveDeviceOps":
		return t.execSaveDeviceOps(req.Params)
	case "listDevicesWithoutOps":
		return t.execListDevicesWithoutOps(req.Params)
	case "markNoAction":
		return t.execMarkNoAction(req.Params)
	}
	if req.Brand == "" {
		return &tools.ToolResult{ForLLM: "missing 'brand' in commandJson", IsError: true}
	}
	client, found := t.clients[req.Brand]
	if !found {
		available := make([]string, 0, len(t.clients))
		for k := range t.clients {
			available = append(available, k)
		}
		return &tools.ToolResult{
			ForLLM:  fmt.Sprintf("unknown brand '%s'; registered brands: %v", req.Brand, available),
			IsError: true,
		}
	}

	switch req.Method {
	case "syncHomes":
		return t.execSyncHomes(client)
	case "syncDevices":
		return t.execSyncDevices(client, req.Params)
	case "getProps":
		return t.execGetProps(client, req.Params)
	case "setProps":
		return t.execSetProps(client, req.Params)
	case "execute":
		return t.execExecute(client, req.Params)
	case "getSpec":
		return t.execGetSpec(client, req.Params)
	case "exe":
		return t.execExe(client, req.Params)
	default:
		return &tools.ToolResult{
			ForLLM:  fmt.Sprintf("unknown method '%s'; Must Confirm! tool must invoke by skills,please use the right skill!", req.Method),
			IsError: true,
		}
	}
}

// execSyncHomes fetches all homes from the brand cloud and persists them.
func (t *CLITool) execSyncHomes(client third.Client) *tools.ToolResult {
	homes, err := client.GetHomes()
	if err != nil {
		msg := fmt.Sprintf("failed to sync homes: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	if len(homes) == 0 {
		return tools.NewToolResult(fmt.Sprintf("no homes found for brand '%s'", client.Brand()))
	}

	dataHomes := make([]data.Home, 0, len(homes))
	for _, h := range homes {
		dataHomes = append(dataHomes, data.Home{
			FromID:  h.ID,
			From:    client.Brand(),
			Name:    h.Name,
			Current: false,
		})
	}
	// Mark the only home as current automatically
	if len(dataHomes) == 1 {
		dataHomes[0].Current = true
	}

	if err := t.homeStore.Save(dataHomes...); err != nil {
		msg := fmt.Sprintf("failed to save homes: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	result := make([]map[string]string, 0, len(homes))
	for _, h := range homes {
		result = append(result, map[string]string{
			"home_id": h.ID,
			"name":    h.Name,
		})
	}
	b, _ := json.Marshal(result)
	return tools.NewToolResult(fmt.Sprintf("synced %d homes for brand '%s': %s", len(homes), client.Brand(), string(b)))
}

// execSyncDevices fetches rooms and devices for a home and persists them.
func (t *CLITool) execSyncDevices(client third.Client, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for syncDevices", IsError: true}
	}
	homeID, ok := params["homeId"].(string)
	if !ok || homeID == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'homeId' in params", IsError: true}
	}

	if err := t.homeStore.SetCurrent(homeID, client.Brand()); err != nil {
		msg := fmt.Sprintf("failed to set current home: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	rooms, err := client.GetRooms(homeID)
	if err != nil {
		msg := fmt.Sprintf("failed to sync rooms: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}
	if len(rooms) > 0 {
		spaceValues := make([]data.Space, 0, len(rooms))
		for _, r := range rooms {
			if r != nil {
				spaceValues = append(spaceValues, *r)
			}
		}
		if len(spaceValues) > 0 {
			t.spaceStore.Save(spaceValues...)
		}
	}

	devices, err := client.GetDevices(homeID)
	if err != nil {
		msg := fmt.Sprintf("failed to sync devices: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	// Skip devices that already exist in the store
	existingDevices, _ := t.deviceStore.GetAll()
	existingSet := make(map[string]struct{}, len(existingDevices))
	for _, ed := range existingDevices {
		existingSet[ed.FromID+"|"+ed.From] = struct{}{}
	}
	deviceValues := make([]data.Device, 0, len(devices))
	for _, d := range devices {
		if d == nil {
			continue
		}
		key := d.FromID + "|" + d.From
		if _, exists := existingSet[key]; !exists {
			deviceValues = append(deviceValues, *d)
		}
	}
	t.deviceStore.Save(deviceValues...)

	for _, d := range devices {

		streamName := d.From + "_" + d.FromID
		streamURL, err := client.GetRtspStr(d.FromID)
		if err != nil {
			logger.Infof("failed to get RTSP URL: %v", err)
			continue
		}
		if streamURL == "" {
			continue
		}

		if err := config.PatchGo2RTCConfig([]string{"streams", streamName}, []string{streamURL}); err != nil {
			logger.Info(fmt.Sprintf("%s: go2rtc config error - %v", d.Name, err))
		}

	}

	b, _ := json.Marshal(devices)
	summary := fmt.Sprintf("synced %d rooms and %d devices for brand '%s': %s",
		len(rooms), len(devices), client.Brand(), string(b))
	return tools.NewToolResult(summary)
}

// execGetProps reads device properties via the brand client.
func (t *CLITool) execGetProps(client third.Client, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for getProps", IsError: true}
	}
	result, err := client.GetProps(params)
	if err != nil {
		msg := fmt.Sprintf("failed to execute getProps: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}
	b, _ := json.Marshal(result)
	return tools.NewToolResult(fmt.Sprintf("getProps result: %s", string(b)))
}

// execSetProps writes device properties via the brand client.
func (t *CLITool) execSetProps(client third.Client, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for setProps", IsError: true}
	}
	result, err := client.SetProps(params)
	if err != nil {
		msg := fmt.Sprintf("failed to execute setProps: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}
	b, _ := json.Marshal(result)
	return tools.NewToolResult(fmt.Sprintf("setProps result: %s", string(b)))
}

// execExecute sends an action command to a device via the brand client.
func (t *CLITool) execExecute(client third.Client, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for execute", IsError: true}
	}
	result, err := client.Execute(params)
	if err != nil {
		msg := fmt.Sprintf("failed to execute action: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}
	b, _ := json.Marshal(result)
	return tools.NewToolResult(fmt.Sprintf("execute result: %s", string(b)))
}

// execGetSpec fetches the capability specification for a device.
func (t *CLITool) execGetSpec(client third.Client, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for getSpec", IsError: true}
	}
	deviceID, ok := params["deviceId"].(string)
	if !ok || deviceID == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'deviceId' in params", IsError: true}
	}
	spec, err := client.GetSpec(deviceID)
	if err != nil {
		msg := fmt.Sprintf("failed to get spec: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}
	b, _ := json.Marshal(spec)
	return tools.NewToolResult(fmt.Sprintf("getSpec result: %s", string(b)))
}

// execListCameras lists all camera devices with RTSP stream URLs.
func (t *CLITool) execListCameras() *tools.ToolResult {
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list devices: %v", err), IsError: true}
	}

	// Filter camera devices and build response with RTSP URLs
	type cameraInfo struct {
		FromID    string `json:"from_id"`
		From      string `json:"from"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		SpaceName string `json:"space_name,omitempty"`
		RtspURL   string `json:"rtsp_url"`
	}

	var cameras []cameraInfo
	for _, d := range devices {
		if isCamera(d.Type) {
			cameras = append(cameras, cameraInfo{
				FromID:    d.FromID,
				From:      d.From,
				Name:      d.Name,
				Type:      d.Type,
				SpaceName: d.SpaceName,
				RtspURL:   fmt.Sprintf("rtsp://127.0.0.1:8554/%s_%s", d.From, d.FromID),
			})
		}
	}

	if len(cameras) == 0 {
		return tools.NewToolResult(`{"cameras":[],"message":"No camera devices found"}`)
	}

	b, _ := json.Marshal(map[string]any{"cameras": cameras})
	return tools.NewToolResult(string(b))
}

// isCamera checks if the device model indicates a camera device.
func isCamera(model string) bool {
	return containsAny(model, ".camera.", ".cateye.", ".feeder.")
}

// containsAny returns true if s contains any of the substrings.
func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if len(s) >= len(sub) {
			for i := 0; i <= len(s)-len(sub); i++ {
				if s[i:i+len(sub)] == sub {
					return true
				}
			}
		}
	}
	return false
}

// execSetCurrentHome sets the current home for a specific brand and ID.
func (t *CLITool) execSetCurrentHome(params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for setCurrentHome", IsError: true}
	}

	fromID, ok := params["from_id"].(string)
	if !ok || fromID == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from_id", IsError: true}
	}

	from, ok := params["from"].(string)
	if !ok || from == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from", IsError: true}
	}

	if err := t.homeStore.SetCurrent(fromID, from); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to set current home: %v", err), IsError: true}
	}

	return tools.NewToolResult(fmt.Sprintf("successfully set home %s from %s as current", fromID, from))
}

// execGetCurrentHome retrieves the current home for a specific brand.
func (t *CLITool) execGetCurrentHome(params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for getCurrentHome", IsError: true}
	}

	from, ok := params["from"].(string)
	if !ok || from == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from", IsError: true}
	}

	home, err := t.homeStore.GetCurrent(from)
	if err != nil {
		// Check if there are any homes for this brand
		allHomes, _ := t.homeStore.GetAll()
		var brandHomes []string
		for _, h := range allHomes {
			if h.From == from {
				brandHomes = append(brandHomes, fmt.Sprintf("%s (id: %s)", h.Name, h.FromID))
			}
		}
		if len(brandHomes) == 0 {
			return &tools.ToolResult{ForLLM: fmt.Sprintf("no homes found for brand '%s', please sync homes first", from), IsError: true}
		}
		msg := fmt.Sprintf("no current home set for brand '%s', available homes: %v. Must Confirm!", from, brandHomes)
		// Homes exist but none is set as current
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	result, _ := json.Marshal(map[string]any{
		"home_id": home.FromID,
		"name":    home.Name,
		"from":    home.From,
	})
	return tools.NewToolResult(string(result))
}

// execSaveDeviceOps saves device operations analyzed by AI in batch.
// Required params: from, from_id, ops_array (JSON string)
func (t *CLITool) execSaveDeviceOps(params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for saveDeviceOps", IsError: true}
	}

	// Extract from and from_id from top-level params
	from, ok := params["from"].(string)
	if !ok || from == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from", IsError: true}
	}

	fromID, ok := params["from_id"].(string)
	if !ok || fromID == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from_id", IsError: true}
	}

	// Extract ops_array - accept both string (JSON-encoded) and array formats
	var opsArrayJSON string

	// Try string format first (JSON-encoded array)
	if opsArrayStr, ok := params["ops_array"].(string); ok && opsArrayStr != "" {
		opsArrayJSON = opsArrayStr
	} else if opsArrayRaw, ok := params["ops_array"].([]any); ok && len(opsArrayRaw) > 0 {
		// Convert array back to JSON string
		opsArrayBytes, err := json.Marshal(opsArrayRaw)
		if err != nil {
			return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to marshal ops_array: %v", err), IsError: true}
		}
		opsArrayJSON = string(opsArrayBytes)
	} else {
		return &tools.ToolResult{ForLLM: "missing required parameter: ops_array", IsError: true}
	}

	// Parse the JSON array - use json.RawMessage to avoid parsing param
	type opEntry struct {
		Method string          `json:"method"`
		Ops    string          `json:"ops"`
		Param  json.RawMessage `json:"param"`
	}

	var opsArray []opEntry
	if err := json.Unmarshal([]byte(opsArrayJSON), &opsArray); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse ops_array JSON: %v", err), IsError: true}
	}

	if len(opsArray) == 0 {
		return &tools.ToolResult{ForLLM: "ops_array is empty", IsError: true}
	}

	// Convert to DeviceOp slice - param is saved directly as JSON string without parsing
	deviceOps := make([]data.DeviceOp, 0, len(opsArray))
	for _, entry := range opsArray {
		if len(entry.Param) == 0 {
			continue
		}

		deviceOps = append(deviceOps, data.DeviceOp{
			FromID: fromID,
			From:   from,
			Ops:    entry.Ops,
			Method: entry.Method,
			Param:  string(entry.Param),
		})
	}

	if len(deviceOps) == 0 {
		return &tools.ToolResult{ForLLM: "no valid operations to save", IsError: true}
	}

	// Batch save all operations
	if err := t.deviceOpStore.Save(deviceOps...); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to batch save device operations: %v", err), IsError: true}
	}

	return tools.NewToolResult(fmt.Sprintf("successfully saved %d device operations for device %s from %s", len(deviceOps), fromID, from))
}

// execListDevicesWithoutOps lists all devices that don't have any device operations saved.
func (t *CLITool) execListDevicesWithoutOps(params map[string]any) *tools.ToolResult {
	// Get all devices
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}

	// Filter devices without operations
	type deviceWithoutOps struct {
		FromID    string `json:"from_id"`
		From      string `json:"from"`
		Name      string `json:"name"`
		Type      string `json:"type"`
		SpaceName string `json:"space_name,omitempty"`
	}

	var devicesWithoutOps []deviceWithoutOps
	for _, d := range devices {
		// Check if ops is empty or only contains "NoAction"
		if len(d.Ops) == 0 || (len(d.Ops) == 1 && d.Ops[0] == "NoAction") {
			devicesWithoutOps = append(devicesWithoutOps, deviceWithoutOps{
				FromID:    d.FromID,
				From:      d.From,
				Name:      d.Name,
				Type:      d.Type,
				SpaceName: d.SpaceName,
			})
		}
	}

	if len(devicesWithoutOps) == 0 {
		return tools.NewToolResult(`{"devices":[],"message":"All devices have operations configured"}`)
	}

	result, _ := json.Marshal(map[string]any{
		"devices": devicesWithoutOps,
		"count":   len(devicesWithoutOps),
	})
	return tools.NewToolResult(string(result))
}

// execExe executes a device operation by reading from DeviceOpStore and calling the appropriate method.
func (t *CLITool) execExe(client third.Client, params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for exe", IsError: true}
	}

	fromID, ok := params["from_id"].(string)
	if !ok || fromID == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from_id", IsError: true}
	}

	// Use brand as from if not explicitly provided
	from := client.Brand()
	if fromParam, ok := params["from"].(string); ok && fromParam != "" {
		from = fromParam
	}

	ops, ok := params["ops"].(string)
	if !ok || ops == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: ops", IsError: true}
	}

	// Get the device operation from store
	deviceOp, err := t.deviceOpStore.GetOpsCommand(fromID, from, ops)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get device operation: %v", err), IsError: true}
	}

	// Parse the command JSON
	var commandParams map[string]any
	if err := json.Unmarshal([]byte(deviceOp.Param), &commandParams); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse command JSON: %v", err), IsError: true}
	}

	// Execute based on the method.
	// Normalize across naming conventions:
	//   Xiaomi spec_parser stores: "SetProp", "GetProp", "Action"
	//   Tuya skill stores:         "setProps", "getProps", "execute"
	var result any
	switch strings.ToLower(deviceOp.Method) {
	case "getprop", "getprops":
		result, err = client.GetProps(commandParams)
		if err != nil {
			return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to execute getProps: %v", err), IsError: true}
		}
	case "setprop", "setprops":
		result, err = client.SetProps(commandParams)
		if err != nil {
			return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to execute setProps: %v", err), IsError: true}
		}
	case "action", "execute":
		result, err = client.Execute(commandParams)
		if err != nil {
			return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to execute action: %v", err), IsError: true}
		}
	default:
		return &tools.ToolResult{ForLLM: fmt.Sprintf("unknown method: %s", deviceOp.Method), IsError: true}
	}

	b, _ := json.Marshal(result)
	return tools.NewToolResult(fmt.Sprintf("exe result (%s): %s", deviceOp.Method, string(b)))
}

// execMarkNoAction marks a device as non-operable by setting its Ops to ["NoAction"].
func (t *CLITool) execMarkNoAction(params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for markNoAction", IsError: true}
	}

	fromID, ok := params["from_id"].(string)
	if !ok || fromID == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from_id", IsError: true}
	}

	// Use brand as from if not explicitly provided
	from := ""
	if fromParam, ok := params["from"].(string); ok && fromParam != "" {
		from = fromParam
	}

	// Get all devices to find the target device
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}

	// Find and update the target device
	for _, device := range devices {
		if device.FromID == fromID && (from == "" || device.From == from) {
			device.Ops = []string{"NoAction"}
			if err := t.deviceStore.Save(device); err != nil {
				return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save device: %v", err), IsError: true}
			}
			return tools.NewToolResult(fmt.Sprintf("successfully marked device %s from %s as NoAction", fromID, device.From))
		}
	}

	return &tools.ToolResult{ForLLM: fmt.Sprintf("device not found: from_id=%s, from=%s", fromID, from), IsError: true}
}

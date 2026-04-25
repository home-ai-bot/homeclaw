package tool

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

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
	clients        map[string]third.Client
	homeStore      data.HomeStore
	spaceStore     data.SpaceStore
	deviceStore    data.DeviceStore
	deviceOpStore  data.DeviceOpStore
	authStore      data.AuthStore
	workspace      string
	refreshClients func() error // Optional callback to refresh brand clients after auth changes
}

// NewCLITool creates a CLITool with the given brand clients and data stores.
// clients maps brand name (e.g. "xiaomi", "tuya") to its third.Client.
func NewCLITool(
	homeStore data.HomeStore,
	spaceStore data.SpaceStore,
	deviceStore data.DeviceStore,
	deviceOpStore data.DeviceOpStore,
	authStore data.AuthStore,
) *CLITool {
	return &CLITool{
		homeStore:     homeStore,
		spaceStore:    spaceStore,
		deviceStore:   deviceStore,
		deviceOpStore: deviceOpStore,
		authStore:     authStore,
	}
}

// RegisterClient adds or replaces a brand client at runtime.
func (t *CLITool) RegisterClient(client third.Client) {
	if t.clients == nil {
		t.clients = make(map[string]third.Client)
	}
	t.clients[client.Brand()] = client
}

// SetClients sets the brand clients for device spec analysis.
// This can be called after construction to enable the analyzeAndSaveDeviceOps method.
func (t *CLITool) SetClients(clients map[string]third.Client) {
	t.clients = clients
}

// SetWorkspace sets the workspace path for loading reference files.
func (t *CLITool) SetWorkspace(workspace string) {
	t.workspace = workspace
}

// GetClients returns the map of brand clients.
func (t *CLITool) GetClients() map[string]third.Client {
	return t.clients
}

// SetRefreshClients sets the callback function used to refresh brand clients
// after authentication credentials are saved or deleted.
func (t *CLITool) SetRefreshClients(fn func() error) {
	t.refreshClients = fn
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
	case "listOps":
		return t.execListOps()
	case "markNoAction":
		return t.execMarkNoAction(req.Params)
	case "saveAuth":
		return t.execSaveAuth(req.Params)
	case "deleteAuth":
		return t.execDeleteAuth(req.Params)
	case "getAuthStatus":
		return t.execGetAuthStatus(req.Params)
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

	// Find existing current home for this brand to preserve selection
	existingCurrent, _ := t.homeStore.GetCurrent(client.Brand())

	dataHomes := make([]data.Home, 0, len(homes))
	for _, h := range homes {
		isCurrent := false
		if existingCurrent != nil && existingCurrent.FromID == h.ID {
			isCurrent = true
		}
		dataHomes = append(dataHomes, data.Home{
			FromID:  h.ID,
			From:    client.Brand(),
			Name:    h.Name,
			Current: isCurrent,
		})
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

	// Update devices: add new ones and update existing ones if URN is missing
	existingDevices, _ := t.deviceStore.GetAll()
	existingMap := make(map[string]*data.Device, len(existingDevices))
	for i := range existingDevices {
		key := existingDevices[i].FromID + "|" + existingDevices[i].From
		existingMap[key] = &existingDevices[i]
	}

	deviceValuesToSave := make([]data.Device, 0, len(devices))
	for _, d := range devices {
		if d == nil {
			continue
		}
		key := d.FromID + "|" + d.From
		if existing, exists := existingMap[key]; exists {
			// Device exists - update URN if new device has a valid URN
			// Always update if the new URN is non-empty (even if existing URN is not empty)
			// This ensures URN gets corrected from old category-based URNs to model-based URNs
			if d.URN != "" && existing.URN != d.URN {
				existing.URN = d.URN
				deviceValuesToSave = append(deviceValuesToSave, *existing)
				logger.Infof("[SyncDevices] Updated URN for device %s (%s): %s -> %s", d.FromID, d.From, existing.URN, d.URN)
			}
		} else {
			// New device - add it
			deviceValuesToSave = append(deviceValuesToSave, *d)
		}
	}
	if len(deviceValuesToSave) > 0 {
		t.deviceStore.Save(deviceValuesToSave...)
	}

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
// Operations are stored per device type (URN), not per device instance.
// Required params: from, urn, ops_array (JSON string)
// ops_array format: [{"ops":"turn_on","param_type":"bool","param_value":null,"method":"SetProp","method_param":{"did":"{{.deviceId}}","siid":2,"piid":1,"value":"{{.value}}"}}]
func (t *CLITool) execSaveDeviceOps(params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for saveDeviceOps", IsError: true}
	}

	// Extract from and urn from top-level params
	from, ok := params["from"].(string)
	if !ok || from == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from", IsError: true}
	}

	urn, ok := params["urn"].(string)
	if !ok || urn == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: urn", IsError: true}
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

	// Parse the JSON array - use the complete skill output format
	type opEntry struct {
		Ops         string          `json:"ops"`
		ParamType   string          `json:"param_type"`
		ParamValue  json.RawMessage `json:"param_value"`
		Method      string          `json:"method"`
		MethodParam json.RawMessage `json:"method_param"`
	}

	var opsArray []opEntry
	if err := json.Unmarshal([]byte(opsArrayJSON), &opsArray); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse ops_array JSON: %v", err), IsError: true}
	}

	if len(opsArray) == 0 {
		return &tools.ToolResult{ForLLM: "ops_array is empty", IsError: true}
	}

	// Load allowed ops for validation
	allowedOps := t.loadAllowedOps()
	if allowedOps == nil {
		logger.Warnf("[saveDeviceOps] ops.md not found, skipping ops validation")
	}

	// Convert to DeviceOp slice
	deviceOps := make([]data.DeviceOp, 0, len(opsArray))
	skippedOps := 0
	for _, entry := range opsArray {
		if len(entry.MethodParam) == 0 {
			continue
		}

		// Validate operation name against allowed ops list
		if allowedOps != nil && !allowedOps[entry.Ops] {
			logger.Warnf("[saveDeviceOps] skipping unknown operation '%s' (not in ops.md)", entry.Ops)
			skippedOps++
			continue
		}

		// Parse param_value into any type (null, bool, string, number, array, or object)
		var paramValue any
		if len(entry.ParamValue) > 0 {
			if err := json.Unmarshal(entry.ParamValue, &paramValue); err != nil {
				logger.Warnf("[saveDeviceOps] failed to parse param_value for op '%s': %v", entry.Ops, err)
				continue
			}
		}

		deviceOps = append(deviceOps, data.DeviceOp{
			URN:         urn,
			From:        from,
			Ops:         entry.Ops,
			ParamType:   entry.ParamType,
			ParamValue:  paramValue,
			Method:      entry.Method,
			MethodParam: string(entry.MethodParam),
		})
	}

	if len(deviceOps) == 0 {
		// Mark all devices with the same URN as NoAction
		if err := t.markDevicesByURNAsNoAction(urn, from); err != nil {
			logger.Warnf("[saveDeviceOps] failed to mark devices with URN %s as NoAction: %v", urn, err)
		}

		msg := "no valid operations to save - all devices with this URN marked as NoAction"
		if skippedOps > 0 {
			msg = fmt.Sprintf("all %d operations were skipped (not in ops.md) - all devices with this URN marked as NoAction", skippedOps)
		}
		return &tools.ToolResult{ForLLM: msg, IsError: true}
	}

	// Batch save all operations
	if err := t.deviceOpStore.Save(deviceOps...); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to batch save device operations: %v", err), IsError: true}
	}

	result := fmt.Sprintf("successfully saved %d device operations for URN %s from %s", len(deviceOps), urn, from)
	if skippedOps > 0 {
		result += fmt.Sprintf(" (skipped %d invalid operations)", skippedOps)
	}
	return tools.NewToolResult(result)
}

// loadAllowedOps loads the supported operations list from ops.md.
// Returns a map for O(1) lookup. If the file cannot be loaded, returns nil (validation skipped).
func (t *CLITool) loadAllowedOps() map[string]bool {
	if t.workspace == "" {
		return nil
	}

	workspacePaths := []string{
		filepath.Join(t.workspace, "skills", "device-spec-analyze", "reference"),
	}

	for _, basePath := range workspacePaths {
		filePath := filepath.Join(basePath, "ops.md")
		if content, err := os.ReadFile(filePath); err == nil {
			// ops.md is a JSON array: ["op1","op2",...]
			var ops []string
			if err := json.Unmarshal(content, &ops); err == nil {
				allowed := make(map[string]bool, len(ops))
				for _, op := range ops {
					allowed[op] = true
				}
				return allowed
			}
		}
	}

	return nil
}

// execListOps lists all devices with valid operations (ops not empty and not "NoAction").
// Returns devices grouped by room with their operations including param_type and param_value.
func (t *CLITool) execListOps() *tools.ToolResult {
	// Get all devices
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}

	// Get all spaces for room mapping
	spaces, err := t.spaceStore.GetAll()
	if err != nil {
		logger.Warnf("[listOps] failed to get spaces: %v", err)
		spaces = nil
	}

	// Build space name set for validation
	spaceSet := make(map[string]bool)
	if spaces != nil {
		for _, s := range spaces {
			spaceSet[s.Name] = true
		}
	}

	// Get all device operations
	allOps, err := t.deviceOpStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get device ops: %v", err), IsError: true}
	}

	// Build URN+From -> Ops map
	opsMap := make(map[string][]data.DeviceOp)
	for _, op := range allOps {
		key := op.URN + "|" + op.From
		opsMap[key] = append(opsMap[key], op)
	}

	// Filter devices with valid ops (not empty and not NoAction)
	// Group by room (space_name)
	type deviceWithOps struct {
		FromID    string          `json:"from_id"`
		From      string          `json:"from"`
		Name      string          `json:"name"`
		Type      string          `json:"type"`
		URN       string          `json:"urn"`
		SpaceName string          `json:"space_name"`
		Ops       []data.DeviceOp `json:"ops"`
	}

	type roomGroup struct {
		RoomName string          `json:"room_name"`
		Devices  []deviceWithOps `json:"devices"`
	}

	roomMap := make(map[string]*roomGroup)

	for _, d := range devices {
		// Skip devices without URN
		if d.URN == "" {
			continue
		}

		// Check if device has valid ops
		key := d.URN + "|" + d.From
		ops, exists := opsMap[key]
		if !exists || len(ops) == 0 {
			continue
		}

		// Skip NoAction devices
		if len(d.Ops) == 1 && d.Ops[0] == "NoAction" {
			continue
		}

		// Determine room name
		roomName := d.SpaceName
		if roomName == "" {
			roomName = "未分类"
		}

		// Add to room group
		if _, ok := roomMap[roomName]; !ok {
			roomMap[roomName] = &roomGroup{
				RoomName: roomName,
				Devices:  []deviceWithOps{},
			}
		}

		roomMap[roomName].Devices = append(roomMap[roomName].Devices, deviceWithOps{
			FromID:    d.FromID,
			From:      d.From,
			Name:      d.Name,
			Type:      d.Type,
			URN:       d.URN,
			SpaceName: roomName,
			Ops:       ops,
		})
	}

	// Convert map to sorted slice
	rooms := make([]roomGroup, 0, len(roomMap))
	for _, room := range roomMap {
		rooms = append(rooms, *room)
	}

	if len(rooms) == 0 {
		return tools.NewToolResult(`{"rooms":[],"message":"No devices with operations found"}`)
	}

	result, _ := json.Marshal(map[string]any{
		"rooms": rooms,
		"count": len(rooms),
	})
	return tools.NewToolResult(string(result))
}

// execListDevicesWithoutOps lists all devices that don't have any device operations saved.
// Returns full Device objects (including URN) and deduplicates by URN.
// Only filters devices with len(Ops) == 0 (devices with Ops[0] == "NoAction" are excluded from this list).
func (t *CLITool) execListDevicesWithoutOps(params map[string]any) *tools.ToolResult {
	// Get all devices
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}

	// Filter devices without operations (only len(Ops) == 0, skip NoAction devices)
	// Deduplicate by URN - same URN = same device type, only need one representative
	seenURNs := make(map[string]bool)
	var devicesWithoutOps []data.Device
	for _, d := range devices {
		// Only include devices with completely empty ops
		if len(d.Ops) == 0 && d.URN != "" {
			// Deduplicate by URN
			if !seenURNs[d.URN+"|"+d.From] {
				seenURNs[d.URN+"|"+d.From] = true
				devicesWithoutOps = append(devicesWithoutOps, d)
			}
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
// Uses URN-based lookup: finds device by from_id, gets its URN, then looks up DeviceOp by URN.
// MethodParam uses Go templates: {{.deviceId}} and {{.value}} are rendered at execution time.
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

	// Find the device to get its URN
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get devices: %v", err), IsError: true}
	}

	var deviceURN string
	for _, d := range devices {
		if d.FromID == fromID && d.From == from {
			deviceURN = d.URN
			break
		}
	}

	if deviceURN == "" {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("device not found: from_id=%s, from=%s", fromID, from), IsError: true}
	}

	// Get the device operation from store by URN
	deviceOp, err := t.deviceOpStore.GetOpsCommand(deviceURN, from, ops)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to get device operation: %v", err), IsError: true}
	}

	// Render the MethodParam template with device ID and optional value
	// Handle value as any type (bool, string, number, etc.) and convert to string for template
	var valueStr string
	if value := params["value"]; value != nil {
		switch v := value.(type) {
		case bool:
			valueStr = fmt.Sprintf("%t", v)
		case string:
			valueStr = v
		case float64:
			// JSON numbers are float64
			if v == float64(int(v)) {
				valueStr = fmt.Sprintf("%d", int(v))
			} else {
				valueStr = fmt.Sprintf("%f", v)
			}
		default:
			valueStr = fmt.Sprintf("%v", v)
		}
	}
	commandParams, err := t.renderMethodParam(deviceOp.MethodParam, fromID, valueStr)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to render method_param template: %v", err), IsError: true}
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

// renderMethodParam renders a Go template string with deviceId and value variables.
// Template format: {"did":"{{.deviceId}}","siid":2,"piid":1,"value":"{{.value}}"}
// Special handling: If value is "true"/"false" (no quotes in template), convert to bool.
func (t *CLITool) renderMethodParam(templateStr, deviceID, value string) (map[string]any, error) {
	tmpl, err := template.New("method_param").Parse(templateStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{
		"deviceId": deviceID,
		"value":    value,
	}); err != nil {
		return nil, fmt.Errorf("failed to execute template: %w", err)
	}

	var result map[string]any
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse rendered JSON: %w (JSON: %s)", err, buf.String())
	}

	// Post-process: convert string "true"/"false" to actual bool values
	// This handles the case where template has "{{.value}}" with quotes
	for key, val := range result {
		if strVal, ok := val.(string); ok {
			if strVal == "true" {
				result[key] = true
				logger.Debugf("[CLITool] renderMethodParam - Converted string 'true' to bool true for key: %s", key)
			} else if strVal == "false" {
				result[key] = false
				logger.Debugf("[CLITool] renderMethodParam - Converted string 'false' to bool false for key: %s", key)
			}
		}
	}

	return result, nil
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

// markDevicesByURNAsNoAction marks all devices with the same URN and brand as NoAction.
func (t *CLITool) markDevicesByURNAsNoAction(urn, from string) error {
	if t.deviceStore == nil {
		return fmt.Errorf("deviceStore is not initialized")
	}

	// Get all devices
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return fmt.Errorf("failed to get devices: %w", err)
	}

	// Find and update all devices with matching URN and brand
	markedCount := 0
	for _, device := range devices {
		if device.URN == urn && device.From == from {
			device.Ops = []string{"NoAction"}
			if err := t.deviceStore.Save(device); err != nil {
				logger.Warnf("[markDevicesByURNAsNoAction] failed to save device %s: %v", device.FromID, err)
				continue
			}
			markedCount++
			logger.Infof("[markDevicesByURNAsNoAction] marked device %s (URN: %s) as NoAction", device.FromID, urn)
		}
	}

	logger.Infof("[markDevicesByURNAsNoAction] marked %d devices with URN %s from %s as NoAction", markedCount, urn, from)
	return nil
}

// execSaveAuth stores authentication credentials (token or password) for a brand using AuthStore.
// Required params: brand, token (or password)
// Optional params: region, userName, extra (JSON string or map)
func (t *CLITool) execSaveAuth(params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for saveAuth", IsError: true}
	}

	// Extract required brand parameter
	brand, ok := params["brand"].(string)
	if !ok || brand == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: brand", IsError: true}
	}

	// Extract token/password (can be stored in 'token' or 'password' field)
	token, hasToken := params["token"].(string)
	if !hasToken || token == "" {
		// Try 'password' field as alternative
		password, hasPassword := params["password"].(string)
		if hasPassword && password != "" {
			token = password
		} else {
			return &tools.ToolResult{ForLLM: "missing required parameter: token or password", IsError: true}
		}
	}

	// Extract optional parameters
	region, _ := params["region"].(string)
	userName, _ := params["userName"].(string)

	// Extract extra fields (accept both string JSON and map)
	var extra map[string]string
	if extraRaw, ok := params["extra"]; ok && extraRaw != nil {
		if extraStr, ok := extraRaw.(string); ok && extraStr != "" {
			// Parse JSON string
			if err := json.Unmarshal([]byte(extraStr), &extra); err != nil {
				return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse 'extra' JSON: %v", err), IsError: true}
			}
		} else if extraMap, ok := extraRaw.(map[string]any); ok && len(extraMap) > 0 {
			// Convert map[string]any to map[string]string
			extra = make(map[string]string, len(extraMap))
			for k, v := range extraMap {
				extra[k] = fmt.Sprintf("%v", v)
			}
		}
	}

	// Save to AuthStore
	if err := t.authStore.SaveBrand(brand, region, userName, token, extra); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save auth for brand '%s': %v", brand, err), IsError: true}
	}

	// Refresh brand clients to pick up the new credentials
	if t.refreshClients != nil {
		if err := t.refreshClients(); err != nil {
			// Log the error but don't fail the operation - credentials are saved
			// The Tuya client supports lazy token loading, so it will work on next use
		}
	}

	return tools.NewToolResult(fmt.Sprintf("successfully saved auth credentials for brand '%s'", brand))
}

// execDeleteAuth removes authentication credentials for a brand using AuthStore.
// Required params: brand
func (t *CLITool) execDeleteAuth(params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for deleteAuth", IsError: true}
	}

	// Extract required brand parameter
	brand, ok := params["brand"].(string)
	if !ok || brand == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: brand", IsError: true}
	}

	// Check if brand exists
	if !t.authStore.Exists(brand) {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("no auth credentials found for brand '%s'", brand), IsError: true}
	}

	// Delete from AuthStore
	if err := t.authStore.DeleteBrand(brand); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to delete auth for brand '%s': %v", brand, err), IsError: true}
	}

	// Refresh brand clients to reflect the deletion
	if t.refreshClients != nil {
		if err := t.refreshClients(); err != nil {
			// Log the error but don't fail the operation
		}
	}

	return tools.NewToolResult(fmt.Sprintf("successfully deleted auth credentials for brand '%s'", brand))
}

// execGetAuthStatus returns the authentication status for a brand.
// Required params: brand
// Returns: JSON with logged_in status and brand info
func (t *CLITool) execGetAuthStatus(params map[string]any) *tools.ToolResult {
	if params == nil {
		return &tools.ToolResult{ForLLM: "missing 'params' for getAuthStatus", IsError: true}
	}

	// Extract required brand parameter
	brand, ok := params["brand"].(string)
	if !ok || brand == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: brand", IsError: true}
	}

	// Check if auth credentials exist
	exists := t.authStore.Exists(brand)

	// Build response
	result := map[string]any{
		"brand":     brand,
		"logged_in": exists,
	}

	if exists {
		// Get brand data (without decrypting token for security)
		authData, err := t.authStore.GetBrand(brand)
		if err == nil && authData != nil {
			result["has_credentials"] = true
			if authData.Region != "" {
				result["region"] = authData.Region
			}
			if authData.UserName != "" {
				result["username"] = authData.UserName
			}
		}
	} else {
		result["has_credentials"] = false
	}

	b, _ := json.Marshal(result)
	return tools.NewToolResult(string(b))
}

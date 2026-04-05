package tool

import (
	"context"
	"encoding/json"
	"fmt"

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
	clients     map[string]third.Client
	homeStore   data.HomeStore
	spaceStore  data.SpaceStore
	deviceStore data.DeviceStore
}

// NewCLITool creates a CLITool with the given brand clients and data stores.
// clients maps brand name (e.g. "xiaomi", "tuya") to its third.Client.
func NewCLITool(
	clients map[string]third.Client,
	homeStore data.HomeStore,
	spaceStore data.SpaceStore,
	deviceStore data.DeviceStore,
) *CLITool {
	return &CLITool{
		clients:     clients,
		homeStore:   homeStore,
		spaceStore:  spaceStore,
		deviceStore: deviceStore,
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
	return "Unified CLI tool for all smart-home brands. " +
		"Use commandJson to specify brand, method and params. " +
		"Supported methods: syncHomes, syncDevices, getProps, setProps, execute, getSpec,capImage,."
}

func (t *CLITool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"commandJson": map[string]any{
				"type": "string",
				"description": `JSON string with "brand", "method", and optional "params". Examples:
` +
					`syncHomes:   {"brand":"tuya","method":"syncHomes"}` + "\n" +
					`syncHomes:   {"brand":"xiaomi","method":"syncHomes"}` + "\n" +
					`syncDevices: {"brand":"tuya","method":"syncDevices","params":{"homeId":"xxx"}}` + "\n" +
					`getProps:    {"brand":"tuya","method":"getProps","params":{"device_id":"xxx"}}` + "\n" +
					`getProps:    {"brand":"xiaomi","method":"getProps","params":{"did":"xxx","siid":2,"piid":1}}` + "\n" +
					`setProps:    {"brand":"tuya","method":"setProps","params":{"device_id":"xxx","switch_led":true}}` + "\n" +
					`setProps:    {"brand":"xiaomi","method":"setProps","params":{"did":"xxx","siid":2,"piid":1,"value":true}}` + "\n" +
					`execute:     {"brand":"xiaomi","method":"execute","params":{"did":"xxx","siid":2,"aiid":1}}` + "\n" +
					`getSpec:     {"brand":"xiaomi","method":"getSpec","params":{"deviceId":"xxx"}}` + "\n" +
					`getSpec:     {"brand":"tuya","method":"getSpec","params":{"deviceId":"xxx"}}`,
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

	if req.Brand == "" {
		return &tools.ToolResult{ForLLM: "missing 'brand' in commandJson", IsError: true}
	}
	if req.Method == "" {
		return &tools.ToolResult{ForLLM: "missing 'method' in commandJson", IsError: true}
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
	default:
		return &tools.ToolResult{
			ForLLM:  fmt.Sprintf("unknown method '%s'; supported: syncHomes, syncDevices, getProps, setProps, execute, getSpec", req.Method),
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

// Package tool provides Xiaomi MIoT LLM tools for device sync and action execution.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio/util"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ────────────────────────────────────────────────────────────────────────────────
// SyncHomesTool - Sync homes from Xiaomi cloud
// ────────────────────────────────────────────────────────────────────────────────

// SyncHomesTool syncs homes from Xiaomi cloud.
type SyncHomesTool struct {
	client    *miio.MiClient
	homeStore data.HomeStore
}

// NewSyncHomesTool creates a SyncHomesTool backed by the given MiClient and HomeStore.
func NewSyncHomesTool(client *miio.MiClient, homeStore data.HomeStore) (*SyncHomesTool, error) {
	return &SyncHomesTool{client: client, homeStore: homeStore}, nil
}

func (t *SyncHomesTool) Name() string { return "mi__internal_1" }

func (t *SyncHomesTool) Description() string {
	return "must only invoked by mi-sync skill,when home is empty"
}

func (t *SyncHomesTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}

func (t *SyncHomesTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	homes, err := t.client.GetHomes()
	if err != nil {
		msg := fmt.Sprintf("failed to sync homes: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	if len(homes) == 0 {
		return tools.NewToolResult("no homes found in Xiaomi Mi Home")
	}

	// Convert to data.Home and save
	dataHomes := make([]data.Home, 0, len(homes))
	for _, h := range homes {
		dataHomes = append(dataHomes, data.Home{
			FromID:  h.ID,
			From:    miio.BrandXiaomi,
			Name:    h.Name,
			Current: false,
		})
	}

	// Set first home as current if only one home
	if len(dataHomes) == 1 {
		dataHomes[0].Current = true
	}

	if err := t.homeStore.Save(dataHomes...); err != nil {
		msg := fmt.Sprintf("failed to save homes: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	// Build result
	result := make([]map[string]string, 0, len(homes))
	for _, h := range homes {
		result = append(result, map[string]string{
			"home_id": h.ID,
			"name":    h.Name,
		})
	}
	b, _ := json.Marshal(result)
	return tools.NewToolResult(fmt.Sprintf("synced %d homes: %s", len(homes), string(b)))
}

// ────────────────────────────────────────────────────────────────────────────────
// SyncDevicesTool - Sync devices from Xiaomi cloud
// ────────────────────────────────────────────────────────────────────────────────

// SyncDevicesTool syncs devices from Xiaomi cloud for a given home.
type SyncDevicesTool struct {
	client      *miio.MiClient
	homeStore   data.HomeStore
	spaceStore  data.SpaceStore
	deviceStore data.DeviceStore
	specFetcher *miio.SpecFetcher
}

// NewSyncDevicesTool creates a SyncDevicesTool backed by the given MiClient.
func NewSyncDevicesTool(
	client *miio.MiClient,
	homeStore data.HomeStore,
	spaceStore data.SpaceStore,
	deviceStore data.DeviceStore,
	specFetcher *miio.SpecFetcher,
) *SyncDevicesTool {
	return &SyncDevicesTool{
		client:      client,
		homeStore:   homeStore,
		spaceStore:  spaceStore,
		deviceStore: deviceStore,
		specFetcher: specFetcher,
	}
}

func (t *SyncDevicesTool) Name() string { return "mi__internal_2" }

func (t *SyncDevicesTool) Description() string {
	return "must only invoked by mi_sync skill, when mi-sync detemine which homeId should be sync"
}

func (t *SyncDevicesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"homeId": map[string]any{
				"type":        "string",
				"description": "The Xiaomi home ID to sync devices from",
			},
		},
		"required": []string{"homeId"},
	}
}

func (t *SyncDevicesTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	homeID, ok := args["homeId"].(string)
	if !ok || homeID == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'homeId' parameter", IsError: true}
	}

	err := t.homeStore.SetCurrent(homeID, miio.BrandXiaomi)
	if err != nil {
		msg := fmt.Sprintf("failed to set current home: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}
	devices, err := t.client.GetDevices(homeID)
	if err != nil {
		msg := fmt.Sprintf("failed to sync devices: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	// Sync rooms to SpaceStore
	rooms, err := t.client.GetRooms(homeID)
	if err != nil {
		msg := fmt.Sprintf("failed to sync rooms: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}
	if len(rooms) > 0 {
		// Convert []*Space to []Space for batch save
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

	// Get existing devices to avoid overwriting
	existingDevices, _ := t.deviceStore.GetAll()
	existingSet := make(map[string]struct{}, len(existingDevices))
	for _, ed := range existingDevices {
		existingSet[ed.FromID+"|"+ed.From] = struct{}{}
	}

	// Convert []*Device to []Device for batch save, skip existing ones
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
	if len(deviceValues) > 0 {
		t.deviceStore.Save(deviceValues...)
	}

	// Process and save spec for all devices
	var specProcessed int
	var specErrors []string
	for _, d := range devices {
		if d == nil || d.URN == "" {
			continue
		}
		// Get spec for this device
		spec, err := t.client.GetSpec(d.FromID)
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
		// Generate device commands JSON
		commandsJSON, err := parsedSpec.GenerateDeviceCommandsCompactJSON(d.FromID)
		if err != nil {
			specErrors = append(specErrors, fmt.Sprintf("%s: generate commands error - %v", d.Name, err))
			continue
		}
		// Save processed spec as _new.json
		if t.specFetcher != nil {
			if err := t.specFetcher.SaveProcessedSpec(d.URN, commandsJSON); err != nil {
				specErrors = append(specErrors, fmt.Sprintf("%s: save error - %v", d.Name, err))
				continue
			}
		}
		specProcessed++
	}

	b, _ := json.Marshal(devices)
	summary := fmt.Sprintf("synced %d devices: %s", len(devices), string(b))
	if specProcessed > 0 {
		summary += fmt.Sprintf("\nProcessed specs for %d devices", specProcessed)
	}
	if len(specErrors) > 0 {
		summary += fmt.Sprintf("\nSpec errors: %s", strings.Join(specErrors, "; "))
	}
	return tools.NewToolResult(summary)
}

// ────────────────────────────────────────────────────────────────────────────────
// ExecuteActionTool - Execute action/get/set property on a Xiaomi device
// ────────────────────────────────────────────────────────────────────────────────

// ExecuteActionTool executes MIoT commands (Action/GetProp/SetProp) on a Xiaomi device.
type ExecuteActionTool struct {
	client *miio.MiClient
}

// NewExecuteActionTool creates an ExecuteActionTool backed by the given MiClient.
func NewExecuteActionTool(client *miio.MiClient) *ExecuteActionTool {
	return &ExecuteActionTool{client: client}
}

func (t *ExecuteActionTool) Name() string { return "mi_execute_action" }

func (t *ExecuteActionTool) Description() string {
	return "send commond to xiaomi device ,use by control xiaomi device,before use should find target device,get one its action from actions"
}

func (t *ExecuteActionTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"actionJson": map[string]any{
				"type":        "string",
				"description": `JSON string with method and param. Example: {"method":"SetProp","param":{"did":"xxx","siid":2,"piid":1,"value":true}}`,
			},
		},
		"required": []string{"actionJson"},
	}
}

// actionRequest represents the JSON structure for action requests.
type actionRequest struct {
	Method string         `json:"method"`
	Param  map[string]any `json:"param"`
}

func (t *ExecuteActionTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	actionJson, ok := args["actionJson"].(string)
	if !ok || actionJson == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'actionJson' parameter", IsError: true}
	}

	var req actionRequest
	if err := json.Unmarshal([]byte(actionJson), &req); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse actionJson: %v", err), IsError: true}
	}

	if req.Method == "" {
		return &tools.ToolResult{ForLLM: "missing 'method' in actionJson", IsError: true}
	}
	if req.Param == nil {
		return &tools.ToolResult{ForLLM: "missing 'param' in actionJson", IsError: true}
	}

	var result any
	var err error

	switch req.Method {
	case "GetProp":
		result, err = t.client.GetProps(req.Param)
	case "SetProp":
		result, err = t.client.SetProps(req.Param)
	case "Action":
		result, err = t.client.Execute(req.Param)
	default:
		return &tools.ToolResult{ForLLM: fmt.Sprintf("unknown method '%s', expected 'Action', 'GetProp', or 'SetProp'", req.Method), IsError: true}
	}

	if err != nil {
		msg := fmt.Sprintf("failed to execute %s: %v", req.Method, err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	b, _ := json.Marshal(result)
	return tools.NewToolResult(fmt.Sprintf("%s result: %s", req.Method, string(b)))
}

// ────────────────────────────────────────────────────────────────────────────────
// GetSpecCommandsTool - Get processed spec commands by URN
// ────────────────────────────────────────────────────────────────────────────────

// GetSpecCommandsTool reads the processed spec commands from _new.json file by URN.
type GetSpecCommandsTool struct {
	specFetcher *miio.SpecFetcher
}

// NewGetSpecCommandsTool creates a GetSpecCommandsTool backed by the given SpecFetcher.
func NewGetSpecCommandsTool(specFetcher *miio.SpecFetcher) *GetSpecCommandsTool {
	return &GetSpecCommandsTool{specFetcher: specFetcher}
}

func (t *GetSpecCommandsTool) Name() string { return "mi_get_spec_commands" }

func (t *GetSpecCommandsTool) Description() string {
	return "Get processed device commands by URN. Returns the pre-processed device commands JSON from _new.json cache."
}

func (t *GetSpecCommandsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"urn": map[string]any{
				"type":        "string",
				"description": "Device URN (e.g., urn:miot-spec-v2:device:light:0000A001:yeelink-v1)",
			},
		},
		"required": []string{"urn"},
	}
}

func (t *GetSpecCommandsTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	urn, ok := args["urn"].(string)
	if !ok || urn == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'urn' parameter", IsError: true}
	}

	if t.specFetcher == nil {
		return &tools.ToolResult{ForLLM: "spec fetcher not initialized", IsError: true}
	}

	commandsJSON, err := t.specFetcher.GetProcessedSpec(urn)
	if err != nil {
		msg := fmt.Sprintf("failed to get processed spec for URN %s: %v", urn, err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	return tools.NewToolResult(commandsJSON)
}

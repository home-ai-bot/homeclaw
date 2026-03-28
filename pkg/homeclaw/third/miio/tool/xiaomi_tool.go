// Package tool provides Xiaomi MIoT LLM tools for device sync and action execution.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third/miio/util"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ────────────────────────────────────────────────────────────────────────────────
// SyncDevicesTool - Sync devices from Xiaomi cloud
// ────────────────────────────────────────────────────────────────────────────────

// SyncDevicesTool syncs devices from Xiaomi cloud for a given home.
type SyncDevicesTool struct {
	client      *miio.MiClient
	homeStore   data.HomeStore
	spaceStore  data.SpaceStore
	deviceStore data.DeviceStore
}

// NewSyncDevicesTool creates a SyncDevicesTool backed by the given MiClient.
func NewSyncDevicesTool(
	client *miio.MiClient,
	homeStore data.HomeStore,
	spaceStore data.SpaceStore,
	deviceStore data.DeviceStore,
) *SyncDevicesTool {
	return &SyncDevicesTool{
		client:      client,
		homeStore:   homeStore,
		spaceStore:  spaceStore,
		deviceStore: deviceStore,
	}
}

func (t *SyncDevicesTool) Name() string { return "mi_sync_devices" }

func (t *SyncDevicesTool) Description() string {
	return "Sync all devices from Xiaomi/Mi Home cloud for a specific home. Returns the list of synced devices with their details."
}

func (t *SyncDevicesTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *SyncDevicesTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	homeID := ""
	// Try to get current home from homeStore
	currentHome, err := t.homeStore.GetCurrent(miio.BrandXiaomi)
	if err == nil && currentHome != nil {
		homeID = currentHome.FromID
	}

	if homeID == "" {
		// Get all Xiaomi homes from local store
		allHomes, _ := t.homeStore.GetAll()
		var xiaomiHomes []data.Home
		for _, h := range allHomes {
			if h.From == miio.BrandXiaomi {
				xiaomiHomes = append(xiaomiHomes, h)
			}
		}

		// If no local homes, fetch from cloud and save
		if len(xiaomiHomes) == 0 {
			cloudHomes, err := t.client.GetHomes()
			if err != nil {
				msg := fmt.Sprintf("failed to fetch homes from cloud: %v", err)
				return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
			}
			if len(cloudHomes) == 0 {
				msg := "No homes found for this Xiaomi account. Please create a home in Mi Home app first."
				return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
			}
			// Convert and save to homeStore
			homesToSave := make([]data.Home, 0, len(cloudHomes))
			for _, ch := range cloudHomes {
				homesToSave = append(homesToSave, data.Home{
					FromID: ch.ID,
					From:   miio.BrandXiaomi,
					Name:   ch.Name,
				})
			}
			if err := t.homeStore.Save(homesToSave...); err != nil {
				msg := fmt.Sprintf("failed to save homes: %v", err)
				return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
			}
			xiaomiHomes = homesToSave
		}

		// Handle based on number of homes
		if len(xiaomiHomes) == 0 {
			msg := "No homes found for this Xiaomi account. Please create a home in Mi Home app first."
			return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
		} else if len(xiaomiHomes) == 1 {
			// Only one home, set as current automatically
			homeID = xiaomiHomes[0].FromID
			_ = t.homeStore.SetCurrent(homeID, miio.BrandXiaomi)
		} else {
			// Multiple homes, ask user to choose
			var homeList strings.Builder
			homeList.WriteString("Multiple homes found. Please specify a home_id:\n")
			for _, h := range xiaomiHomes {
				currentMark := ""
				if h.Current {
					currentMark = " (current)"
				}
				homeList.WriteString(fmt.Sprintf("- %s: %s%s\n", h.FromID, h.Name, currentMark))
			}
			msg := homeList.String()
			return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
		}
	}
	if homeID != "" {
		err := t.homeStore.SetCurrent(homeID, miio.BrandXiaomi)
		if err != nil {
			msg := fmt.Sprintf("failed to set current home: %v", err)
			return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
		}
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

	b, _ := json.Marshal(devices)
	return tools.NewToolResult(fmt.Sprintf("synced %d devices: %s", len(devices), string(b)))
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
	return `Execute MIoT commands on a Xiaomi device via JSON. 
Supports three methods:
- Action: {"method":"Action","param":{"did":"xxx","siid":2,"aiid":1,"in":[...]}}
- GetProp: {"method":"GetProp","param":{"did":"xxx","siid":2,"piid":1}}
- SetProp: {"method":"SetProp","param":{"did":"xxx","siid":2,"piid":1,"value":true}}`
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
// GenActionsTool - Generate actions for Xiaomi devices with empty actions
// ────────────────────────────────────────────────────────────────────────────────

// GenActionsTool generates actions for Xiaomi devices that have empty actions.
type GenActionsTool struct {
	client         *miio.MiClient
	deviceStore    data.DeviceStore
	intentProvider providers.LLMProvider
	intentModel    string
}

// NewGenActionsTool creates a GenActionsTool backed by the given MiClient.
func NewGenActionsTool(
	client *miio.MiClient,
	deviceStore data.DeviceStore,
	intentProvider providers.LLMProvider,
	intentModel string,
) *GenActionsTool {
	return &GenActionsTool{
		client:         client,
		deviceStore:    deviceStore,
		intentProvider: intentProvider,
		intentModel:    intentModel,
	}
}

func (t *GenActionsTool) Name() string { return "mi_gen_actions" }

func (t *GenActionsTool) Description() string {
	return "Generate action mappings for Xiaomi devices that have empty actions. This uses LLM to analyze device specs and create action commands."
}

func (t *GenActionsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *GenActionsTool) Execute(ctx context.Context, args map[string]any) *tools.ToolResult {
	allDevices, err := t.deviceStore.GetAll()
	if err != nil {
		msg := fmt.Sprintf("failed to get devices: %v", err)
		return &tools.ToolResult{ForLLM: msg, ForUser: msg, IsError: true}
	}

	var generated int
	var skipped int
	var results []string

	for _, dev := range allDevices {
		if len(dev.Actions) > 0 {
			skipped++
			continue // Skip devices that already have actions
		}
		if dev.From != miio.BrandXiaomi {
			continue // Only handle Xiaomi devices
		}

		// Get device spec
		spec, err := t.client.GetSpec(dev.FromID)
		if err != nil {
			results = append(results, fmt.Sprintf("%s: spec unavailable", dev.Name))
			continue
		}

		// Generate actions using LLM
		actions := t.generateActionsWithLLM(ctx, &dev, spec.Raw)
		if len(actions) > 0 {
			t.deviceStore.SetActions(dev.FromID, dev.From, actions)
			generated++
			results = append(results, fmt.Sprintf("%s: %d actions generated", dev.Name, len(actions)))
		} else {
			results = append(results, fmt.Sprintf("%s: no actions generated", dev.Name))
		}
	}

	summary := fmt.Sprintf("Generated actions for %d devices, skipped %d devices with existing actions", generated, skipped)
	if len(results) > 0 {
		summary += "\nDetails:\n" + strings.Join(results, "\n")
	}
	return tools.NewToolResult(summary)
}

// generateActionsWithLLM uses LLM to map device spec to standard actions
func (t *GenActionsTool) generateActionsWithLLM(ctx context.Context, dev *data.Device, specJSON string) string {
	prompt := t.buildActionGenerationPrompt(dev, specJSON)

	messages := []providers.Message{
		{Role: "system", Content: "You are a smart home device expert. Generate action mappings in JSON format only."},
		{Role: "user", Content: prompt},
	}

	resp, err := t.intentProvider.Chat(ctx, messages, nil, t.intentModel, nil)
	if err != nil || resp == nil || resp.Content == "" {
		return ""
	}

	return t.parseActionsResponse(resp.Content, dev.FromID)
}

// buildActionGenerationPrompt builds the prompt for action generation
func (t *GenActionsTool) buildActionGenerationPrompt(dev *data.Device, specJSON string) string {
	return fmt.Sprintf(`Based on the device spec below, generate action mappings for this Xiaomi device.
Device Info:
- DID: %s
- Name: %s
- Type/Model: %s
- URN: %s

Device MIoT Spec (JSON):
%s

Available Standard Actions:
%s

Generate a JSON array object mapping standard action names to MIoT commands.
Format example: 
1. {"start": {"method":"Action","param":{"did":"#did","siid":2,"aiid":1,"in":[true,1]}}}
2. {"get_state": {"method":"GetProp","param":{"did":"#did","siid":2,"piid":1}}}
3. {"turn_on": {"method":"SetProp","param":{"did":"#did","siid":2,"piid":1,"value":true}}}

Rules:
1. Only include actions that the device actually supports based on its spec
2. services sub iid is siid
   services sub properties sub iid is piid,generate 2,3,value must follow format,must related to Available Standard Actions
   services sub actions sub iid is siid,generate 1, in must follow in ; must related to Available Standard Actions
3. start\get_state\turn_on must in Available Standard Actions
4. some device may have multiple entities, you need to generate actions for each entity,such as light+fan
5. ignore  properties\actions not related to Available Standard Actions

Output ONLY the JSON object array, no explanation:`,
		dev.FromID, dev.Name, dev.Type, dev.URN, specJSON, config.DeviceActionsJSON)
}

// parseActionsResponse extracts action mappings from LLM response
func (t *GenActionsTool) parseActionsResponse(content string, did string) string {
	logger.Info(content)
	content = strings.TrimSpace(content)

	// Handle markdown code blocks
	if strings.HasPrefix(content, "```") {
		lines := strings.Split(content, "\n")
		var jsonLines []string
		inBlock := false
		for _, line := range lines {
			if strings.HasPrefix(line, "```") {
				inBlock = !inBlock
				continue
			}
			if inBlock {
				jsonLines = append(jsonLines, line)
			}
		}
		content = strings.Join(jsonLines, "\n")
	}
	// 使用 util.ValidJson 验证 JSON
	valid, err := util.ValidJson(content)
	if err != nil {
		logger.Errorf("ValidJson parse error: %v", err)
	} else if len(valid.ActionErrors) > 0 || len(valid.MethondErrors) > 0 {
		logger.Errorf("ValidJson validation errors - ActionErrors: %v, MethodErrors: %v", valid.ActionErrors, valid.MethondErrors)
	}
	return content
}

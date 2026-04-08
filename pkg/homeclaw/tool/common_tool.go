package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_common
// ─────────────────────────────────────────────────────────────────────────────

// CommonTool provides common utility methods that are brand-agnostic.
// These methods operate on local data stores without requiring brand-specific clients.
//
// commandJson schema:
//
//	{
//	  "method": "listDevices" | …,
//	  "params": { … }   // optional
//	}
//
// listDevices – list all registered smart devices with full details.
type CommonTool struct {
	deviceStore data.DeviceStore
}

// NewCommonTool creates a CommonTool with the given data stores.
func NewCommonTool(deviceStore data.DeviceStore) *CommonTool {
	return &CommonTool{
		deviceStore: deviceStore,
	}
}

func (t *CommonTool) Name() string { return "hc_common" }

func (t *CommonTool) Description() string {
	return "Common utility tool for getting information.\n" +
		"Available methods:\n" +
		"- listDevices: lists all registered smart devices with full details (no params required)\n" +
		"Usage: commandJson = {\"method\": \"listDevices\"}"
}

func (t *CommonTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"commandJson": map[string]any{
				"type":        "string",
				"description": `JSON string with "method" and optional "params". Do NOT fabricate or guess any json, must follow skill`,
			},
		},
		"required": []string{"commandJson"},
	}
}

// commonCommandRequest is the parsed form of the commandJson argument.
type commonCommandRequest struct {
	Method string         `json:"method"`
	Params map[string]any `json:"params"`
}

func (t *CommonTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	commandJson, ok := args["commandJson"].(string)
	if !ok || commandJson == "" {
		return &tools.ToolResult{ForLLM: "missing or invalid 'commandJson' parameter", IsError: true}
	}

	var req commonCommandRequest
	if err := json.Unmarshal([]byte(commandJson), &req); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to parse commandJson: %v", err), IsError: true}
	}

	if req.Method == "" {
		return &tools.ToolResult{ForLLM: "missing 'method' in commandJson", IsError: true}
	}

	switch req.Method {
	case "listDevices":
		return t.execListDevices()
	default:
		return &tools.ToolResult{
			ForLLM:  fmt.Sprintf("unknown method '%s'; tool must invoke by skills, please use the right skill!", req.Method),
			IsError: true,
		}
	}
}

// execListDevices lists all registered smart devices with full details.
func (t *CommonTool) execListDevices() *tools.ToolResult {
	devices, err := t.deviceStore.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list devices: %v", err), IsError: true}
	}
	b, _ := json.Marshal(devices)
	return tools.NewToolResult(string(b))
}

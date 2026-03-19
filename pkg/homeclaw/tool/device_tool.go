// Package tool provides HomeClaw LLM tools for devices, spaces, members and workflows.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_list_devices
// ─────────────────────────────────────────────────────────────────────────────

// deviceSummary is a lightweight view returned by hc_list_devices.
type deviceSummary struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Brand    string `json:"brand"`
	RoomName string `json:"room_name"`
}

// ListDevicesTool lists all registered smart devices.
type ListDevicesTool struct {
	store data.DeviceStore
}

// NewListDevicesTool creates a ListDevicesTool backed by the given DeviceStore.
func NewListDevicesTool(store data.DeviceStore) *ListDevicesTool {
	return &ListDevicesTool{store: store}
}

func (t *ListDevicesTool) Name() string { return "hc_list_devices" }

func (t *ListDevicesTool) Description() string {
	return "List registered HomeClaw smart devices. Optionally filter by room_name and/or brand. Returns device ID, name, brand and room name for each matched device."
}

func (t *ListDevicesTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"room_name": map[string]any{
				"type":        "string",
				"description": "Optional. Filter devices by room name (case-insensitive).",
			},
			"brand": map[string]any{
				"type":        "string",
				"description": "Optional. Filter devices by brand (case-insensitive), e.g. \"mijia\", \"tuya\".",
			},
		},
		"required": []string{},
	}
}

func (t *ListDevicesTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	filterRoom, _ := args["room_name"].(string)
	filterBrand, _ := args["brand"].(string)

	devices, err := t.store.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list devices: %v", err), IsError: true}
	}
	summaries := make([]deviceSummary, 0, len(devices))
	for _, d := range devices {
		if filterRoom != "" && !strings.EqualFold(d.RoomName, filterRoom) {
			continue
		}
		if filterBrand != "" && !strings.EqualFold(d.Brand, filterBrand) {
			continue
		}
		summaries = append(summaries, deviceSummary{
			ID:       d.ID,
			Name:     d.Name,
			Brand:    d.Brand,
			RoomName: d.RoomName,
		})
	}
	b, _ := json.Marshal(summaries)
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_get_device
// ─────────────────────────────────────────────────────────────────────────────

// GetDeviceTool fetches a single device by ID.
type GetDeviceTool struct {
	store data.DeviceStore
}

func NewGetDeviceTool(store data.DeviceStore) *GetDeviceTool {
	return &GetDeviceTool{store: store}
}

func (t *GetDeviceTool) Name() string { return "hc_get_device" }

func (t *GetDeviceTool) Description() string {
	return "Get full details of a specific HomeClaw device by its ID, including all fields and current state."
}

func (t *GetDeviceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"device_id": map[string]any{
				"type":        "string",
				"description": "The device ID to retrieve",
			},
		},
		"required": []string{"device_id"},
	}
}

func (t *GetDeviceTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	id, ok := args["device_id"].(string)
	if !ok || id == "" {
		return &tools.ToolResult{ForLLM: "device_id is required", IsError: true}
	}
	device, err := t.store.GetByID(id)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("device not found: %v", err), IsError: true}
	}
	b, _ := json.Marshal(device)
	return tools.NewToolResult(string(b))
}

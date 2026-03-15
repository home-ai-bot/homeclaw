// Package tool provides HomeClaw LLM tools for devices, spaces, members and workflows.
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_list_devices
// ─────────────────────────────────────────────────────────────────────────────

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
	return "List all registered HomeClaw smart devices. Returns device IDs, names, models, protocols, capabilities and their current states."
}

func (t *ListDevicesTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *ListDevicesTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	devices, err := t.store.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list devices: %v", err), IsError: true}
	}
	b, _ := json.Marshal(devices)
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
	return "Get details of a specific HomeClaw device by its ID, including current state and capabilities."
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

// ─────────────────────────────────────────────────────────────────────────────
// hc_save_device
// ─────────────────────────────────────────────────────────────────────────────

// SaveDeviceTool creates or updates a device record.
type SaveDeviceTool struct {
	store data.DeviceStore
}

func NewSaveDeviceTool(store data.DeviceStore) *SaveDeviceTool {
	return &SaveDeviceTool{store: store}
}

func (t *SaveDeviceTool) Name() string { return "hc_save_device" }

func (t *SaveDeviceTool) Description() string {
	return "Create or update a HomeClaw device record. Provide a JSON object matching the Device schema (id, name, brand, protocol, model, space_id, ip, token, capabilities)."
}

func (t *SaveDeviceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"device": map[string]any{
				"type":        "object",
				"description": "Device object to save. Must include an 'id' field.",
				"properties": map[string]any{
					"id":           map[string]any{"type": "string"},
					"name":         map[string]any{"type": "string"},
					"brand":        map[string]any{"type": "string"},
					"protocol":     map[string]any{"type": "string"},
					"model":        map[string]any{"type": "string"},
					"space_id":     map[string]any{"type": "string"},
					"ip":           map[string]any{"type": "string"},
					"token":        map[string]any{"type": "string"},
					"capabilities": map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
				},
				"required": []string{"id", "name"},
			},
		},
		"required": []string{"device"},
	}
}

func (t *SaveDeviceTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	raw, ok := args["device"]
	if !ok {
		return &tools.ToolResult{ForLLM: "device object is required", IsError: true}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to serialize device: %v", err), IsError: true}
	}
	var device data.Device
	if err := json.Unmarshal(b, &device); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("invalid device object: %v", err), IsError: true}
	}
	if device.ID == "" {
		return &tools.ToolResult{ForLLM: "device.id is required", IsError: true}
	}
	if err := t.store.Save(device); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save device: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("device %q saved successfully", device.ID))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_update_device_state
// ─────────────────────────────────────────────────────────────────────────────

// UpdateDeviceStateTool updates only the state fields of a device.
type UpdateDeviceStateTool struct {
	store data.DeviceStore
}

func NewUpdateDeviceStateTool(store data.DeviceStore) *UpdateDeviceStateTool {
	return &UpdateDeviceStateTool{store: store}
}

func (t *UpdateDeviceStateTool) Name() string { return "hc_update_device_state" }

func (t *UpdateDeviceStateTool) Description() string {
	return "Update the state of a HomeClaw device (e.g. turn on/off, adjust brightness). Provide the device ID and a state map."
}

func (t *UpdateDeviceStateTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"device_id": map[string]any{
				"type":        "string",
				"description": "The device ID",
			},
			"state": map[string]any{
				"type":                 "object",
				"description":          "Key-value state fields to set (e.g. {\"power\": \"on\", \"brightness\": 80})",
				"additionalProperties": true,
			},
		},
		"required": []string{"device_id", "state"},
	}
}

func (t *UpdateDeviceStateTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	id, ok := args["device_id"].(string)
	if !ok || id == "" {
		return &tools.ToolResult{ForLLM: "device_id is required", IsError: true}
	}
	stateRaw, ok := args["state"]
	if !ok {
		return &tools.ToolResult{ForLLM: "state is required", IsError: true}
	}
	state, ok := stateRaw.(map[string]interface{})
	if !ok {
		return &tools.ToolResult{ForLLM: "state must be an object", IsError: true}
	}
	if err := t.store.UpdateState(id, state); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to update device state: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("device %q state updated", id))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_delete_device
// ─────────────────────────────────────────────────────────────────────────────

// DeleteDeviceTool removes a device from the registry.
type DeleteDeviceTool struct {
	store data.DeviceStore
}

func NewDeleteDeviceTool(store data.DeviceStore) *DeleteDeviceTool {
	return &DeleteDeviceTool{store: store}
}

func (t *DeleteDeviceTool) Name() string { return "hc_delete_device" }

func (t *DeleteDeviceTool) Description() string {
	return "Delete a HomeClaw device from the registry by its ID."
}

func (t *DeleteDeviceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"device_id": map[string]any{
				"type":        "string",
				"description": "The device ID to delete",
			},
		},
		"required": []string{"device_id"},
	}
}

func (t *DeleteDeviceTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	id, ok := args["device_id"].(string)
	if !ok || id == "" {
		return &tools.ToolResult{ForLLM: "device_id is required", IsError: true}
	}
	if err := t.store.Delete(id); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to delete device: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("device %q deleted", id))
}

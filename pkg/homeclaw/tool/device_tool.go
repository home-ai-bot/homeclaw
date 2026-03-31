// Package tool provides HomeClaw LLM tools for devices, spaces, members and workflows.
package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ListDevicesTool lists all registered smart devices with full details.
type ListDevicesTool struct {
	store data.DeviceStore
}

// NewListDevicesTool creates a ListDevicesTool backed by the given DeviceStore.
func NewListDevicesTool(store data.DeviceStore) *ListDevicesTool {
	return &ListDevicesTool{store: store}
}

func (t *ListDevicesTool) Name() string { return "hc_list_devices" }

func (t *ListDevicesTool) Description() string {
	return "List all  smart devices "
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
// hc_list_cameras
// ─────────────────────────────────────────────────────────────────────────────

// ListCamerasTool lists all camera devices with RTSP stream URLs.
type ListCamerasTool struct {
	store data.DeviceStore
}

// NewListCamerasTool creates a ListCamerasTool backed by the given DeviceStore.
func NewListCamerasTool(store data.DeviceStore) *ListCamerasTool {
	return &ListCamerasTool{store: store}
}

func (t *ListCamerasTool) Name() string { return "hc_list_cameras" }

func (t *ListCamerasTool) Description() string {
	return "List all camera devices with their RTSP stream URLs. " +
		"Returns cameras including standard cameras, smart doorbells (cateye), and pet feeders with cameras."
}

func (t *ListCamerasTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *ListCamerasTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	devices, err := t.store.GetAll()
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

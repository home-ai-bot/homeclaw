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

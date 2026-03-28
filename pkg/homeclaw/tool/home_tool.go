// Package tool provides HomeClaw LLM tools for devices, spaces, members and workflows.
package tool

import (
	"context"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// SetCurrentHomeTool sets the current home for a specific brand and ID.
type SetCurrentHomeTool struct {
	store data.HomeStore
}

// NewSetCurrentHomeTool creates a SetCurrentHomeTool backed by the given HomeStore.
func NewSetCurrentHomeTool(store data.HomeStore) *SetCurrentHomeTool {
	return &SetCurrentHomeTool{store: store}
}

func (t *SetCurrentHomeTool) Name() string { return "hc_set_current_home" }

func (t *SetCurrentHomeTool) Description() string {
	return "Set the current home for a specific brand and ID. This marks the specified home as current and unsets all other homes of the same brand."
}

func (t *SetCurrentHomeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from_id": map[string]any{
				"type":        "string",
				"description": "The home ID from the brand",
			},
			"from": map[string]any{
				"type":        "string",
				"description": "The brand/source name (e.g. 'xiaomi', 'homekit')",
			},
		},
		"required": []string{"from_id", "from"},
	}
}

func (t *SetCurrentHomeTool) Execute(_ context.Context, params map[string]any) *tools.ToolResult {
	fromID, ok := params["from_id"].(string)
	if !ok || fromID == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from_id", IsError: true}
	}

	from, ok := params["from"].(string)
	if !ok || from == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from", IsError: true}
	}

	if err := t.store.SetCurrent(fromID, from); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to set current home: %v", err), IsError: true}
	}

	return tools.NewToolResult(fmt.Sprintf("successfully set home %s from %s as current", fromID, from))
}

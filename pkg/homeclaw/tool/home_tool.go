// Package tool provides HomeClaw LLM tools for devices, spaces, members and workflows.
package tool

import (
	"context"
	"encoding/json"
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

// GetCurrentHomeTool retrieves the current home for a specific brand.
type GetCurrentHomeTool struct {
	store data.HomeStore
}

// NewGetCurrentHomeTool creates a GetCurrentHomeTool backed by the given HomeStore.
func NewGetCurrentHomeTool(store data.HomeStore) *GetCurrentHomeTool {
	return &GetCurrentHomeTool{store: store}
}

func (t *GetCurrentHomeTool) Name() string { return "hc_get_current_home" }

func (t *GetCurrentHomeTool) Description() string {
	return "Get the current home for a specific brand. Returns the home ID, name, and brand information."
}

func (t *GetCurrentHomeTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"from": map[string]any{
				"type":        "string",
				"description": "The brand/source name (e.g. 'xiaomi', 'homekit')",
			},
		},
		"required": []string{"from"},
	}
}

func (t *GetCurrentHomeTool) Execute(_ context.Context, params map[string]any) *tools.ToolResult {
	from, ok := params["from"].(string)
	if !ok || from == "" {
		return &tools.ToolResult{ForLLM: "missing required parameter: from", IsError: true}
	}

	home, err := t.store.GetCurrent(from)
	if err != nil {
		// Check if there are any homes for this brand
		allHomes, _ := t.store.GetAll()
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

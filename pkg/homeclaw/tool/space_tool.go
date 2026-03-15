package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_list_spaces
// ─────────────────────────────────────────────────────────────────────────────

// ListSpacesTool lists all spaces (floors, rooms, areas) in the home.
type ListSpacesTool struct {
	store data.SpaceStore
}

func NewListSpacesTool(store data.SpaceStore) *ListSpacesTool {
	return &ListSpacesTool{store: store}
}

func (t *ListSpacesTool) Name() string { return "hc_list_spaces" }

func (t *ListSpacesTool) Description() string {
	return "List all HomeClaw spaces (floors, rooms, areas) including their IDs, names, types and child spaces."
}

func (t *ListSpacesTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *ListSpacesTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	spaces, err := t.store.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list spaces: %v", err), IsError: true}
	}
	b, _ := json.Marshal(spaces)
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_get_space
// ─────────────────────────────────────────────────────────────────────────────

// GetSpaceTool fetches a single space by ID or name.
type GetSpaceTool struct {
	store data.SpaceStore
}

func NewGetSpaceTool(store data.SpaceStore) *GetSpaceTool {
	return &GetSpaceTool{store: store}
}

func (t *GetSpaceTool) Name() string { return "hc_get_space" }

func (t *GetSpaceTool) Description() string {
	return "Get a HomeClaw space by its ID or by name (case-insensitive). Provide either space_id or space_name."
}

func (t *GetSpaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"space_id": map[string]any{
				"type":        "string",
				"description": "Space ID to look up",
			},
			"space_name": map[string]any{
				"type":        "string",
				"description": "Space name to search for (case-insensitive)",
			},
		},
		"required": []string{},
	}
}

func (t *GetSpaceTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	if id, ok := args["space_id"].(string); ok && id != "" {
		space, err := t.store.GetByID(id)
		if err != nil {
			return &tools.ToolResult{ForLLM: fmt.Sprintf("space not found: %v", err), IsError: true}
		}
		b, _ := json.Marshal(space)
		return tools.NewToolResult(string(b))
	}
	if name, ok := args["space_name"].(string); ok && name != "" {
		space, err := t.store.FindByName(name)
		if err != nil {
			return &tools.ToolResult{ForLLM: fmt.Sprintf("space not found: %v", err), IsError: true}
		}
		b, _ := json.Marshal(space)
		return tools.NewToolResult(string(b))
	}
	return &tools.ToolResult{ForLLM: "either space_id or space_name is required", IsError: true}
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_save_space
// ─────────────────────────────────────────────────────────────────────────────

// SaveSpaceTool creates or updates a space.
type SaveSpaceTool struct {
	store data.SpaceStore
}

func NewSaveSpaceTool(store data.SpaceStore) *SaveSpaceTool {
	return &SaveSpaceTool{store: store}
}

func (t *SaveSpaceTool) Name() string { return "hc_save_space" }

func (t *SaveSpaceTool) Description() string {
	return "Create or update a HomeClaw space (floor, room, area). Provide a space object with id, name and type."
}

func (t *SaveSpaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"space": map[string]any{
				"type":        "object",
				"description": "Space object to save",
				"properties": map[string]any{
					"id":   map[string]any{"type": "string"},
					"name": map[string]any{"type": "string"},
					"type": map[string]any{
						"type":        "string",
						"description": "One of: floor, room, area",
					},
				},
				"required": []string{"id", "name", "type"},
			},
		},
		"required": []string{"space"},
	}
}

func (t *SaveSpaceTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	raw, ok := args["space"]
	if !ok {
		return &tools.ToolResult{ForLLM: "space object is required", IsError: true}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to serialize space: %v", err), IsError: true}
	}
	var space data.Space
	if err := json.Unmarshal(b, &space); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("invalid space object: %v", err), IsError: true}
	}
	if space.ID == "" {
		return &tools.ToolResult{ForLLM: "space.id is required", IsError: true}
	}
	if err := t.store.Save(space); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save space: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("space %q saved successfully", space.ID))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_delete_space
// ─────────────────────────────────────────────────────────────────────────────

// DeleteSpaceTool removes a space.
type DeleteSpaceTool struct {
	store data.SpaceStore
}

func NewDeleteSpaceTool(store data.SpaceStore) *DeleteSpaceTool {
	return &DeleteSpaceTool{store: store}
}

func (t *DeleteSpaceTool) Name() string { return "hc_delete_space" }

func (t *DeleteSpaceTool) Description() string {
	return "Delete a HomeClaw space (floor, room, area) by its ID."
}

func (t *DeleteSpaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"space_id": map[string]any{
				"type":        "string",
				"description": "The space ID to delete",
			},
		},
		"required": []string{"space_id"},
	}
}

func (t *DeleteSpaceTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	id, ok := args["space_id"].(string)
	if !ok || id == "" {
		return &tools.ToolResult{ForLLM: "space_id is required", IsError: true}
	}
	if err := t.store.Delete(id); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to delete space: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("space %q deleted", id))
}

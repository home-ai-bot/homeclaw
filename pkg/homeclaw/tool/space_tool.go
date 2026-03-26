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
	return "List all HomeClaw spaces. Each space has a name (primary key), from (source, e.g. xiaomi/manual), and from_id (source-side ID)."
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

// ───────────────────────────────────────────────────────────────────────────────
// hc_get_space
// ───────────────────────────────────────────────────────────────────────────────

// GetSpaceTool retrieves a space by name.
type GetSpaceTool struct {
	store data.SpaceStore
}

func NewGetSpaceTool(store data.SpaceStore) *GetSpaceTool {
	return &GetSpaceTool{store: store}
}

func (t *GetSpaceTool) Name() string { return "hc_get_space" }

func (t *GetSpaceTool) Description() string {
	return "Get a HomeClaw space by name."
}

func (t *GetSpaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Space name to look up",
			},
		},
		"required": []string{"name"},
	}
}

func (t *GetSpaceTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return &tools.ToolResult{ForLLM: "name is required", IsError: true}
	}
	spaces, err := t.store.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list spaces: %v", err), IsError: true}
	}
	for _, sp := range spaces {
		if sp.Name == name {
			b, _ := json.Marshal(sp)
			return tools.NewToolResult(string(b))
		}
	}
	return &tools.ToolResult{ForLLM: fmt.Sprintf("space %q not found", name), IsError: true}
}

// ───────────────────────────────────────────────────────────────────────────────
// hc_save_space
// ───────────────────────────────────────────────────────────────────────────────

// SaveSpaceTool creates or updates a space.
type SaveSpaceTool struct {
	store data.SpaceStore
}

func NewSaveSpaceTool(store data.SpaceStore) *SaveSpaceTool {
	return &SaveSpaceTool{store: store}
}

func (t *SaveSpaceTool) Name() string { return "hc_save_space" }

func (t *SaveSpaceTool) Description() string {
	return "Create or update a HomeClaw space. Provide a space object with name (required) and optional from info."
}

func (t *SaveSpaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"space": map[string]any{
				"type":        "object",
				"description": "Space object to save",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"from": map[string]any{
						"type":        "object",
						"description": "Source info, e.g. {\"name\": \"xiaomi\", \"id\": \"123\"}",
					},
				},
				"required": []string{"name"},
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
	if space.Name == "" {
		return &tools.ToolResult{ForLLM: "space.name is required", IsError: true}
	}
	if err := t.store.Save(space); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save space: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("space %q saved successfully", space.Name))
}

// ───────────────────────────────────────────────────────────────────────────────
// hc_delete_space
// ───────────────────────────────────────────────────────────────────────────────

// DeleteSpaceTool removes a space by name.
type DeleteSpaceTool struct {
	store data.SpaceStore
}

func NewDeleteSpaceTool(store data.SpaceStore) *DeleteSpaceTool {
	return &DeleteSpaceTool{store: store}
}

func (t *DeleteSpaceTool) Name() string { return "hc_delete_space" }

func (t *DeleteSpaceTool) Description() string {
	return "Delete a HomeClaw space by name."
}

func (t *DeleteSpaceTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The space name to delete",
			},
		},
		"required": []string{"name"},
	}
}

func (t *DeleteSpaceTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return &tools.ToolResult{ForLLM: "name is required", IsError: true}
	}
	if err := t.store.Delete(name); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to delete space: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("space %q deleted", name))
}

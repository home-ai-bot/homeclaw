package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_list_members
// ─────────────────────────────────────────────────────────────────────────────

// ListMembersTool lists all family members registered in HomeClaw.
type ListMembersTool struct {
	store data.MemberStore
}

func NewListMembersTool(store data.MemberStore) *ListMembersTool {
	return &ListMembersTool{store: store}
}

func (t *ListMembersTool) Name() string { return "hc_list_members" }

func (t *ListMembersTool) Description() string {
	return "List all HomeClaw family members, including their roles, space permissions and channel bindings."
}

func (t *ListMembersTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *ListMembersTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	members, err := t.store.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list members: %v", err), IsError: true}
	}
	b, _ := json.Marshal(members)
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_get_member
// ─────────────────────────────────────────────────────────────────────────────

// GetMemberTool retrieves a member by name or by channel binding.
type GetMemberTool struct {
	store data.MemberStore
}

func NewGetMemberTool(store data.MemberStore) *GetMemberTool {
	return &GetMemberTool{store: store}
}

func (t *GetMemberTool) Name() string { return "hc_get_member" }

func (t *GetMemberTool) Description() string {
	return "Get a HomeClaw family member by name, or look up by channel + channel_user_id binding."
}

func (t *GetMemberTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "Member name to look up",
			},
		},
		"required": []string{"name"},
	}
}

func (t *GetMemberTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	members, err := t.store.GetAll()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list members: %v", err), IsError: true}
	}
	if name, ok := args["name"].(string); ok && name != "" {
		for _, m := range members {
			if m.Name == name {
				b, _ := json.Marshal(m)
				return tools.NewToolResult(string(b))
			}
		}
		return &tools.ToolResult{ForLLM: fmt.Sprintf("member %q not found", name), IsError: true}
	}
	return &tools.ToolResult{ForLLM: "name is required", IsError: true}
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_save_member
// ─────────────────────────────────────────────────────────────────────────────

// SaveMemberTool creates or updates a family member record.
type SaveMemberTool struct {
	store data.MemberStore
}

func NewSaveMemberTool(store data.MemberStore) *SaveMemberTool {
	return &SaveMemberTool{store: store}
}

func (t *SaveMemberTool) Name() string { return "hc_save_member" }

func (t *SaveMemberTool) Description() string {
	return "Create or update a HomeClaw family member. Provide a member object with name, role and optional channel bindings."
}

func (t *SaveMemberTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"member": map[string]any{
				"type":        "object",
				"description": "Member object to save",
				"properties": map[string]any{
					"name": map[string]any{"type": "string"},
					"role": map[string]any{
						"type":        "string",
						"description": "admin or member",
					},
					"my_spaces": map[string]any{
						"type":        "array",
						"items":       map[string]any{"type": "string"},
						"description": "Space names this member can access.",
					},
					"sleep_space": map[string]any{"type": "string", "description": "The space where this member sleeps."},
				},
				"required": []string{"name", "role"},
			},
		},
		"required": []string{"member"},
	}
}

func (t *SaveMemberTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	raw, ok := args["member"]
	if !ok {
		return &tools.ToolResult{ForLLM: "member object is required", IsError: true}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to serialize member: %v", err), IsError: true}
	}
	var member data.Member
	if err := json.Unmarshal(b, &member); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("invalid member object: %v", err), IsError: true}
	}
	if member.Name == "" {
		return &tools.ToolResult{ForLLM: "member.name is required", IsError: true}
	}
	if err := t.store.Save(member); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save member: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("member %q saved successfully", member.Name))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_delete_member
// ─────────────────────────────────────────────────────────────────────────────

// DeleteMemberTool removes a family member record.
type DeleteMemberTool struct {
	store data.MemberStore
}

func NewDeleteMemberTool(store data.MemberStore) *DeleteMemberTool {
	return &DeleteMemberTool{store: store}
}

func (t *DeleteMemberTool) Name() string { return "hc_delete_member" }

func (t *DeleteMemberTool) Description() string {
	return "Delete a HomeClaw family member by their name."
}

func (t *DeleteMemberTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name": map[string]any{
				"type":        "string",
				"description": "The member name to delete",
			},
		},
		"required": []string{"name"},
	}
}

func (t *DeleteMemberTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	name, ok := args["name"].(string)
	if !ok || name == "" {
		return &tools.ToolResult{ForLLM: "name is required", IsError: true}
	}
	if err := t.store.Delete(name); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to delete member: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("member %q deleted", name))
}

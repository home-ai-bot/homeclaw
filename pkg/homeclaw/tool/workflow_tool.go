package tool

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_list_workflows
// ─────────────────────────────────────────────────────────────────────────────

// ListWorkflowsTool lists all workflow metadata entries.
type ListWorkflowsTool struct {
	store data.WorkflowStore
}

func NewListWorkflowsTool(store data.WorkflowStore) *ListWorkflowsTool {
	return &ListWorkflowsTool{store: store}
}

func (t *ListWorkflowsTool) Name() string { return "hc_list_workflows" }

func (t *ListWorkflowsTool) Description() string {
	return "List all HomeClaw workflow metadata (ID, name, description, enabled status). Use hc_get_workflow to retrieve full workflow definitions."
}

func (t *ListWorkflowsTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []string{},
	}
}

func (t *ListWorkflowsTool) Execute(_ context.Context, _ map[string]any) *tools.ToolResult {
	metas, err := t.store.GetAllMeta()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to list workflows: %v", err), IsError: true}
	}
	b, _ := json.Marshal(metas)
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_get_workflow
// ─────────────────────────────────────────────────────────────────────────────

// GetWorkflowTool loads the full workflow definition by ID.
type GetWorkflowTool struct {
	store data.WorkflowStore
}

func NewGetWorkflowTool(store data.WorkflowStore) *GetWorkflowTool {
	return &GetWorkflowTool{store: store}
}

func (t *GetWorkflowTool) Name() string { return "hc_get_workflow" }

func (t *GetWorkflowTool) Description() string {
	return "Get the full definition of a HomeClaw workflow by its ID, including triggers, steps and variants."
}

func (t *GetWorkflowTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{
				"type":        "string",
				"description": "The workflow ID to retrieve",
			},
		},
		"required": []string{"workflow_id"},
	}
}

func (t *GetWorkflowTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	id, ok := args["workflow_id"].(string)
	if !ok || id == "" {
		return &tools.ToolResult{ForLLM: "workflow_id is required", IsError: true}
	}
	def, err := t.store.GetByID(id)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("workflow not found: %v", err), IsError: true}
	}
	b, _ := json.Marshal(def)
	return tools.NewToolResult(string(b))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_save_workflow
// ─────────────────────────────────────────────────────────────────────────────

// SaveWorkflowTool creates or updates a workflow definition.
type SaveWorkflowTool struct {
	store data.WorkflowStore
}

func NewSaveWorkflowTool(store data.WorkflowStore) *SaveWorkflowTool {
	return &SaveWorkflowTool{store: store}
}

func (t *SaveWorkflowTool) Name() string { return "hc_save_workflow" }

func (t *SaveWorkflowTool) Description() string {
	return "Create or update a HomeClaw workflow definition. Provide the full workflow JSON with id, name, description, triggers, steps and optional variants."
}

func (t *SaveWorkflowTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow": map[string]any{
				"type":        "object",
				"description": "Full WorkflowDef object. Must include id, name. Steps define the automation logic.",
				"properties": map[string]any{
					"id":          map[string]any{"type": "string"},
					"name":        map[string]any{"type": "string"},
					"description": map[string]any{"type": "string"},
					"triggers":    map[string]any{"type": "array"},
					"steps":       map[string]any{"type": "array"},
				},
				"required": []string{"id", "name"},
			},
			"created_by": map[string]any{
				"type":        "string",
				"description": "Creator identifier (e.g. member name or 'llm')",
			},
		},
		"required": []string{"workflow"},
	}
}

func (t *SaveWorkflowTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	raw, ok := args["workflow"]
	if !ok {
		return &tools.ToolResult{ForLLM: "workflow object is required", IsError: true}
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to serialize workflow: %v", err), IsError: true}
	}
	var def data.WorkflowDef
	if err := json.Unmarshal(b, &def); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("invalid workflow object: %v", err), IsError: true}
	}
	if def.ID == "" {
		return &tools.ToolResult{ForLLM: "workflow.id is required", IsError: true}
	}
	createdBy, _ := args["created_by"].(string)
	if createdBy == "" {
		createdBy = "llm"
	}
	if err := t.store.Save(&def, createdBy); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to save workflow: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("workflow %q saved successfully", def.ID))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_delete_workflow
// ─────────────────────────────────────────────────────────────────────────────

// DeleteWorkflowTool removes a workflow by ID.
type DeleteWorkflowTool struct {
	store data.WorkflowStore
}

func NewDeleteWorkflowTool(store data.WorkflowStore) *DeleteWorkflowTool {
	return &DeleteWorkflowTool{store: store}
}

func (t *DeleteWorkflowTool) Name() string { return "hc_delete_workflow" }

func (t *DeleteWorkflowTool) Description() string {
	return "Delete a HomeClaw workflow by its ID."
}

func (t *DeleteWorkflowTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{
				"type":        "string",
				"description": "The workflow ID to delete",
			},
		},
		"required": []string{"workflow_id"},
	}
}

func (t *DeleteWorkflowTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	id, ok := args["workflow_id"].(string)
	if !ok || id == "" {
		return &tools.ToolResult{ForLLM: "workflow_id is required", IsError: true}
	}
	if err := t.store.Delete(id); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to delete workflow: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("workflow %q deleted", id))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_enable_workflow / hc_disable_workflow
// ─────────────────────────────────────────────────────────────────────────────

// EnableWorkflowTool enables a workflow.
type EnableWorkflowTool struct {
	store data.WorkflowStore
}

func NewEnableWorkflowTool(store data.WorkflowStore) *EnableWorkflowTool {
	return &EnableWorkflowTool{store: store}
}

func (t *EnableWorkflowTool) Name() string { return "hc_enable_workflow" }

func (t *EnableWorkflowTool) Description() string {
	return "Enable a disabled HomeClaw workflow so it can be matched and executed."
}

func (t *EnableWorkflowTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "Workflow ID to enable"},
		},
		"required": []string{"workflow_id"},
	}
}

func (t *EnableWorkflowTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	id, _ := args["workflow_id"].(string)
	if id == "" {
		return &tools.ToolResult{ForLLM: "workflow_id is required", IsError: true}
	}
	if err := t.store.Enable(id); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to enable workflow: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("workflow %q enabled", id))
}

// DisableWorkflowTool disables a workflow.
type DisableWorkflowTool struct {
	store data.WorkflowStore
}

func NewDisableWorkflowTool(store data.WorkflowStore) *DisableWorkflowTool {
	return &DisableWorkflowTool{store: store}
}

func (t *DisableWorkflowTool) Name() string { return "hc_disable_workflow" }

func (t *DisableWorkflowTool) Description() string {
	return "Disable a HomeClaw workflow so it will not be matched or executed."
}

func (t *DisableWorkflowTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"workflow_id": map[string]any{"type": "string", "description": "Workflow ID to disable"},
		},
		"required": []string{"workflow_id"},
	}
}

func (t *DisableWorkflowTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	id, _ := args["workflow_id"].(string)
	if id == "" {
		return &tools.ToolResult{ForLLM: "workflow_id is required", IsError: true}
	}
	if err := t.store.Disable(id); err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to disable workflow: %v", err), IsError: true}
	}
	return tools.NewToolResult(fmt.Sprintf("workflow %q disabled", id))
}

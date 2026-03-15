package intent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/workflow"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
)

// DeviceControlIntent handles all device.control.* intents.
//
// Processing logic (mirrors 意图识别设计.md §4.1):
//  1. Query the WorkflowStore for a workflow whose triggers match the user input.
//  2. If a match is found, execute it via workflow.Engine and return the result.
//  3. If no match is found, return Handled=false so the agent loop falls through
//     to the large model, which will generate a new workflow.
type DeviceControlIntent struct {
	workflowStore data.WorkflowStore
	engine        workflow.Engine
	// provider is the small LLM used for workflow matching.
	provider providers.LLMProvider
	// modelName is the model identifier passed to provider when making calls.
	modelName string
}

// NewDeviceControlIntent creates a DeviceControlIntent.
// workflowStore and engine are required; provider/modelName are used for
// LLM-based workflow matching and may be nil/empty (falls back gracefully).
func NewDeviceControlIntent(store data.WorkflowStore, eng workflow.Engine, provider providers.LLMProvider, modelName string) *DeviceControlIntent {
	return &DeviceControlIntent{
		workflowStore: store,
		engine:        eng,
		provider:      provider,
		modelName:     modelName,
	}
}

// Types implements Intent.
// DeviceControlIntent handles all device.control.* subtypes.
func (d *DeviceControlIntent) Types() []IntentType {
	return []IntentType{
		IntentDeviceControlSingle,
		IntentDeviceControlScene,
		IntentDeviceControlGlobal,
		IntentDeviceControlCorrect,
	}
}

// Run executes the device control intent.
func (d *DeviceControlIntent) Run(ctx context.Context, ictx IntentContext) IntentResponse {
	if d.workflowStore == nil || d.engine == nil {
		// Infrastructure not available – fall through to large model.
		return IntentResponse{Handled: false}
	}

	// Try to find a matching workflow via LLM-based matching.
	wf, err := d.matchWorkflow(ctx, ictx.UserInput)
	if err != nil {
		logger.ErrorCF("intent", "device control workflow match error",
			map[string]any{"error": err.Error(), "input": ictx.UserInput})
		return IntentResponse{Handled: false}
	}

	if wf == nil {
		// No match found – ask the large model to generate a workflow.
		logger.InfoCF("intent", "no matching workflow, falling through to large model",
			map[string]any{"input": ictx.UserInput})
		return IntentResponse{Handled: false}
	}

	// Execute the matched workflow.
	execCtx := data.ExecutionContext{
		MemberName: ictx.SenderID,
		TriggerBy:  "intent",
		Input: map[string]interface{}{
			"user_input": ictx.UserInput,
			"entities":   ictx.Result.Entities,
		},
	}

	record, err := d.engine.Execute(ctx, wf, execCtx)
	if err != nil {
		logger.ErrorCF("intent", "workflow execution failed",
			map[string]any{"workflow_id": wf.ID, "error": err.Error()})
		return IntentResponse{
			Handled:  true,
			Response: fmt.Sprintf("执行工作流「%s」时出现错误，请稍后重试。", wf.Name),
			Error:    err,
		}
	}

	msg := buildExecutionSummary(wf.Name, record)
	return IntentResponse{Handled: true, Response: msg}
}

// matchWorkflow finds the best-matching workflow for the given user input
// using the small LLM provider.  It builds a concise catalogue of enabled
// workflows (id / name / description) and asks the model to return the best
// matching ID as JSON {"id":"<workflow-id>"}.  If no workflow fits, the model
// should return {"id":""} and the method returns nil.
// Falls back to nil (no match) on any error so the caller can fall through
// safely to the large model.
func (d *DeviceControlIntent) matchWorkflow(ctx context.Context, userInput string) (*data.WorkflowDef, error) {
	metas, err := d.workflowStore.GetAllMeta()
	if err != nil {
		return nil, fmt.Errorf("list workflows: %w", err)
	}

	// Build enabled-only catalogue.
	var catalogue strings.Builder
	for _, m := range metas {
		if !m.Enabled {
			continue
		}
		catalogue.WriteString(fmt.Sprintf("- id=%q  name=%q  description=%q\n", m.ID, m.Name, m.Description))
	}
	if catalogue.Len() == 0 {
		return nil, nil
	}

	systemPrompt := `你是一个工作流匹配助手。根据用户指令，从候选工作流列表中选出最匹配的一个。
只输出 JSON，格式为 {"id":"<workflow-id>"}。
如果没有合适的工作流，输出 {"id":""}。
不要输出任何其他内容。`

	userMsg := fmt.Sprintf("候选工作流：\n%s\n用户指令：%s", catalogue.String(), userInput)

	messages := []providers.Message{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userMsg},
	}

	mn := d.modelName
	if mn == "" {
		mn = d.provider.GetDefaultModel()
	}

	resp, err := d.provider.Chat(ctx, messages, nil, mn, map[string]any{
		"max_tokens":  64,
		"temperature": 0.0,
	})
	if err != nil {
		logger.WarnCF("intent", "workflow match LLM error, falling through",
			map[string]any{"error": err.Error()})
		return nil, nil
	}
	if resp == nil || resp.Content == "" {
		return nil, nil
	}

	// Extract JSON from response.
	raw := extractJSON(resp.Content)
	if raw == "" {
		return nil, nil
	}

	var result struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal([]byte(raw), &result); err != nil || result.ID == "" {
		return nil, nil
	}

	wf, err := d.workflowStore.GetByID(result.ID)
	if err != nil {
		return nil, fmt.Errorf("load workflow %s: %w", result.ID, err)
	}
	return wf, nil
}

// scoreWorkflow gives a rough relevance score between 0 and 100.
// Kept as fallback; not used when provider is available.
func scoreWorkflow(m *data.WorkflowMeta, userInput string, hints []string) int {
	score := 0
	target := m.Name + " " + m.Description

	for _, hint := range hints {
		if hint == "" {
			continue
		}
		if contains(target, hint) {
			score += 30
		}
	}
	if contains(target, userInput) {
		score += 20
	}
	return score
}

// contains reports whether s contains substr (case-insensitive ASCII).
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		indexInsensitive(s, substr) >= 0
}

func indexInsensitive(s, substr string) int {
	sLow := strings.ToLower(s)
	subLow := strings.ToLower(substr)
	return indexOf(sLow, subLow)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// buildExecutionSummary creates a brief human-readable summary of a workflow run.
func buildExecutionSummary(workflowName string, record *data.ExecutionRecord) string {
	if record == nil {
		return fmt.Sprintf("已执行工作流「%s」。", workflowName)
	}
	if !record.Success {
		return fmt.Sprintf("工作流「%s」执行失败：%s", workflowName, record.Error)
	}
	return fmt.Sprintf("已完成：%s（共执行 %d 步）。", workflowName, len(record.StepExecutions))
}

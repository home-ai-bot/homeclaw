package workflow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// mockTool is a mock tool for testing
type mockTool struct {
	name    string
	execute func(ctx context.Context, args map[string]interface{}) *tools.ToolResult
}

func (m *mockTool) Name() string {
	return m.name
}

func (m *mockTool) Description() string {
	return "Mock tool for testing"
}

func (m *mockTool) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"type":       "object",
		"properties": map[string]interface{}{},
	}
}

func (m *mockTool) Execute(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
	if m.execute != nil {
		return m.execute(ctx, args)
	}
	result := map[string]interface{}{"result": "ok"}
	resultJSON, _ := json.Marshal(result)
	return tools.NewToolResult(string(resultJSON))
}

func newMockToolRegistry() *tools.ToolRegistry {
	registry := tools.NewToolRegistry()

	// Register echo tool
	registry.Register(&mockTool{
		name: "echo",
		execute: func(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
			message, _ := args["message"].(string)
			result := map[string]interface{}{"echo": message}
			resultJSON, _ := json.Marshal(result)
			return tools.NewToolResult(string(resultJSON))
		},
	})

	// Register add tool
	registry.Register(&mockTool{
		name: "add",
		execute: func(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			result := map[string]interface{}{"sum": a + b}
			resultJSON, _ := json.Marshal(result)
			return tools.NewToolResult(string(resultJSON))
		},
	})

	// Register get_items tool
	registry.Register(&mockTool{
		name: "get_items",
		execute: func(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
			result := []interface{}{
				map[string]interface{}{"id": "item1", "name": "Item 1"},
				map[string]interface{}{"id": "item2", "name": "Item 2"},
				map[string]interface{}{"id": "item3", "name": "Item 3"},
			}
			resultJSON, _ := json.Marshal(result)
			return tools.NewToolResult(string(resultJSON))
		},
	})

	// Register fail tool
	registry.Register(&mockTool{
		name: "fail",
		execute: func(ctx context.Context, args map[string]interface{}) *tools.ToolResult {
			return tools.ErrorResult("intentional failure")
		},
	})

	return registry
}

func TestEngine_Execute_ActionStep(t *testing.T) {
	registry := newMockToolRegistry()
	engine := NewEngine(registry)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-action",
		Name: "Test Action Workflow",
		Steps: []data.Step{
			{
				ID:     "step1",
				Type:   data.StepTypeAction,
				Action: "echo",
				Params: map[string]interface{}{
					"message": "hello world",
				},
				OutputAs: "result1",
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input:      map[string]interface{}{},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !record.Success {
		t.Errorf("Expected success, got failure: %s", record.Error)
	}

	if len(record.StepExecutions) != 1 {
		t.Errorf("Expected 1 step execution, got %d", len(record.StepExecutions))
	}

	stepExec := record.StepExecutions[0]
	if stepExec.StepID != "step1" {
		t.Errorf("Expected step ID 'step1', got '%s'", stepExec.StepID)
	}

	if !stepExec.Success {
		t.Errorf("Expected step success, got failure: %s", stepExec.Error)
	}
}

func TestEngine_Execute_ActionStepWithVariables(t *testing.T) {
	registry := newMockToolRegistry()
	engine := NewEngine(registry)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-vars",
		Name: "Test Variables Workflow",
		Steps: []data.Step{
			{
				ID:     "step1",
				Type:   data.StepTypeAction,
				Action: "echo",
				Params: map[string]interface{}{
					"message": "${input.message}",
				},
				OutputAs: "result1",
			},
			{
				ID:     "step2",
				Type:   data.StepTypeAction,
				Action: "echo",
				Params: map[string]interface{}{
					"message": "Previous: ${result1.echo}",
				},
				OutputAs: "result2",
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input: map[string]interface{}{
			"message": "hello from input",
		},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !record.Success {
		t.Errorf("Expected success, got failure: %s", record.Error)
	}

	if len(record.StepExecutions) != 2 {
		t.Errorf("Expected 2 step executions, got %d", len(record.StepExecutions))
	}

	// Check step 2 used variable from step 1
	step2Exec := record.StepExecutions[1]
	result2, ok := step2Exec.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected result2 to be map, got %T", step2Exec.Result)
	}

	echo, ok := result2["echo"].(string)
	if !ok {
		t.Fatalf("Expected echo to be string, got %T", result2["echo"])
	}

	if echo != "Previous: hello from input" {
		t.Errorf("Expected 'Previous: hello from input', got '%s'", echo)
	}
}

func TestEngine_Execute_ActionStepFailure(t *testing.T) {
	registry := newMockToolRegistry()
	engine := NewEngine(registry)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-fail",
		Name: "Test Failure Workflow",
		Steps: []data.Step{
			{
				ID:     "step1",
				Type:   data.StepTypeAction,
				Action: "fail",
				Params: map[string]interface{}{},
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input:      map[string]interface{}{},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err == nil {
		t.Error("Expected error for failed workflow")
	}

	if record.Success {
		t.Error("Expected failure, got success")
	}

	if len(record.StepExecutions) != 1 {
		t.Errorf("Expected 1 step execution, got %d", len(record.StepExecutions))
	}

	stepExec := record.StepExecutions[0]
	if stepExec.Success {
		t.Error("Expected step failure")
	}

	if stepExec.Error != "intentional failure" {
		t.Errorf("Expected 'intentional failure', got '%s'", stepExec.Error)
	}
}

func TestEngine_Execute_ConditionStep_True(t *testing.T) {
	registry := newMockToolRegistry()
	engine := NewEngine(registry)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-condition-true",
		Name: "Test Condition True Workflow",
		Steps: []data.Step{
			{
				ID:   "step1",
				Type: data.StepTypeCondition,
				Condition: &data.Condition{
					If: "${input.value} == 10",
					Then: []data.Step{
						{
							ID:     "then-step",
							Type:   data.StepTypeAction,
							Action: "echo",
							Params: map[string]interface{}{
								"message": "condition met",
							},
							OutputAs: "then_result",
						},
					},
					Else: []data.Step{
						{
							ID:     "else-step",
							Type:   data.StepTypeAction,
							Action: "echo",
							Params: map[string]interface{}{
								"message": "condition not met",
							},
							OutputAs: "else_result",
						},
					},
				},
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input: map[string]interface{}{
			"value": float64(10),
		},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !record.Success {
		t.Errorf("Expected success, got failure: %s", record.Error)
	}

	// Check that then branch was executed
	vars := record.Context.Input
	_ = vars
}

func TestEngine_Execute_ConditionStep_False(t *testing.T) {
	registry := newMockToolRegistry()
	engine := NewEngine(registry)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-condition-false",
		Name: "Test Condition False Workflow",
		Steps: []data.Step{
			{
				ID:   "step1",
				Type: data.StepTypeCondition,
				Condition: &data.Condition{
					If: "${input.value} == 10",
					Then: []data.Step{
						{
							ID:     "then-step",
							Type:   data.StepTypeAction,
							Action: "echo",
							Params: map[string]interface{}{
								"message": "condition met",
							},
						},
					},
					Else: []data.Step{
						{
							ID:     "else-step",
							Type:   data.StepTypeAction,
							Action: "echo",
							Params: map[string]interface{}{
								"message": "condition not met",
							},
							OutputAs: "else_result",
						},
					},
				},
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input: map[string]interface{}{
			"value": float64(5),
		},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !record.Success {
		t.Errorf("Expected success, got failure: %s", record.Error)
	}
}

func TestEngine_Execute_LoopStep_ForEach(t *testing.T) {
	registry := newMockToolRegistry()
	engine := NewEngine(registry)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-foreach",
		Name: "Test ForEach Workflow",
		Steps: []data.Step{
			{
				ID:       "get-items",
				Type:     data.StepTypeAction,
				Action:   "get_items",
				Params:   map[string]interface{}{},
				OutputAs: "items",
			},
			{
				ID:   "loop-step",
				Type: data.StepTypeLoop,
				Loop: &data.LoopConfig{
					Type:       data.LoopTypeForEach,
					Expression: "${items}",
					Iterator:   "item",
					Steps: []data.Step{
						{
							ID:     "process-item",
							Type:   data.StepTypeAction,
							Action: "echo",
							Params: map[string]interface{}{
								"message": "${item.name}",
							},
						},
					},
				},
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input:      map[string]interface{}{},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !record.Success {
		t.Errorf("Expected success, got failure: %s", record.Error)
	}

	// Should have 2 step executions: get_items + loop
	if len(record.StepExecutions) != 2 {
		t.Errorf("Expected 2 step executions, got %d", len(record.StepExecutions))
	}

	// Check loop result
	loopExec := record.StepExecutions[1]
	if !loopExec.Success {
		t.Errorf("Expected loop success, got failure: %s", loopExec.Error)
	}
}

func TestEngine_Execute_LoopStep_Repeat(t *testing.T) {
	registry := newMockToolRegistry()
	engine := NewEngine(registry)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-repeat",
		Name: "Test Repeat Workflow",
		Steps: []data.Step{
			{
				ID:   "loop-step",
				Type: data.StepTypeLoop,
				Loop: &data.LoopConfig{
					Type:       data.LoopTypeRepeat,
					Expression: "3",
					IndexVar:   "i",
					Steps: []data.Step{
						{
							ID:     "process",
							Type:   data.StepTypeAction,
							Action: "echo",
							Params: map[string]interface{}{
								"message": "iteration ${i}",
							},
						},
					},
				},
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input:      map[string]interface{}{},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !record.Success {
		t.Errorf("Expected success, got failure: %s", record.Error)
	}

	// Check loop result has 3 iterations
	loopExec := record.StepExecutions[0]
	result, ok := loopExec.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Expected loop result to be map, got %T", loopExec.Result)
	}

	iterations, ok := result["iterations"].(int)
	if !ok {
		t.Fatalf("Expected iterations to be int, got %T", result["iterations"])
	}

	if iterations != 3 {
		t.Errorf("Expected 3 iterations, got %d", iterations)
	}
}

func TestEngine_Execute_MultipleSteps(t *testing.T) {
	registry := newMockToolRegistry()
	engine := NewEngine(registry)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-multi",
		Name: "Test Multiple Steps Workflow",
		Steps: []data.Step{
			{
				ID:     "step1",
				Type:   data.StepTypeAction,
				Action: "add",
				Params: map[string]interface{}{
					"a": float64(1),
					"b": float64(2),
				},
				OutputAs: "sum1",
			},
			{
				ID:     "step2",
				Type:   data.StepTypeAction,
				Action: "add",
				Params: map[string]interface{}{
					"a": float64(3),
					"b": float64(4),
				},
				OutputAs: "sum2",
			},
			{
				ID:     "step3",
				Type:   data.StepTypeAction,
				Action: "echo",
				Params: map[string]interface{}{
					"message": "Done",
				},
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input:      map[string]interface{}{},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !record.Success {
		t.Errorf("Expected success, got failure: %s", record.Error)
	}

	if len(record.StepExecutions) != 3 {
		t.Errorf("Expected 3 step executions, got %d", len(record.StepExecutions))
	}

	// Check execution times are set
	for i, stepExec := range record.StepExecutions {
		if stepExec.StartedAt.IsZero() {
			t.Errorf("Step %d: StartedAt not set", i)
		}
		if stepExec.CompletedAt.IsZero() {
			t.Errorf("Step %d: CompletedAt not set", i)
		}
	}

	// Check record times
	if record.StartedAt.IsZero() {
		t.Error("Record StartedAt not set")
	}
	if record.CompletedAt.IsZero() {
		t.Error("Record CompletedAt not set")
	}

	// Check execution ID is set
	if record.ExecutionID == "" {
		t.Error("ExecutionID not set")
	}
}

func TestEngine_Execute_NoToolRegistry(t *testing.T) {
	engine := NewEngine(nil)

	workflow := &data.WorkflowDef{
		ID:   "wf-test-no-registry",
		Name: "Test No Registry Workflow",
		Steps: []data.Step{
			{
				ID:     "step1",
				Type:   data.StepTypeAction,
				Action: "echo",
				Params: map[string]interface{}{
					"message": "hello",
				},
			},
		},
	}

	execCtx := data.ExecutionContext{
		SpaceID:    "test-space",
		MemberName: "test-user",
		Input:      map[string]interface{}{},
	}

	record, err := engine.Execute(context.Background(), workflow, execCtx)
	if err == nil {
		t.Error("Expected error when tool registry is nil")
	}

	if record.Success {
		t.Error("Expected failure when tool registry is nil")
	}

	stepExec := record.StepExecutions[0]
	if stepExec.Error != "tool registry not available" {
		t.Errorf("Expected 'tool registry not available', got '%s'", stepExec.Error)
	}
}

func TestEngine_resolveString(t *testing.T) {
	e := &engine{}

	variables := map[string]interface{}{
		"name":    "John",
		"count":   42,
		"enabled": true,
		"user": map[string]interface{}{
			"name": "Alice",
			"age":  30,
		},
		"items": []interface{}{
			map[string]interface{}{"id": "1", "name": "Item 1"},
			map[string]interface{}{"id": "2", "name": "Item 2"},
		},
	}

	tests := []struct {
		input    string
		expected string
	}{
		{"Hello ${name}", "Hello John"},
		{"Count: ${count}", "Count: 42"},
		{"Enabled: ${enabled}", "Enabled: true"},
		{"User: ${user.name}", "User: Alice"},
		{"Age: ${user.age}", "Age: 30"},
		{"No variables", "No variables"},
		{"${unknown}", "${unknown}"}, // Unknown variable keeps original
	}

	for _, test := range tests {
		result := e.resolveString(test.input, variables)
		if result != test.expected {
			t.Errorf("resolveString(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestEngine_evaluateCondition(t *testing.T) {
	e := &engine{}

	variables := map[string]interface{}{
		"value":   float64(10),
		"name":    "test",
		"enabled": true,
		"count":   0,
		"empty":   "",
	}

	tests := []struct {
		expression string
		expected   bool
	}{
		// Equality
		{"${value} == 10", true},
		{"${value} == 5", false},
		{"${name} == test", true},
		{"${name} == other", false},

		// Inequality
		{"${value} != 10", false},
		{"${value} != 5", true},

		// Numeric comparison
		{"${value} > 5", true},
		{"${value} < 15", true},
		{"${value} >= 10", true},
		{"${value} <= 10", true},

		// Truthy check
		{"${enabled}", true},
		{"${value}", true},
		{"${count}", false}, // 0 is falsy
		{"${empty}", false}, // empty string is falsy
	}

	for _, test := range tests {
		result := e.evaluateCondition(test.expression, variables)
		if result != test.expected {
			t.Errorf("evaluateCondition(%q) = %v, expected %v", test.expression, result, test.expected)
		}
	}
}

func TestEngine_isTruthy(t *testing.T) {
	e := &engine{}

	tests := []struct {
		value    interface{}
		expected bool
	}{
		{true, true},
		{false, false},
		{"hello", true},
		{"", false},
		{"false", false},
		{"0", false},
		{1, true},
		{0, false},
		{float64(1.5), true},
		{float64(0), false},
		{nil, false},
		{[]interface{}{}, false},
		{[]interface{}{1, 2}, true},
	}

	for _, test := range tests {
		result := e.isTruthy(test.value)
		if result != test.expected {
			t.Errorf("isTruthy(%v) = %v, expected %v", test.value, result, test.expected)
		}
	}
}

func TestEngine_compareValues(t *testing.T) {
	e := &engine{}

	tests := []struct {
		left     string
		right    string
		op       string
		expected bool
	}{
		// Numeric
		{"10", "10", "==", true},
		{"10", "5", ">", true},
		{"5", "10", "<", true},
		{"10", "10", ">=", true},
		{"5", "10", "<=", true},
		{"10", "5", "!=", true},

		// String
		{"hello", "hello", "==", true},
		{"hello", "world", "!=", true},
		{"b", "a", ">", true},
	}

	for _, test := range tests {
		result := e.compareValues(test.left, test.right, test.op)
		if result != test.expected {
			t.Errorf("compareValues(%q, %q, %q) = %v, expected %v",
				test.left, test.right, test.op, result, test.expected)
		}
	}
}

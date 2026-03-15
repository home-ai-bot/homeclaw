// Package workflow provides workflow engine for HomeClaw.
package workflow

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// Engine is the workflow execution engine
type Engine interface {
	// Execute executes a workflow with given definition and context
	Execute(ctx context.Context, workflowDef *data.WorkflowDef, execCtx data.ExecutionContext) (*data.ExecutionRecord, error)
}

// engine implements the Engine interface
type engine struct {
	toolRegistry *tools.ToolRegistry
}

// NewEngine creates a new workflow engine
func NewEngine(toolRegistry *tools.ToolRegistry) Engine {
	return &engine{
		toolRegistry: toolRegistry,
	}
}

// Execute executes a workflow
func (e *engine) Execute(ctx context.Context, workflowDef *data.WorkflowDef, execCtx data.ExecutionContext) (*data.ExecutionRecord, error) {
	executionID := uuid.New().String()
	execCtx.ExecutionID = executionID
	execCtx.WorkflowID = workflowDef.ID

	record := &data.ExecutionRecord{
		WorkflowID:     workflowDef.ID,
		ExecutionID:    executionID,
		Context:        execCtx,
		StartedAt:      time.Now(),
		StepExecutions: make([]data.StepExecution, 0),
	}

	logger.InfoCF("workflow", "Starting workflow execution",
		map[string]interface{}{
			"execution_id":  executionID,
			"workflow_id":   workflowDef.ID,
			"workflow_name": workflowDef.Name,
			"step_count":    len(workflowDef.Steps),
		})

	// Initialize variables storage
	variables := map[string]interface{}{
		"input": execCtx.Input,
		"context": map[string]interface{}{
			"space_id":    execCtx.SpaceID,
			"member_name": execCtx.MemberName,
			"trigger_by":  execCtx.TriggerBy,
		},
	}

	// Execute steps
	for i, step := range workflowDef.Steps {
		stepExec := e.executeStep(ctx, step, variables, i+1, len(workflowDef.Steps))
		record.StepExecutions = append(record.StepExecutions, stepExec)

		if !stepExec.Success {
			record.Success = false
			record.Error = stepExec.Error
			record.CompletedAt = time.Now()
			logger.ErrorCF("workflow", "Workflow execution failed",
				map[string]interface{}{
					"execution_id": executionID,
					"workflow_id":  workflowDef.ID,
					"failed_step":  step.ID,
					"error":        stepExec.Error,
				})
			return record, fmt.Errorf("step %s failed: %s", step.ID, stepExec.Error)
		}

		// Store output in variables if specified
		if step.OutputAs != "" && stepExec.Result != nil {
			variables[step.OutputAs] = stepExec.Result
		}
	}

	record.Success = true
	record.CompletedAt = time.Now()

	logger.InfoCF("workflow", "Workflow execution completed",
		map[string]interface{}{
			"execution_id":   executionID,
			"workflow_id":    workflowDef.ID,
			"execution_time": record.CompletedAt.Sub(record.StartedAt).Milliseconds(),
		})

	// Log execution record as JSON
	recordJSON, _ := json.Marshal(record)
	logger.InfoCF("workflow", "Execution record", map[string]interface{}{
		"record": string(recordJSON),
	})

	return record, nil
}

// executeStep executes a single step
func (e *engine) executeStep(ctx context.Context, step data.Step, variables map[string]interface{}, stepNum, totalSteps int) data.StepExecution {
	now := time.Now()
	stepExec := data.StepExecution{
		StepID:    step.ID,
		StartedAt: now,
	}

	logger.InfoCF("workflow", fmt.Sprintf("Executing step %d/%d: %s", stepNum, totalSteps, step.Type),
		map[string]interface{}{
			"step_id":     step.ID,
			"step_type":   step.Type,
			"step_name":   step.Name,
			"step_num":    stepNum,
			"total_steps": totalSteps,
		})

	switch step.Type {
	case data.StepTypeAction:
		return e.executeActionStep(ctx, step, variables, stepExec)
	case data.StepTypeCondition:
		return e.executeConditionStep(ctx, step, variables, stepExec)
	case data.StepTypeLoop:
		return e.executeLoopStep(ctx, step, variables, stepExec)
	default:
		stepExec.Success = false
		stepExec.Error = fmt.Sprintf("unknown step type: %s", step.Type)
		stepExec.CompletedAt = time.Now()
		return stepExec
	}
}

// executeActionStep executes an action step (tool/skill call)
func (e *engine) executeActionStep(ctx context.Context, step data.Step, variables map[string]interface{}, stepExec data.StepExecution) data.StepExecution {
	stepExec.Action = step.Action

	// Resolve variables in parameters
	resolvedParams := e.resolveParams(step.Params, variables)
	stepExec.Params = resolvedParams

	// Log action execution
	paramsJSON, _ := json.Marshal(resolvedParams)
	logger.InfoCF("workflow", fmt.Sprintf("Action: %s", step.Action),
		map[string]interface{}{
			"step_id": step.ID,
			"action":  step.Action,
			"params":  string(paramsJSON),
		})

	// Execute tool/skill
	if e.toolRegistry == nil {
		stepExec.Success = false
		stepExec.Error = "tool registry not available"
		stepExec.CompletedAt = time.Now()
		return stepExec
	}

	result := e.toolRegistry.Execute(ctx, step.Action, resolvedParams)
	stepExec.CompletedAt = time.Now()

	if result.Err != nil || result.IsError {
		stepExec.Success = false
		if result.Err != nil {
			stepExec.Error = result.Err.Error()
		} else {
			stepExec.Error = result.ForLLM
		}
		logger.ErrorCF("workflow", "Action failed",
			map[string]interface{}{
				"step_id": step.ID,
				"action":  step.Action,
				"error":   stepExec.Error,
			})
	} else {
		stepExec.Success = true
		// Parse ForLLM as JSON if possible, otherwise use as string
		var resultData interface{}
		if err := json.Unmarshal([]byte(result.ForLLM), &resultData); err != nil {
			resultData = result.ForLLM
		}
		stepExec.Result = resultData
		logger.InfoCF("workflow", "Action completed",
			map[string]interface{}{
				"step_id": step.ID,
				"action":  step.Action,
			})
	}

	return stepExec
}

// executeConditionStep executes a condition step
func (e *engine) executeConditionStep(ctx context.Context, step data.Step, variables map[string]interface{}, stepExec data.StepExecution) data.StepExecution {
	stepExec.Action = "condition"

	if step.Condition == nil {
		stepExec.Success = false
		stepExec.Error = "condition is nil"
		stepExec.CompletedAt = time.Now()
		return stepExec
	}

	// Evaluate condition
	conditionMet := e.evaluateCondition(step.Condition.If, variables)
	stepExec.Params = map[string]interface{}{
		"condition": step.Condition.If,
		"result":    conditionMet,
	}

	logger.InfoCF("workflow", fmt.Sprintf("Condition: %s = %v", step.Condition.If, conditionMet),
		map[string]interface{}{
			"step_id":   step.ID,
			"condition": step.Condition.If,
			"result":    conditionMet,
		})

	// Execute branch
	var branchSteps []data.Step
	if conditionMet {
		branchSteps = step.Condition.Then
	} else {
		branchSteps = step.Condition.Else
	}

	for _, branchStep := range branchSteps {
		branchExec := e.executeStep(ctx, branchStep, variables, 1, len(branchSteps))
		if !branchExec.Success {
			stepExec.Success = false
			stepExec.Error = branchExec.Error
			stepExec.CompletedAt = time.Now()
			return stepExec
		}
		// Store output if specified
		if branchStep.OutputAs != "" && branchExec.Result != nil {
			variables[branchStep.OutputAs] = branchExec.Result
		}
	}

	stepExec.Success = true
	stepExec.CompletedAt = time.Now()
	return stepExec
}

// executeLoopStep executes a loop step
func (e *engine) executeLoopStep(ctx context.Context, step data.Step, variables map[string]interface{}, stepExec data.StepExecution) data.StepExecution {
	stepExec.Action = "loop"

	if step.Loop == nil {
		stepExec.Success = false
		stepExec.Error = "loop config is nil"
		stepExec.CompletedAt = time.Now()
		return stepExec
	}

	loop := step.Loop
	maxIterations := loop.MaxIterations
	if maxIterations <= 0 {
		maxIterations = 100 // Default max iterations
	}

	stepExec.Params = map[string]interface{}{
		"type":       loop.Type,
		"expression": loop.Expression,
	}

	logger.InfoCF("workflow", fmt.Sprintf("Loop: %s", loop.Type),
		map[string]interface{}{
			"step_id":    step.ID,
			"loop_type":  loop.Type,
			"expression": loop.Expression,
		})

	iterationCount := 0

	switch loop.Type {
	case data.LoopTypeForEach:
		// Resolve collection expression
		collection := e.resolveValue(loop.Expression, variables)

		// Handle case where collection is a JSON string that needs parsing
		if str, ok := collection.(string); ok {
			var parsed interface{}
			if err := json.Unmarshal([]byte(str), &parsed); err == nil {
				collection = parsed
			}
		}

		collectionValue := reflect.ValueOf(collection)

		if collectionValue.Kind() != reflect.Slice && collectionValue.Kind() != reflect.Array {
			stepExec.Success = false
			stepExec.Error = fmt.Sprintf("foreach requires a collection, got %T", collection)
			stepExec.CompletedAt = time.Now()
			return stepExec
		}

		for i := 0; i < collectionValue.Len() && iterationCount < maxIterations; i++ {
			iterationCount++
			item := collectionValue.Index(i).Interface()

			// Set iterator variable
			if loop.Iterator != "" {
				variables[loop.Iterator] = item
			}
			if loop.IndexVar != "" {
				variables[loop.IndexVar] = i
			}

			// Execute loop body
			for _, loopStep := range loop.Steps {
				loopStepExec := e.executeStep(ctx, loopStep, variables, iterationCount, maxIterations)
				if !loopStepExec.Success {
					stepExec.Success = false
					stepExec.Error = loopStepExec.Error
					stepExec.CompletedAt = time.Now()
					return stepExec
				}
				if loopStep.OutputAs != "" && loopStepExec.Result != nil {
					variables[loopStep.OutputAs] = loopStepExec.Result
				}
			}
		}

	case data.LoopTypeWhile:
		for iterationCount < maxIterations {
			conditionMet := e.evaluateCondition(loop.Expression, variables)
			if !conditionMet {
				break
			}
			iterationCount++

			// Execute loop body
			for _, loopStep := range loop.Steps {
				loopStepExec := e.executeStep(ctx, loopStep, variables, iterationCount, maxIterations)
				if !loopStepExec.Success {
					stepExec.Success = false
					stepExec.Error = loopStepExec.Error
					stepExec.CompletedAt = time.Now()
					return stepExec
				}
				if loopStep.OutputAs != "" && loopStepExec.Result != nil {
					variables[loopStep.OutputAs] = loopStepExec.Result
				}
			}
		}

	case data.LoopTypeRepeat:
		// Parse repeat count
		countStr := e.resolveString(loop.Expression, variables)
		count, err := strconv.Atoi(countStr)
		if err != nil {
			stepExec.Success = false
			stepExec.Error = fmt.Sprintf("invalid repeat count: %s", loop.Expression)
			stepExec.CompletedAt = time.Now()
			return stepExec
		}

		for i := 0; i < count && iterationCount < maxIterations; i++ {
			iterationCount++

			if loop.IndexVar != "" {
				variables[loop.IndexVar] = i
			}

			// Execute loop body
			for _, loopStep := range loop.Steps {
				loopStepExec := e.executeStep(ctx, loopStep, variables, iterationCount, maxIterations)
				if !loopStepExec.Success {
					stepExec.Success = false
					stepExec.Error = loopStepExec.Error
					stepExec.CompletedAt = time.Now()
					return stepExec
				}
				if loopStep.OutputAs != "" && loopStepExec.Result != nil {
					variables[loopStep.OutputAs] = loopStepExec.Result
				}
			}
		}

	default:
		stepExec.Success = false
		stepExec.Error = fmt.Sprintf("unknown loop type: %s", loop.Type)
		stepExec.CompletedAt = time.Now()
		return stepExec
	}

	stepExec.Success = true
	stepExec.Result = map[string]interface{}{
		"iterations": iterationCount,
	}
	stepExec.CompletedAt = time.Now()

	logger.InfoCF("workflow", fmt.Sprintf("Loop completed: %d iterations", iterationCount),
		map[string]interface{}{
			"step_id":    step.ID,
			"iterations": iterationCount,
		})

	return stepExec
}

// resolveParams resolves variables in parameter map
func (e *engine) resolveParams(params map[string]interface{}, variables map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}

	result := make(map[string]interface{})
	for key, value := range params {
		result[key] = e.resolveValue(value, variables)
	}
	return result
}

// resolveValue resolves variables in a value
func (e *engine) resolveValue(value interface{}, variables map[string]interface{}) interface{} {
	switch v := value.(type) {
	case string:
		return e.resolveString(v, variables)
	case map[string]interface{}:
		return e.resolveParams(v, variables)
	case []interface{}:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = e.resolveValue(item, variables)
		}
		return result
	default:
		return value
	}
}

// resolveString resolves ${var} syntax in a string
func (e *engine) resolveString(template string, variables map[string]interface{}) string {
	// Match ${var} or ${var.property} or ${var[index]}
	re := regexp.MustCompile(`\$\{([^}]+)\}`)

	return re.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable path (remove ${ and })
		path := match[2 : len(match)-1]
		value := e.getVariableValue(path, variables)
		if value == nil {
			return match // Keep original if not found
		}

		// Convert to string
		switch v := value.(type) {
		case string:
			return v
		case int:
			return strconv.Itoa(v)
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			return strconv.FormatBool(v)
		default:
			// Try JSON marshaling for complex types
			if jsonBytes, err := json.Marshal(v); err == nil {
				return string(jsonBytes)
			}
			return fmt.Sprintf("%v", v)
		}
	})
}

// getVariableValue gets a variable value by path (e.g., "input.room" or "context.space_id")
func (e *engine) getVariableValue(path string, variables map[string]interface{}) interface{} {
	parts := strings.Split(path, ".")
	if len(parts) == 0 {
		return nil
	}

	// Get root variable
	value, ok := variables[parts[0]]
	if !ok {
		return nil
	}

	// Navigate path
	for _, part := range parts[1:] {
		if value == nil {
			return nil
		}

		// Handle array index
		if idx := strings.Index(part, "["); idx >= 0 {
			arrayName := part[:idx]
			indexStr := part[idx+1 : len(part)-1] // Remove [ and ]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil
			}

			// Get array
			val := reflect.ValueOf(value)
			if arrayName != "" {
				// Access field first
				val = reflect.Indirect(val).FieldByName(arrayName)
			}
			if val.Kind() != reflect.Slice && val.Kind() != reflect.Array {
				return nil
			}
			if index < 0 || index >= val.Len() {
				return nil
			}
			value = val.Index(index).Interface()
			continue
		}

		// Handle map access
		if m, ok := value.(map[string]interface{}); ok {
			value = m[part]
			continue
		}

		// Handle struct field access via reflection
		val := reflect.ValueOf(value)
		if val.Kind() == reflect.Ptr {
			val = val.Elem()
		}
		if val.Kind() != reflect.Struct {
			return nil
		}

		field := val.FieldByName(part)
		if !field.IsValid() {
			// Try lowercase field name
			field = val.FieldByName(strings.Title(part))
		}
		if !field.IsValid() {
			return nil
		}
		value = field.Interface()
	}

	return value
}

// evaluateCondition evaluates a condition expression
func (e *engine) evaluateCondition(expression string, variables map[string]interface{}) bool {
	// Handle simple truthy check (no operators)
	if !strings.Contains(expression, "==") && !strings.Contains(expression, "!=") &&
		!strings.Contains(expression, ">=") && !strings.Contains(expression, "<=") &&
		!strings.Contains(expression, ">") && !strings.Contains(expression, "<") {
		// Just check if value is truthy
		// First resolve any ${var} references
		resolvedExpr := e.resolveString(expression, variables)
		// If it's a simple variable reference that resolved to a value, use that value
		if resolvedExpr != expression {
			// It was a variable reference, check if the resolved value is truthy
			// But we need to get the actual value, not the string representation
			value := e.getVariableValue(expression[2:len(expression)-1], variables) // Remove ${ and }
			return e.isTruthy(value)
		}
		// Otherwise check if the string itself is truthy
		return e.isTruthy(resolvedExpr)
	}

	// Resolve variables in expression
	resolved := e.resolveString(expression, variables)

	// Parse comparison operators
	// Support: ==, !=, >, <, >=, <=
	operators := []string{"==", "!=", ">=", "<=", ">", "<"}
	for _, op := range operators {
		if idx := strings.Index(resolved, op); idx >= 0 {
			left := strings.TrimSpace(resolved[:idx])
			right := strings.TrimSpace(resolved[idx+len(op):])

			return e.compareValues(left, right, op)
		}
	}

	// Default: check if resolved string is truthy
	return e.isTruthy(resolved)
}

// isTruthy checks if a value is truthy
func (e *engine) isTruthy(value interface{}) bool {
	if value == nil {
		return false
	}

	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v != "" && v != "false" && v != "0"
	case int:
		return v != 0
	case float64:
		return v != 0
	case []interface{}:
		return len(v) > 0
	default:
		val := reflect.ValueOf(value)
		if val.Kind() == reflect.Slice || val.Kind() == reflect.Array {
			return val.Len() > 0
		}
		return true
	}
}

// compareValues compares two values with an operator
func (e *engine) compareValues(left, right, op string) bool {
	// Try numeric comparison
	leftFloat, leftErr := strconv.ParseFloat(left, 64)
	rightFloat, rightErr := strconv.ParseFloat(right, 64)

	if leftErr == nil && rightErr == nil {
		switch op {
		case "==":
			return leftFloat == rightFloat
		case "!=":
			return leftFloat != rightFloat
		case ">":
			return leftFloat > rightFloat
		case "<":
			return leftFloat < rightFloat
		case ">=":
			return leftFloat >= rightFloat
		case "<=":
			return leftFloat <= rightFloat
		}
	}

	// String comparison
	switch op {
	case "==":
		return left == right
	case "!=":
		return left != right
	case ">":
		return left > right
	case "<":
		return left < right
	case ">=":
		return left >= right
	case "<=":
		return left <= right
	}

	return false
}

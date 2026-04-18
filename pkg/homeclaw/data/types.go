// Package data provides data access layer for HomeClaw.
package data

import "time"

// Space represents a physical space in the home (floor, room, area, etc.)
type Space struct {
	Name string            `json:"name"`           // Primary key, unique space name
	From map[string]string `json:"from,omitempty"` // Source info, e.g. {"xiaomi": "123456"}
}

// Device represents a smart device in the home.
type Device struct {
	FromID    string   `json:"from_id"`
	From      string   `json:"from"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Token     string   `json:"token"`
	IP        string   `json:"ip"`
	URN       string   `json:"urn"`
	SpaceName string   `json:"space_name,omitempty"`
	Ops       []string `json:"ops,omitempty"`
}

// DeviceOp represents an operation that a device can perform
type DeviceOp struct {
	FromID string `json:"from_id"`
	From   string `json:"from"`
	Ops    string `json:"ops"`    // Operation name, e.g. "turn_on,turn_off"
	Method string `json:"method"` // Method name, e.g. "getProps,setProps,execute"
	Param  string `json:"param"`  // CLI tool command parameters
}

// Home represents a home information
type Home struct {
	FromID  string `json:"from_id"`
	From    string `json:"from"`
	Name    string `json:"name"`
	Current bool   `json:"current"`
}

// SpacesData is the root structure for spaces.json
type SpacesData struct {
	Version string  `json:"version"`
	Spaces  []Space `json:"spaces"`
}

// DevicesData is the root structure for devices.json
type DevicesData struct {
	Version string   `json:"version"`
	Devices []Device `json:"devices"`
}

// DeviceOpsData is the root structure for device_ops.json
type DeviceOpsData struct {
	Version   string     `json:"version"`
	DeviceOps []DeviceOp `json:"device_ops"`
}

// HomesData is the root structure for homes.json
type HomesData struct {
	Version string `json:"version"`
	Homes   []Home `json:"homes"`
}

// ==================== Workflow Types ====================

// WorkflowMeta represents workflow metadata in the index
type WorkflowMeta struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	FileName    string    `json:"file_name"`
	CreatedBy   string    `json:"created_by"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	Enabled     bool      `json:"enabled"`
}

// WorkflowsData is the root structure for workflow-index.json
type WorkflowsData struct {
	Version   string         `json:"version"`
	Workflows []WorkflowMeta `json:"workflows"`
}

// WorkflowDef represents a workflow definition
type WorkflowDef struct {
	ID          string             `json:"id"`
	Name        string             `json:"name"`
	Description string             `json:"description"`
	Version     string             `json:"version"`
	Triggers    []Trigger          `json:"triggers"`
	Context     WorkflowContext    `json:"context"`
	Steps       []Step             `json:"steps"`
	Variants    map[string]Variant `json:"variants"`
	CreatedBy   string             `json:"created_by"`
	CreatedAt   time.Time          `json:"created_at"`
	UpdatedAt   time.Time          `json:"updated_at"`
}

// Trigger defines when a workflow should be executed
type Trigger struct {
	Type     string   `json:"type"`     // "intent", "event", "cron"
	Patterns []string `json:"patterns"` // for intent triggers
}

// WorkflowContext defines the context requirements for workflow execution
type WorkflowContext struct {
	Space  string `json:"space"`  // "current" or specific space ID
	Member string `json:"member"` // "current" or specific member name
}

// StepType defines the type of a workflow step
type StepType string

const (
	StepTypeAction    StepType = "action"    // Tool/skill execution
	StepTypeCondition StepType = "condition" // Conditional branching
	StepTypeLoop      StepType = "loop"      // Loop control
)

// Step represents a single step in a workflow
type Step struct {
	ID   string   `json:"id"`
	Type StepType `json:"type"`
	Name string   `json:"name,omitempty"` // Step name for debugging

	// Action type fields
	Action   string                 `json:"action,omitempty"`    // Tool/skill name
	Params   map[string]interface{} `json:"params,omitempty"`    // Parameters with variable support
	OutputAs string                 `json:"output_as,omitempty"` // Variable name to store result

	// Condition type fields
	Condition *Condition `json:"condition,omitempty"`

	// Loop type fields
	Loop *LoopConfig `json:"loop,omitempty"`
}

// Condition defines conditional branching
type Condition struct {
	// Expression supports:
	// - "${varName}" - truthy check
	// - "${varName} == value" - equality
	// - "${varName} != value" - inequality
	// - "${varName} > 10" - numeric comparison
	If   string `json:"if"`
	Then []Step `json:"then"` // Executed if condition is true
	Else []Step `json:"else"` // Executed if condition is false
}

// LoopType defines the type of loop
type LoopType string

const (
	LoopTypeForEach LoopType = "foreach" // Iterate over collection
	LoopTypeWhile   LoopType = "while"   // Condition-based loop
	LoopTypeRepeat  LoopType = "repeat"  // Fixed count loop
)

// LoopConfig defines loop configuration
type LoopConfig struct {
	Type          LoopType `json:"type"`
	Expression    string   `json:"expression"`               // Collection, condition, or count
	Iterator      string   `json:"iterator,omitempty"`       // Loop variable name
	IndexVar      string   `json:"index_var,omitempty"`      // Index variable name (optional)
	Steps         []Step   `json:"steps"`                    // Loop body
	MaxIterations int      `json:"max_iterations,omitempty"` // Default: 100
}

// Variant represents a personalized variant of a workflow
type Variant struct {
	Description string `json:"description"`
	Steps       []Step `json:"steps"`
}

// ExecutionContext provides context for workflow execution
type ExecutionContext struct {
	WorkflowID  string                 `json:"workflow_id"`
	ExecutionID string                 `json:"execution_id"`
	MemberName  string                 `json:"member_name"`
	SpaceID     string                 `json:"space_id"`
	TriggerBy   string                 `json:"trigger_by"` // "intent" | "event" | "cron"
	Input       map[string]interface{} `json:"input"`
}

// StepExecution represents the execution record of a single step
type StepExecution struct {
	StepID      string                 `json:"step_id"`
	Action      string                 `json:"action"`
	Params      map[string]interface{} `json:"params"`
	StartedAt   time.Time              `json:"started_at"`
	CompletedAt time.Time              `json:"completed_at,omitempty"`
	Success     bool                   `json:"success"`
	Result      interface{}            `json:"result,omitempty"`
	Error       string                 `json:"error,omitempty"`
}

// ExecutionRecord represents a complete workflow execution record
type ExecutionRecord struct {
	WorkflowID     string           `json:"workflow_id"`
	ExecutionID    string           `json:"execution_id"`
	Context        ExecutionContext `json:"context"`
	StartedAt      time.Time        `json:"started_at"`
	CompletedAt    time.Time        `json:"completed_at,omitempty"`
	Success        bool             `json:"success"`
	StepExecutions []StepExecution  `json:"step_executions"`
	Error          string           `json:"error,omitempty"`
}

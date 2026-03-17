// Package data provides data access layer for HomeClaw.
package data

import "time"

// Space represents a physical space in the home (floor, room, etc.)
type Space struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	Type     string  `json:"type"`             // "floor", "room", "area"
	Source   string  `json:"source,omitempty"` // 来源: "xiaomi", "manual" 等
	Children []Space `json:"children"`
}

// Device represents a smart device in the home
type Device struct {
	ID       string            `json:"id"`
	Name     string            `json:"name"`
	Brand    string            `json:"brand"`    // "mijia", "tuya", "homekit", "matter"
	Protocol string            `json:"protocol"` // "miio", "local", "hap", "matter"
	Model    string            `json:"model"`
	SpaceID  string            `json:"space_id"`
	IP       string            `json:"ip"`
	Token    string            `json:"token"`
	Props    map[string]string `json:"props"`
	LastSeen time.Time         `json:"last_seen"`
	AddedAt  time.Time         `json:"added_at"`

	// Xiaomi device fields (synced from Mi Home)
	DID          string             `json:"did,omitempty"`
	UID          string             `json:"uid,omitempty"`
	URN          string             `json:"urn,omitempty"`
	ConnectType  int                `json:"connect_type,omitempty"`
	Online       bool               `json:"online,omitempty"`
	Icon         string             `json:"icon,omitempty"`
	ParentID     string             `json:"parent_id,omitempty"`
	Manufacturer string             `json:"manufacturer,omitempty"`
	VoiceCtrl    int                `json:"voice_ctrl,omitempty"`
	SSID         string             `json:"ssid,omitempty"`
	BSSID        string             `json:"bssid,omitempty"`
	OrderTime    int64              `json:"order_time,omitempty"`
	FWVersion    string             `json:"fw_version,omitempty"`
	SubDevices   map[string]*Device `json:"sub_devices,omitempty"`
	RoomID       string             `json:"room_id,omitempty"`
	RoomName     string             `json:"room_name,omitempty"`
	GroupID      string             `json:"group_id,omitempty"`
}

// Member represents a family member
type Member struct {
	Name             string                 `json:"name"`
	Role             string                 `json:"role"`              // "admin", "member"
	SpacePermissions []string               `json:"space_permissions"` // ["*"] for all
	Channels         map[string]ChannelInfo `json:"channels"`
	DefaultSpaceID   string                 `json:"default_space_id"`
	CreatedAt        time.Time              `json:"created_at"`
}

// ChannelInfo represents a member's binding to a communication channel
type ChannelInfo struct {
	UserID  string    `json:"user_id"`
	BoundAt time.Time `json:"bound_at"`
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

// MembersData is the root structure for members.json
type MembersData struct {
	Version string   `json:"version"`
	Members []Member `json:"members"`
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

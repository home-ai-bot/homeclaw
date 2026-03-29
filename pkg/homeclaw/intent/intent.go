// Package intent defines the interfaces and core types for HomeClaw intent
// recognition and dispatching.
//
// Architecture:
//
//	processMessage()
//	  ├── handleCommand()    ← existing PicoClaw command handling
//	  ├── handleIntent()     ← HomeClaw intent recognition & dispatch
//	  │       ↓
//	  │   IntentClassifier (small model)
//	  │       ↓ IntentResult{Type, Confidence, Entities}
//	  │       ↓
//	  │   Router → selects Intent implementation by type
//	  │       ↓
//	  │   Intent.Run(ctx, IntentContext) → IntentResponse
//	  │       ├── Handled=true  → return response directly
//	  │       └── Handled=false → fall through to runAgentLoop (large model)
//	  └── runAgentLoop()    ← existing large-model processing
package intent

import "context"

// ─────────────────────────────────────────────
// Intent type constants
// ─────────────────────────────────────────────

// IntentType identifies a specific user intent category.
// Values correspond 1-to-1 with the intent taxonomy defined in 意图识别设计.md.
type IntentType string

const (
	// ── Device control ──────────────────────────────────────────────────────
	// IntentDeviceControlSingle controls a single device, e.g. "turn on the light".
	IntentDeviceControlSingle IntentType = "device.control.single"
	// IntentDeviceControlScene triggers a scene, e.g. "I'm going to sleep".
	IntentDeviceControlScene IntentType = "device.control.scene"
	// IntentDeviceControlGlobal performs a global action, e.g. "turn off all lights".
	IntentDeviceControlGlobal IntentType = "device.control.global"
	// IntentDeviceControlCorrect amends the last action, e.g. "keep the desk lamp on".
	IntentDeviceControlCorrect IntentType = "device.control.correct"

	// ── Device management ───────────────────────────────────────────────────
	// IntentDeviceAdd adds a new device.
	IntentDeviceAdd IntentType = "device.add"
	// IntentDeviceScan scans the local network for devices.
	IntentDeviceScan IntentType = "device.scan"
	// IntentDeviceRemove removes a device.
	IntentDeviceRemove IntentType = "device.remove"
	// IntentDeviceRename renames a device.
	IntentDeviceRename IntentType = "device.rename"
	// IntentDeviceMove moves a device to another room.
	IntentDeviceMove IntentType = "device.move"
	// IntentDeviceQueryStatus queries the current state of a device.
	IntentDeviceQueryStatus IntentType = "device.query.status"

	// ── Space management ────────────────────────────────────────────────────
	// IntentSpaceDefine defines the home space structure.
	IntentSpaceDefine IntentType = "space.define"
	// IntentSpaceRename renames a space.
	IntentSpaceRename IntentType = "space.rename"
	// IntentSpaceQuery queries the space structure.
	IntentSpaceQuery IntentType = "space.query"

	// ── System configuration ─────────────────────────────────────────────────
	// IntentConfigSkillEnable enables a skill plugin.
	IntentConfigSkillEnable IntentType = "config.skill.enable"
	// IntentConfigSkillDisable disables a skill plugin.
	IntentConfigSkillDisable IntentType = "config.skill.disable"

	// ── Conversational ───────────────────────────────────────────────────────
	// IntentChatGreeting handles greetings.
	IntentChatGreeting IntentType = "chat.greeting"
	// IntentChatHelp handles help requests.
	IntentChatHelp IntentType = "chat.help"
	// IntentChatConfirm handles user confirmation in a multi-turn flow.
	IntentChatConfirm IntentType = "chat.confirm"
	// IntentChatCancel handles cancellation in a multi-turn flow.
	IntentChatCancel IntentType = "chat.cancel"

	// IntentUnknown is returned when the classifier cannot determine the intent
	// or when confidence is below the configured threshold.
	// The agent loop will fall through to the large-model handler.
	IntentUnknown IntentType = "unknown"
)

// ─────────────────────────────────────────────
// Data structures
// ─────────────────────────────────────────────

// IntentResult is the output of the intent classifier.
type IntentResult struct {
	// Type is the recognised intent.
	Type IntentType `json:"intent"`
	// Confidence is the classifier's confidence score in [0, 1].
	Confidence float64 `json:"confidence"`
	// Entities contains extracted named entities relevant to the intent,
	// e.g. {"device_name": "台灯", "action": "on"}.
	Entities map[string]interface{} `json:"entities"`
}

// IntentContext carries all information an Intent handler needs to process
// a user request.
type IntentContext struct {
	// UserInput is the raw user message content.
	UserInput string
	// Channel is the communication channel (e.g. "telegram", "wechat").
	Channel string
	// ChatID is the target chat identifier.
	ChatID string
	// SenderID identifies the message sender.
	SenderID string
	// SessionKey identifies the conversation session.
	SessionKey string
	// Result is the classifier output that triggered this handler.
	Result IntentResult
	// DataDir is the path to the HomeClaw data directory.
	// Intent handlers that need to read/write data use this path.
	Workspace string
}

// IntentResponse is the result returned by an Intent handler.
//
// Decision matrix for the agent loop:
//
//	Handled=false, ForwardToLLM=false → intent not processed, fall through to large model with original user input
//	Handled=false, ForwardToLLM=true  → (invalid combination, treated same as Handled=false)
//	Handled=true,  ForwardToLLM=false → fully handled by small model, return Response directly to user
//	Handled=true,  ForwardToLLM=true  → small model processed the intent and produced context in Response;
//	                                     forward Response as additional context to the large model for
//	                                     further reasoning / presentation
type IntentResponse struct {
	// Handled indicates whether the small-model handler has processed the intent.
	// When false, the agent loop falls through to the large-model handler with
	// the original user input.
	Handled bool
	// ForwardToLLM instructs the agent loop to continue to the large-model handler
	// even when Handled is true.  The Response string (if non-empty) is injected
	// as additional context so the large model can reason over the result.
	// Typical use: device control executed successfully but a natural-language
	// summary or follow-up reasoning is still desired from the large model.
	ForwardToLLM bool
	// Response is the text produced by the handler.
	// • When Handled=true and ForwardToLLM=false: sent directly to the user.
	// • When Handled=true and ForwardToLLM=true:  injected as context for the
	//   large model (not sent directly to the user).
	// • When Handled=false: ignored.
	Response string
	// Error contains any error encountered during handling.
	// When non-nil the caller should log the error; Handled may still be true
	// if a graceful error message was placed in Response.
	Error error
}

// ─────────────────────────────────────────────
// Interfaces
// ─────────────────────────────────────────────

// Intent is the core interface that every intent handler must implement.
// Each concrete implementation handles one or more related IntentTypes.
type Intent interface {
	// Types returns all IntentTypes this handler is responsible for.
	// The Router registers the handler under every type in this slice.
	// At least one type must be returned.
	Types() []IntentType
	// Run executes the intent handling logic and returns a response.
	Run(ctx context.Context, ictx IntentContext) IntentResponse
}

// IntentClassifier analyses user input and returns the most likely intent.
// Implementations are expected to call a small language model.
type IntentClassifier interface {
	// Classify performs intent recognition on userInput.
	// On failure or low confidence, implementations MUST return IntentUnknown
	// rather than propagating errors, so the agent loop degrades gracefully.
	Classify(ctx context.Context, userInput string) (IntentResult, error)
}

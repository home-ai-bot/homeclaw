// Package homeclaw provides the HomeClaw subsystem for intent recognition
// and workflow dispatching.  The HomeClaw type is the single entry point
// consumed by the agent loop.
package homeclaw

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/intent"
	"github.com/sipeed/picoclaw/pkg/homeclaw/ioc"
	third "github.com/sipeed/picoclaw/pkg/homeclaw/third/ioc"
	"github.com/sipeed/picoclaw/pkg/media"

	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ErrDisabled is returned by New when HomeClaw is explicitly disabled or
// homeclaw.json is absent. Callers can use errors.Is(err, ErrDisabled) to
// distinguish a deliberate no-op from a real initialisation failure.
var ErrDisabled = ioc.ErrDisabled

// HomeClaw holds all HomeClaw subsystem objects and exposes a single
// RunIntent method that the agent loop calls from processMessage.
type HomeClaw struct {
	f      *ioc.Factory
	thirdf *third.ThirdFactory
}

// NewHomeClaw creates a HomeClaw instance from the given workspace directory,
// PicoClaw config, and message bus.
// workspace is the data root used for all HomeClaw data files (users, devices, workflows …).
// Returns nil (no error) when HomeClaw is disabled or homeclaw.json is absent –
// the caller should treat nil as "not configured".
func NewHomeClaw(workspace string, picolawerCfg *config.Config, msgBus *bus.MessageBus) (*HomeClaw, error) {
	// Create factory which handles all singleton object creation
	factory, err := ioc.NewFactory(workspace, picolawerCfg, msgBus)
	if err != nil {
		if errors.Is(err, ErrDisabled) {
			return nil, ErrDisabled
		}
		return nil, fmt.Errorf("HomeClaw factory creation failed: %w", err)
	}
	thirdf := third.NewThirdFactory(factory)
	return &HomeClaw{
		f:      factory,
		thirdf: thirdf,
	}, nil
}

// RunIntentInput contains all inputs needed for intent recognition.
type RunIntentInput struct {
	UserInput  string
	Channel    string
	ChatID     string
	SenderID   string
	SessionKey string
}

// RunIntent performs intent classification and dispatching.
//
// Return semantics:
//   - (response, true,  false, nil) – fully handled by small model; send response to user.
//   - (context,  true,  true,  nil) – small model handled and produced context; forward
//     context to large model for further reasoning.
//   - ("",       false, false, nil) – not handled; fall through to large model with original input.
//
// A non-nil error is always accompanied by handled=false so the caller can fall
// through safely.
func (hc *HomeClaw) RunIntent(ctx context.Context, in RunIntentInput) (response string, handled bool, forwardToLLM bool, err error) {
	if hc == nil {
		return "", false, false, nil
	}

	// Skip intent processing if IntentEnabled is false
	hcfg := hc.f.GetHomeclawConfig()
	if hcfg != nil && !hcfg.IntentEnabled {
		return "", false, false, nil
	}

	classifier, classifierErr := hc.f.GetIntentClassifier()
	if classifierErr != nil {
		return "", false, false, fmt.Errorf("intent classifier unavailable: %w", classifierErr)
	}

	result, classErr := classifier.Classify(ctx, in.UserInput)
	if classErr != nil {
		logger.WarnCF("homeclaw", "intent classification error, falling through",
			map[string]any{"error": classErr.Error()})
	}
	if result.Type == intent.IntentUnknown {
		return "", false, false, nil
	}

	router, routerErr := hc.f.GetIntentRouter()
	if routerErr != nil {
		return "", false, false, fmt.Errorf("intent router unavailable: %w", routerErr)
	}

	handler, ok := router.Route(result)
	if !ok {
		return "", false, false, nil
	}

	ictx := intent.IntentContext{
		UserInput:  in.UserInput,
		Channel:    in.Channel,
		ChatID:     in.ChatID,
		SenderID:   in.SenderID,
		SessionKey: in.SessionKey,
		Result:     result,
		Workspace:  hc.f.Workspace,
	}

	resp := handler.Run(ctx, ictx)
	if resp.Error != nil {
		logger.ErrorCF("homeclaw", "intent handler error",
			map[string]any{
				"intent": string(result.Type),
				"error":  resp.Error.Error(),
			})
	}
	return resp.Response, resp.Handled, resp.ForwardToLLM, resp.Error
}

// ─────────────────────────────────────────────────────────────────────────────
// HomeClaw tool registration
// ─────────────────────────────────────────────────────────────────────────────

// registerTool is a helper that calls a factory method, logs on error, and
// registers the resulting tool when successful.
func registerTool[T tools.Tool](toolRegistry *tools.ToolRegistry, create func() (T, error)) {
	t, err := create()
	if err != nil {
		logger.WarnCF("homeclaw", "tool creation failed, skipping",
			map[string]any{"error": err.Error()})
		return
	}
	toolRegistry.Register(t)
}

// RegisterTools registers all HomeClaw tools (device, space, workflow)
// into the given tool registry.
// It is safe to call when hc is nil — the method becomes a no-op.
func (hc *HomeClaw) RegisterTools(toolRegistry *tools.ToolRegistry) {
	if hc == nil || toolRegistry == nil {
		return
	}

	f := hc.f

	// Workflow tools
	registerTool(toolRegistry, f.GetListWorkflowsTool)
	registerTool(toolRegistry, f.GetGetWorkflowTool)
	registerTool(toolRegistry, f.GetSaveWorkflowTool)
	registerTool(toolRegistry, f.GetDeleteWorkflowTool)
	registerTool(toolRegistry, f.GetEnableWorkflowTool)
	registerTool(toolRegistry, f.GetDisableWorkflowTool)

	// Video / RTSP tools
	registerTool(toolRegistry, f.GetVideoTool)

	// LLM tools
	registerTool(toolRegistry, f.GetLLMTool)

	// Common tools
	registerTool(toolRegistry, f.GetCommonTool)

	// CLI tool for device control
	registerTool(toolRegistry, f.GetCLITool)
}

// SetMediaStore sets the media store for HomeClaw tools that need to send images to channels.
func (hc *HomeClaw) SetMediaStore(store media.MediaStore) {
	if hc == nil || hc.f == nil {
		return
	}
	hc.f.SetMediaStore(store)
}

// SetClients initializes and registers all third-party brand clients (Xiaomi, Tuya, etc.)
// and injects them into the CLI and LLM tools.
func (hc *HomeClaw) SetClients() error {
	if hc == nil || hc.thirdf == nil {
		return nil
	}
	return hc.thirdf.SetClients()
}

// ─────────────────────────────────────────────────────────────────────────────
// Device command handling via hc_cli tool
// ─────────────────────────────────────────────────────────────────────────────

// HandleToolCall checks if the message is a tool command (format: "tool:name" + JSON params)
// and executes it via the specified tool directly, bypassing the LLM.
// Returns (response, handled) where handled=true means the command was processed.
func (hc *HomeClaw) HandleToolCall(ctx context.Context, channel, chatID, content string, toolRegistry *tools.ToolRegistry) (string, bool) {
	if hc == nil || toolRegistry == nil {
		return "", false
	}

	// Parse tool name and command JSON from content
	toolName, commandJSON, ok := hc.ParseToolCommand(content)
	if !ok {
		return "", false
	}

	logger.InfoCF("homeclaw", "Tool command detected, executing via tool",
		map[string]any{
			"channel":   channel,
			"chat_id":   chatID,
			"tool_name": toolName,
		})

	// Get the specified tool from registry
	tool, ok := toolRegistry.Get(toolName)
	if !ok {
		logger.ErrorCF("homeclaw", "Tool not found",
			map[string]any{
				"tool_name":       toolName,
				"available_tools": toolRegistry.List(),
			})
		return fmt.Sprintf("工具执行失败：工具 '%s' 未注册", toolName), true
	}

	// Execute the tool with the command JSON
	toolArgs := map[string]any{
		"commandJson": commandJSON,
	}

	logger.DebugCF("homeclaw", "Executing tool",
		map[string]any{
			"tool_name":    toolName,
			"command_json": commandJSON,
		})

	result := tool.Execute(ctx, toolArgs)

	if result.IsError {
		logger.ErrorCF("homeclaw", "Tool execution failed",
			map[string]any{
				"tool_name": toolName,
				"error":     result.ForLLM,
			})
		return fmt.Sprintf("工具执行失败：%s", result.ForLLM), true
	}

	logger.InfoCF("homeclaw", "Tool executed successfully",
		map[string]any{
			"tool_name":     toolName,
			"result_length": len(result.ForLLM),
		})

	return result.ForLLM, true
}

// ParseToolCommand parses the content to extract tool name and command JSON.
// Expected format: "tool:toolName {json_params}"
// Returns (toolName, commandJSON, success)
func (hc *HomeClaw) ParseToolCommand(content string) (string, string, bool) {
	content = strings.TrimSpace(content)

	// Check if content starts with "tool:"
	if !strings.HasPrefix(content, "tool:") {
		return "", "", false
	}

	// Remove "tool:" prefix
	content = content[5:]

	// Find the first space to separate tool name from JSON
	spaceIdx := strings.Index(content, " ")
	if spaceIdx == -1 {
		return "", "", false
	}

	toolName := strings.TrimSpace(content[:spaceIdx])
	if toolName == "" {
		return "", "", false
	}

	// Extract JSON part
	commandJSON := strings.TrimSpace(content[spaceIdx+1:])
	if commandJSON == "" {
		return "", "", false
	}

	// Validate JSON format
	var cmd map[string]interface{}
	if err := json.Unmarshal([]byte(commandJSON), &cmd); err != nil {
		return "", "", false
	}

	return toolName, commandJSON, true
}

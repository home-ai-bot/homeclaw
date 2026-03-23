// Package homeclaw provides the HomeClaw subsystem for intent recognition
// and workflow dispatching.  The HomeClaw type is the single entry point
// consumed by the agent loop.
package homeclaw

import (
	"context"
	"errors"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/intent"
	"github.com/sipeed/picoclaw/pkg/homeclaw/ioc"
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
	f *ioc.Factory
}

// New creates a HomeClaw instance from the given workspace directory,
// PicoClaw config, and message bus.
// workspace is the data root used for all HomeClaw data files (users, devices, workflows …).
// Returns nil (no error) when HomeClaw is disabled or homeclaw.json is absent –
// the caller should treat nil as "not configured".
func New(workspace string, picolawerCfg *config.Config, msgBus *bus.MessageBus) (*HomeClaw, error) {
	// Create factory which handles all singleton object creation
	factory, err := ioc.NewFactory(workspace, picolawerCfg, msgBus)
	if err != nil {
		if errors.Is(err, ErrDisabled) {
			return nil, ErrDisabled
		}
		return nil, fmt.Errorf("HomeClaw factory creation failed: %w", err)
	}

	return &HomeClaw{
		f: factory,
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

// RegisterTools registers all HomeClaw tools (device, space, member, workflow)
// into the given tool registry.
// It is safe to call when hc is nil — the method becomes a no-op.
func (hc *HomeClaw) RegisterTools(toolRegistry *tools.ToolRegistry) {
	if hc == nil || toolRegistry == nil {
		return
	}

	f := hc.f

	// Device tools
	registerTool(toolRegistry, f.GetListDevicesTool)

	// Space tools
	registerTool(toolRegistry, f.GetListSpacesTool)
	registerTool(toolRegistry, f.GetGetSpaceTool)
	registerTool(toolRegistry, f.GetSaveSpaceTool)
	registerTool(toolRegistry, f.GetDeleteSpaceTool)

	// Member tools
	registerTool(toolRegistry, f.GetListMembersTool)
	registerTool(toolRegistry, f.GetGetMemberTool)
	registerTool(toolRegistry, f.GetSaveMemberTool)
	registerTool(toolRegistry, f.GetDeleteMemberTool)

	// Workflow tools
	registerTool(toolRegistry, f.GetListWorkflowsTool)
	registerTool(toolRegistry, f.GetGetWorkflowTool)
	registerTool(toolRegistry, f.GetSaveWorkflowTool)
	registerTool(toolRegistry, f.GetDeleteWorkflowTool)
	registerTool(toolRegistry, f.GetEnableWorkflowTool)
	registerTool(toolRegistry, f.GetDisableWorkflowTool)

	// Mi Home (miio) tools
	registerTool(toolRegistry, f.GetGetXiaomiAccountTool)
	registerTool(toolRegistry, f.GetUpdateXiaomiHomeTool)
	registerTool(toolRegistry, f.GetGetXiaomiOAuthURLTool)
	registerTool(toolRegistry, f.GetGetXiaomiAccessTokenTool)
	registerTool(toolRegistry, f.GetSyncXiaomiHomesTool)
	registerTool(toolRegistry, f.GetSyncXiaomiRoomsTool)
	registerTool(toolRegistry, f.GetSyncXiaomiDevicesTool)
	registerTool(toolRegistry, f.GetGetXiaomiSpecTool)
	registerTool(toolRegistry, f.GetXiaomiActionTool)
	registerTool(toolRegistry, f.GetSetXiaomiPropTool)
	registerTool(toolRegistry, f.GetMiSendEmailCodeTool)
	registerTool(toolRegistry, f.GetMiLoginEmailTool)

	// Video / RTSP tools
	registerTool(toolRegistry, f.GetRTSPAnalyzeTool)
}

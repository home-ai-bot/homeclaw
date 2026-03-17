// Package homeclaw provides the HomeClaw subsystem for intent recognition
// and workflow dispatching.  The HomeClaw type is the single entry point
// consumed by the agent loop.
package homeclaw

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	common "github.com/sipeed/picoclaw/pkg/homeclaw/common"
	homeclawconfig "github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/intent"
	"github.com/sipeed/picoclaw/pkg/homeclaw/miio"
	homeclawtool "github.com/sipeed/picoclaw/pkg/homeclaw/tool"
	"github.com/sipeed/picoclaw/pkg/logger"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// HomeClaw holds all HomeClaw subsystem objects and exposes a single
// RunIntent method that the agent loop calls from processMessage.
type HomeClaw struct {
	classifier intent.IntentClassifier
	router     *intent.Router
	// workspace is the data root directory for all HomeClaw data files
	// (users, devices, workflows, etc.).
	workspace string
	// provider is the small LLM used for intent classification and workflow
	// matching (e.g. selecting a workflow by comparing GetAllMeta results
	// against the user's message).
	provider providers.LLMProvider
	// modelName is the model identifier passed to provider when making calls.
	modelName string

	// data stores – kept for tool registration
	deviceStore        data.DeviceStore
	spaceStore         data.SpaceStore
	memberStore        data.MemberStore
	workflowStore      data.WorkflowStore
	xiaomiAccountStore data.XiaomiAccountStore

	// miio OAuth client for Xiaomi tools
	oauthClient *miio.MIoTOauthClient

	// bus is the message bus for publishing outbound messages.
	bus *bus.MessageBus
	// cfg is the PicoClaw configuration.
	cfg *config.Config
	// hcfg is the HomeClaw-specific configuration.
	hcfg *homeclawconfig.HomeclawConfig
}

// New creates a HomeClaw instance from the given workspace directory,
// PicoClaw config, and message bus.
// workspace is the data root used for all HomeClaw data files (users, devices, workflows …).
// Returns nil (no error) when HomeClaw is disabled or homeclaw.json is absent –
// the caller should treat nil as "not configured".
func New(workspace string, picolawerCfg *config.Config, msgBus *bus.MessageBus) (*HomeClaw, error) {
	hcfg, err := loadHomeclawConfig()
	if err != nil {
		return nil, fmt.Errorf("HomeClaw config load error: %w", err)
	}
	if hcfg == nil || !hcfg.Enabled {
		return nil, nil
	}

	smallProvider, modelName, provErr := resolveIntentProvider(hcfg, picolawerCfg)
	if provErr != nil {
		return nil, fmt.Errorf("HomeClaw intent provider setup failed: %w", provErr)
	}

	classifier := intent.NewLLMClassifier(smallProvider, hcfg, modelName)

	// Initialise data stores backed by the workspace/data directory.
	jsonStore, err := data.NewJSONStore(filepath.Join(workspace, "data"))
	if err != nil {
		return nil, fmt.Errorf("HomeClaw data store init failed: %w", err)
	}
	deviceStore, err := data.NewDeviceStore(jsonStore)
	if err != nil {
		return nil, fmt.Errorf("HomeClaw device store init failed: %w", err)
	}
	spaceStore, err := data.NewSpaceStore(jsonStore)
	if err != nil {
		return nil, fmt.Errorf("HomeClaw space store init failed: %w", err)
	}
	memberStore, err := data.NewMemberStore(jsonStore)
	if err != nil {
		return nil, fmt.Errorf("HomeClaw member store init failed: %w", err)
	}

	// Initialise workflow store.
	workflowStore, err := data.NewWorkflowStore(jsonStore)
	if err != nil {
		return nil, fmt.Errorf("HomeClaw workflow store init failed: %w", err)
	}

	// Initialise Xiaomi account store.
	xiaomiAccountStore, err := data.NewXiaomiAccountStore(jsonStore)
	if err != nil {
		return nil, fmt.Errorf("HomeClaw xiaomi account store init failed: %w", err)
	}
	// 获取 xiaomiAccount 信息
	xiaomiAccount, err := xiaomiAccountStore.Get()
	if err != nil {
		if errors.Is(err, data.ErrRecordNotFound) {
			xiaomiAccount = &data.XiaomiAccount{}
			xiaomiAccount.ClientID = miio.OAuth2ClientID
			xiaomiAccount.ID = common.GenerateUUID()
		}

	}
	// Initialise MIoT OAuth client for Xiaomi tools.
	oauthClient, err := miio.NewMIoTOauthClient("", "", "", "")
	if err != nil {
		return nil, fmt.Errorf("HomeClaw MIoT OAuth client init failed: %w", err)
	}

	chatHandler := &intent.ChatIntent{}
	deviceControlHandler := intent.NewDeviceControlIntent(nil, nil, smallProvider, modelName)
	router := intent.NewRouter(
		chatHandler,
		deviceControlHandler,
		intent.NewDeviceMgmtIntent(deviceStore, spaceStore),
		intent.NewSpaceMgmtIntent(spaceStore),
		intent.NewUserMgmtIntent(memberStore),
		&intent.SystemConfigIntent{},
	)

	logger.InfoCF("homeclaw", "HomeClaw intent processing enabled",
		map[string]any{"workspace": workspace, "model": modelName})

	return &HomeClaw{
		classifier:         classifier,
		router:             router,
		workspace:          workspace,
		provider:           smallProvider,
		modelName:          modelName,
		deviceStore:        deviceStore,
		spaceStore:         spaceStore,
		memberStore:        memberStore,
		workflowStore:      workflowStore,
		xiaomiAccountStore: xiaomiAccountStore,
		oauthClient:        oauthClient,
		bus:                msgBus,
		cfg:                picolawerCfg,
		hcfg:               hcfg,
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
	if hc.hcfg != nil && !hc.hcfg.IntentEnabled {
		return "", false, false, nil
	}

	result, classErr := hc.classifier.Classify(ctx, in.UserInput)
	if classErr != nil {
		logger.WarnCF("homeclaw", "intent classification error, falling through",
			map[string]any{"error": classErr.Error()})
	}
	if result.Type == intent.IntentUnknown {
		return "", false, false, nil
	}

	handler, ok := hc.router.Route(result)
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
		Workspace:  hc.workspace,
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

// RegisterTools registers all HomeClaw tools (device, space, member, workflow)
// into the given tool registry.
// It is safe to call when hc is nil — the method becomes a no-op.
func (hc *HomeClaw) RegisterTools(toolRegistry *tools.ToolRegistry) {
	if hc == nil || toolRegistry == nil {
		return
	}

	// Device tools
	toolRegistry.Register(homeclawtool.NewListDevicesTool(hc.deviceStore))
	toolRegistry.Register(homeclawtool.NewGetDeviceTool(hc.deviceStore))
	toolRegistry.Register(homeclawtool.NewSaveDeviceTool(hc.deviceStore))
	toolRegistry.Register(homeclawtool.NewUpdateDeviceStateTool(hc.deviceStore))
	toolRegistry.Register(homeclawtool.NewDeleteDeviceTool(hc.deviceStore))

	// Space tools
	toolRegistry.Register(homeclawtool.NewListSpacesTool(hc.spaceStore))
	toolRegistry.Register(homeclawtool.NewGetSpaceTool(hc.spaceStore))
	toolRegistry.Register(homeclawtool.NewSaveSpaceTool(hc.spaceStore))
	toolRegistry.Register(homeclawtool.NewDeleteSpaceTool(hc.spaceStore))

	// Member tools
	toolRegistry.Register(homeclawtool.NewListMembersTool(hc.memberStore))
	toolRegistry.Register(homeclawtool.NewGetMemberTool(hc.memberStore))
	toolRegistry.Register(homeclawtool.NewSaveMemberTool(hc.memberStore))
	toolRegistry.Register(homeclawtool.NewDeleteMemberTool(hc.memberStore))

	// Workflow tools
	toolRegistry.Register(homeclawtool.NewListWorkflowsTool(hc.workflowStore))
	toolRegistry.Register(homeclawtool.NewGetWorkflowTool(hc.workflowStore))
	toolRegistry.Register(homeclawtool.NewSaveWorkflowTool(hc.workflowStore))
	toolRegistry.Register(homeclawtool.NewDeleteWorkflowTool(hc.workflowStore))
	toolRegistry.Register(homeclawtool.NewEnableWorkflowTool(hc.workflowStore))
	toolRegistry.Register(homeclawtool.NewDisableWorkflowTool(hc.workflowStore))

	// Mi Home (miio) tools
	toolRegistry.Register(homeclawtool.NewGetXiaomiAccountTool(hc.xiaomiAccountStore, hc.oauthClient))
	toolRegistry.Register(homeclawtool.NewUpdateXiaomiTokenTool(hc.xiaomiAccountStore))
	toolRegistry.Register(homeclawtool.NewUpdateXiaomiHomeTool(hc.xiaomiAccountStore))
	toolRegistry.Register(homeclawtool.NewGetXiaomiOAuthURLTool(hc.oauthClient))
	toolRegistry.Register(homeclawtool.NewGetXiaomiAccessTokenTool(hc.xiaomiAccountStore, hc.oauthClient))
	toolRegistry.Register(homeclawtool.NewSyncXiaomiHomesTool(hc.xiaomiAccountStore, hc.oauthClient))
	toolRegistry.Register(homeclawtool.NewSyncXiaomiRoomsTool(hc.xiaomiAccountStore, hc.spaceStore, hc.oauthClient))
	toolRegistry.Register(homeclawtool.NewSyncXiaomiDevicesTool(hc.xiaomiAccountStore, hc.deviceStore, hc.oauthClient))
}

// ─────────────────────────────────────────────────────────────────────────────
// internal helpers
// ─────────────────────────────────────────────────────────────────────────────

// resolveIntentProvider builds an LLMProvider for the small intent model.
func resolveIntentProvider(
	hcfg *homeclawconfig.HomeclawConfig,
	picolawerCfg *config.Config,
) (providers.LLMProvider, string, error) {
	mc := hcfg.IntentModel

	if mc.IsModelRef() {
		for i := range picolawerCfg.ModelList {
			if picolawerCfg.ModelList[i].ModelName == mc.ModelRef {
				p, modelID, err := providers.CreateProviderFromConfig(&picolawerCfg.ModelList[i])
				if err != nil {
					return nil, "", fmt.Errorf("intent model_ref %q: %w", mc.ModelRef, err)
				}
				return p, modelID, nil
			}
		}
		return nil, "", fmt.Errorf("intent model_ref %q not found in model_list", mc.ModelRef)
	}

	if mc.Model == "" {
		return nil, "", fmt.Errorf("intent_model: model is required when model_ref is not set")
	}
	modelCfg := &config.ModelConfig{
		ModelName: "homeclaw-intent",
		Model:     mc.Model,
		APIBase:   mc.APIBase,
		APIKey:    mc.APIKey,
	}
	p, modelID, err := providers.CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, "", fmt.Errorf("intent inline provider: %w", err)
	}
	return p, modelID, nil
}

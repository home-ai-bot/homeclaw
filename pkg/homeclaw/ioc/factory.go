// Package ioc provides the HomeClaw subsystem for intent recognition
// and workflow dispatching. The Factory provides centralized object creation
// and singleton management for all HomeClaw components.
package ioc

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/common"
	homeclawconfig "github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/event"
	"github.com/sipeed/picoclaw/pkg/homeclaw/intent"
	"github.com/sipeed/picoclaw/pkg/homeclaw/miio"
	homeclawtool "github.com/sipeed/picoclaw/pkg/homeclaw/tool"
	"github.com/sipeed/picoclaw/pkg/homeclaw/workflow"
	"github.com/sipeed/picoclaw/pkg/providers"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ErrDisabled is returned by NewFactory when HomeClaw is explicitly disabled or
// homeclaw.json is absent. Callers can use errors.Is(err, ErrDisabled) to
// distinguish a deliberate no-op from a real initialisation failure.
var ErrDisabled = fmt.Errorf("homeclaw is disabled")

// Factory is the central factory for creating and managing all HomeClaw objects.
// It follows the singleton pattern for components that should exist only once
// per application lifecycle.
type Factory struct {
	Workspace string
	cfg       *config.Config
	bus       *bus.MessageBus
	hcfg      *homeclawconfig.HomeclawConfig

	// Singleton instances - lazy loaded
	jsonStore          *data.JSONStore
	deviceStore        data.DeviceStore
	spaceStore         data.SpaceStore
	memberStore        data.MemberStore
	workflowStore      data.WorkflowStore
	xiaomiAccountStore data.XiaomiAccountStore
	eventCenter        *event.Center
	oauthClient        *miio.MIoTOauthClient
	classifier         intent.IntentClassifier
	router             *intent.Router
	workflowEngine     workflow.Engine
	toolRegistry       *tools.ToolRegistry

	// Provider for intent classification
	provider  providers.LLMProvider
	modelName string

	// Initialization tracking
	storeOnce sync.Once
	storeErr  error
}

// NewFactory creates a new Factory instance.
// workspace is the data root used for all HomeClaw data files.
// Returns error when HomeClaw is disabled or homeclaw.json is absent.
func NewFactory(workspace string, picoclawCfg *config.Config, msgBus *bus.MessageBus) (*Factory, error) {
	hcfg, err := homeclawconfig.LoadFromDir(workspace)
	if err != nil {
		return nil, fmt.Errorf("homeclaw config load error: %w", err)
	}
	if hcfg == nil || !hcfg.Enabled {
		return nil, ErrDisabled
	}

	return &Factory{
		Workspace: workspace,
		cfg:       picoclawCfg,
		bus:       msgBus,
		hcfg:      hcfg,
	}, nil
}

// GetHomeclawConfig returns the HomeClaw configuration
func (f *Factory) GetHomeclawConfig() *homeclawconfig.HomeclawConfig {
	return f.hcfg
}

// GetJSONStore returns the singleton JSONStore instance (lazy initialized)
func (f *Factory) GetJSONStore() (*data.JSONStore, error) {
	f.storeOnce.Do(func() {
		f.jsonStore, f.storeErr = data.NewJSONStore(filepath.Join(f.Workspace, "data"))
	})
	return f.jsonStore, f.storeErr
}

// GetDeviceStore returns the singleton DeviceStore instance (lazy initialized)
func (f *Factory) GetDeviceStore() (data.DeviceStore, error) {
	if f.deviceStore != nil {
		return f.deviceStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, err
	}

	f.deviceStore, err = data.NewDeviceStore(store)
	if err != nil {
		return nil, fmt.Errorf("device store init failed: %w", err)
	}
	return f.deviceStore, nil
}

// GetSpaceStore returns the singleton SpaceStore instance (lazy initialized)
func (f *Factory) GetSpaceStore() (data.SpaceStore, error) {
	if f.spaceStore != nil {
		return f.spaceStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, err
	}

	f.spaceStore, err = data.NewSpaceStore(store)
	if err != nil {
		return nil, fmt.Errorf("space store init failed: %w", err)
	}
	return f.spaceStore, nil
}

// GetMemberStore returns the singleton MemberStore instance (lazy initialized)
func (f *Factory) GetMemberStore() (data.MemberStore, error) {
	if f.memberStore != nil {
		return f.memberStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, err
	}

	f.memberStore, err = data.NewMemberStore(store)
	if err != nil {
		return nil, fmt.Errorf("member store init failed: %w", err)
	}
	return f.memberStore, nil
}

// GetWorkflowStore returns the singleton WorkflowStore instance (lazy initialized)
func (f *Factory) GetWorkflowStore() (data.WorkflowStore, error) {
	if f.workflowStore != nil {
		return f.workflowStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, err
	}

	f.workflowStore, err = data.NewWorkflowStore(store)
	if err != nil {
		return nil, fmt.Errorf("workflow store init failed: %w", err)
	}
	return f.workflowStore, nil
}

// GetXiaomiAccountStore returns the singleton XiaomiAccountStore instance (lazy initialized)
func (f *Factory) GetXiaomiAccountStore() (data.XiaomiAccountStore, error) {
	if f.xiaomiAccountStore != nil {
		return f.xiaomiAccountStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, err
	}

	f.xiaomiAccountStore, err = data.NewXiaomiAccountStore(store)
	if err != nil {
		return nil, fmt.Errorf("xiaomi account store init failed: %w", err)
	}
	return f.xiaomiAccountStore, nil
}

// GetEventCenter returns the singleton EventCenter instance
func (f *Factory) GetEventCenter() *event.Center {
	if f.eventCenter == nil {
		f.eventCenter = event.GetCenter()
	}
	return f.eventCenter
}

// SetToolRegistry sets the tool registry for workflow engine initialization
func (f *Factory) SetToolRegistry(registry *tools.ToolRegistry) {
	f.toolRegistry = registry
}

// GetToolRegistry returns the tool registry
func (f *Factory) GetToolRegistry() *tools.ToolRegistry {
	return f.toolRegistry
}

// GetWorkflowEngine returns the singleton WorkflowEngine instance (lazy initialized)
func (f *Factory) GetWorkflowEngine() workflow.Engine {
	if f.workflowEngine != nil {
		return f.workflowEngine
	}
	f.workflowEngine = workflow.NewEngine(f.toolRegistry)
	return f.workflowEngine
}

// GetIntentProvider returns the LLM provider for intent classification (lazy initialized)
func (f *Factory) GetIntentProvider() (providers.LLMProvider, error) {
	if f.provider != nil {
		return f.provider, nil
	}

	mc := f.hcfg.IntentModel

	if mc.IsModelName() {
		for i := range f.cfg.ModelList {
			if f.cfg.ModelList[i].ModelName == mc.ModelName {
				p, modelID, err := providers.CreateProviderFromConfig(&f.cfg.ModelList[i])
				if err != nil {
					return nil, fmt.Errorf("intent model_ref %q: %w", mc.ModelName, err)
				}
				f.provider = p
				f.modelName = modelID
				return f.provider, nil
			}
		}
		return nil, fmt.Errorf("intent model_ref %q not found in model_list", mc.ModelName)
	}

	if mc.Model == "" {
		return nil, fmt.Errorf("intent_model: model is required when model_ref is not set")
	}

	modelCfg := &config.ModelConfig{
		ModelName: mc.ModelName,
		Model:     mc.Model,
		APIBase:   mc.APIBase,
		APIKey:    mc.APIKey,
	}
	p, _, err := providers.CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, fmt.Errorf("intent inline provider: %w", err)
	}
	f.provider = p
	f.modelName = mc.ModelName
	return f.provider, nil
}

// GetIntentClassifier returns the singleton IntentClassifier instance (lazy initialized)
func (f *Factory) GetIntentClassifier() (intent.IntentClassifier, error) {
	if f.classifier != nil {
		return f.classifier, nil
	}

	provider, err := f.GetIntentProvider()
	if err != nil {
		return nil, err
	}

	f.classifier = intent.NewLLMClassifier(provider, f.hcfg, f.hcfg.IntentModel.ModelName)
	return f.classifier, nil
}

// GetIntentRouter returns the singleton IntentRouter instance (lazy initialized)
func (f *Factory) GetIntentRouter() (*intent.Router, error) {
	if f.router != nil {
		return f.router, nil
	}

	if !f.hcfg.IntentEnabled {
		return nil, fmt.Errorf("intent processing is disabled")
	}

	provider, err := f.GetIntentProvider()
	if err != nil {
		return nil, err
	}

	deviceStore, err := f.GetDeviceStore()
	if err != nil {
		return nil, err
	}

	spaceStore, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}

	memberStore, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}

	chatHandler := &intent.ChatIntent{}

	workflowStore, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}

	workflowEngine := f.GetWorkflowEngine()
	deviceControlHandler := intent.NewDeviceControlIntent(workflowStore, workflowEngine, provider, f.hcfg.IntentModel.ModelName)
	f.router = intent.NewRouter(
		chatHandler,
		deviceControlHandler,
		intent.NewDeviceMgmtIntent(deviceStore, spaceStore),
		intent.NewSpaceMgmtIntent(spaceStore),
		intent.NewUserMgmtIntent(memberStore),
		&intent.SystemConfigIntent{},
	)

	return f.router, nil
}

// GetMIoTOAuthClient returns the singleton MIoT OAuth client (lazy initialized)
func (f *Factory) GetMIoTOAuthClient() (*miio.MIoTOauthClient, error) {
	if f.oauthClient != nil {
		return f.oauthClient, nil
	}

	xiaomiAccountStore, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}

	xiaomiAccount, err := xiaomiAccountStore.Get()
	if err != nil {
		if err != data.ErrRecordNotFound {
			return nil, fmt.Errorf("xiaomi account load failed: %w", err)
		}
		// Create default account if not exists
		xiaomiAccount = &data.XiaomiAccount{
			ClientID: miio.OAuth2ClientID,
			ID:       common.GenerateUUID(),
		}
		if saveErr := xiaomiAccountStore.Save(*xiaomiAccount); saveErr != nil {
			return nil, fmt.Errorf("xiaomi account save failed: %w", saveErr)
		}
	}

	f.oauthClient, err = miio.NewMIoTOauthClient(xiaomiAccount.ClientID, "", "cn", xiaomiAccount.ID)
	if err != nil {
		return nil, fmt.Errorf("MIoT OAuth client init failed: %w", err)
	}
	return f.oauthClient, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool factory methods
// ─────────────────────────────────────────────────────────────────────────────

// NewListDevicesTool creates a ListDevicesTool
func (f *Factory) NewListDevicesTool() (*homeclawtool.ListDevicesTool, error) {
	store, err := f.GetDeviceStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewListDevicesTool(store), nil
}

// NewGetDeviceTool creates a GetDeviceTool
func (f *Factory) NewGetDeviceTool() (*homeclawtool.GetDeviceTool, error) {
	store, err := f.GetDeviceStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewGetDeviceTool(store), nil
}

// NewListSpacesTool creates a ListSpacesTool
func (f *Factory) NewListSpacesTool() (*homeclawtool.ListSpacesTool, error) {
	store, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewListSpacesTool(store), nil
}

// NewGetSpaceTool creates a GetSpaceTool
func (f *Factory) NewGetSpaceTool() (*homeclawtool.GetSpaceTool, error) {
	store, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewGetSpaceTool(store), nil
}

// NewSaveSpaceTool creates a SaveSpaceTool
func (f *Factory) NewSaveSpaceTool() (*homeclawtool.SaveSpaceTool, error) {
	store, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewSaveSpaceTool(store), nil
}

// NewDeleteSpaceTool creates a DeleteSpaceTool
func (f *Factory) NewDeleteSpaceTool() (*homeclawtool.DeleteSpaceTool, error) {
	store, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewDeleteSpaceTool(store), nil
}

// NewListMembersTool creates a ListMembersTool
func (f *Factory) NewListMembersTool() (*homeclawtool.ListMembersTool, error) {
	store, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewListMembersTool(store), nil
}

// NewGetMemberTool creates a GetMemberTool
func (f *Factory) NewGetMemberTool() (*homeclawtool.GetMemberTool, error) {
	store, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewGetMemberTool(store), nil
}

// NewSaveMemberTool creates a SaveMemberTool
func (f *Factory) NewSaveMemberTool() (*homeclawtool.SaveMemberTool, error) {
	store, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewSaveMemberTool(store), nil
}

// NewDeleteMemberTool creates a DeleteMemberTool
func (f *Factory) NewDeleteMemberTool() (*homeclawtool.DeleteMemberTool, error) {
	store, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewDeleteMemberTool(store), nil
}

// NewListWorkflowsTool creates a ListWorkflowsTool
func (f *Factory) NewListWorkflowsTool() (*homeclawtool.ListWorkflowsTool, error) {
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewListWorkflowsTool(store), nil
}

// NewGetWorkflowTool creates a GetWorkflowTool
func (f *Factory) NewGetWorkflowTool() (*homeclawtool.GetWorkflowTool, error) {
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewGetWorkflowTool(store), nil
}

// NewSaveWorkflowTool creates a SaveWorkflowTool
func (f *Factory) NewSaveWorkflowTool() (*homeclawtool.SaveWorkflowTool, error) {
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewSaveWorkflowTool(store), nil
}

// NewDeleteWorkflowTool creates a DeleteWorkflowTool
func (f *Factory) NewDeleteWorkflowTool() (*homeclawtool.DeleteWorkflowTool, error) {
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewDeleteWorkflowTool(store), nil
}

// NewEnableWorkflowTool creates an EnableWorkflowTool
func (f *Factory) NewEnableWorkflowTool() (*homeclawtool.EnableWorkflowTool, error) {
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewEnableWorkflowTool(store), nil
}

// NewDisableWorkflowTool creates a DisableWorkflowTool
func (f *Factory) NewDisableWorkflowTool() (*homeclawtool.DisableWorkflowTool, error) {
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewDisableWorkflowTool(store), nil
}

// NewGetXiaomiAccountTool creates a GetXiaomiAccountTool
func (f *Factory) NewGetXiaomiAccountTool() (*homeclawtool.GetXiaomiAccountTool, error) {
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewGetXiaomiAccountTool(store, oauthClient), nil
}

// NewUpdateXiaomiTokenTool creates an UpdateXiaomiTokenTool
func (f *Factory) NewUpdateXiaomiTokenTool() (*homeclawtool.UpdateXiaomiTokenTool, error) {
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewUpdateXiaomiTokenTool(store), nil
}

// NewUpdateXiaomiHomeTool creates an UpdateXiaomiHomeTool
func (f *Factory) NewUpdateXiaomiHomeTool() (*homeclawtool.UpdateXiaomiHomeTool, error) {
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewUpdateXiaomiHomeTool(store), nil
}

// NewGetXiaomiOAuthURLTool creates a GetXiaomiOAuthURLTool
func (f *Factory) NewGetXiaomiOAuthURLTool() (*homeclawtool.GetXiaomiOAuthURLTool, error) {
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewGetXiaomiOAuthURLTool(oauthClient), nil
}

// NewGetXiaomiAccessTokenTool creates a GetXiaomiAccessTokenTool
func (f *Factory) NewGetXiaomiAccessTokenTool() (*homeclawtool.GetXiaomiAccessTokenTool, error) {
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewGetXiaomiAccessTokenTool(store, oauthClient), nil
}

// NewSyncXiaomiHomesTool creates a SyncXiaomiHomesTool
func (f *Factory) NewSyncXiaomiHomesTool() (*homeclawtool.SyncXiaomiHomesTool, error) {
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewSyncXiaomiHomesTool(store, oauthClient), nil
}

// NewSyncXiaomiRoomsTool creates a SyncXiaomiRoomsTool
func (f *Factory) NewSyncXiaomiRoomsTool() (*homeclawtool.SyncXiaomiRoomsTool, error) {
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	spaceStore, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewSyncXiaomiRoomsTool(store, spaceStore, oauthClient), nil
}

// NewSyncXiaomiDevicesTool creates a SyncXiaomiDevicesTool
func (f *Factory) NewSyncXiaomiDevicesTool() (*homeclawtool.SyncXiaomiDevicesTool, error) {
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	deviceStore, err := f.GetDeviceStore()
	if err != nil {
		return nil, err
	}
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	return homeclawtool.NewSyncXiaomiDevicesTool(store, deviceStore, oauthClient), nil
}

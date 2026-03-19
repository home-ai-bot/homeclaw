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

	// Tool singleton instances - lazy loaded
	listDevicesTool          *homeclawtool.ListDevicesTool
	getDeviceTool            *homeclawtool.GetDeviceTool
	listSpacesTool           *homeclawtool.ListSpacesTool
	getSpaceTool             *homeclawtool.GetSpaceTool
	saveSpaceTool            *homeclawtool.SaveSpaceTool
	deleteSpaceTool          *homeclawtool.DeleteSpaceTool
	listMembersTool          *homeclawtool.ListMembersTool
	getMemberTool            *homeclawtool.GetMemberTool
	saveMemberTool           *homeclawtool.SaveMemberTool
	deleteMemberTool         *homeclawtool.DeleteMemberTool
	listWorkflowsTool        *homeclawtool.ListWorkflowsTool
	getWorkflowTool          *homeclawtool.GetWorkflowTool
	saveWorkflowTool         *homeclawtool.SaveWorkflowTool
	deleteWorkflowTool       *homeclawtool.DeleteWorkflowTool
	enableWorkflowTool       *homeclawtool.EnableWorkflowTool
	disableWorkflowTool      *homeclawtool.DisableWorkflowTool
	getXiaomiAccountTool     *homeclawtool.GetXiaomiAccountTool
	updateXiaomiHomeTool     *homeclawtool.UpdateXiaomiHomeTool
	getXiaomiOAuthURLTool    *homeclawtool.GetXiaomiOAuthURLTool
	getXiaomiAccessTokenTool *homeclawtool.GetXiaomiAccessTokenTool
	syncXiaomiHomesTool      *homeclawtool.SyncXiaomiHomesTool
	syncXiaomiRoomsTool      *homeclawtool.SyncXiaomiRoomsTool
	syncXiaomiDevicesTool    *homeclawtool.SyncXiaomiDevicesTool
	cloudClient              *miio.CloudClient
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

// GetListDevicesTool returns the singleton ListDevicesTool instance (lazy initialized)
func (f *Factory) GetListDevicesTool() (*homeclawtool.ListDevicesTool, error) {
	if f.listDevicesTool != nil {
		return f.listDevicesTool, nil
	}
	store, err := f.GetDeviceStore()
	if err != nil {
		return nil, err
	}
	f.listDevicesTool = homeclawtool.NewListDevicesTool(store)
	return f.listDevicesTool, nil
}

// GetGetDeviceTool returns the singleton GetDeviceTool instance (lazy initialized)
func (f *Factory) GetGetDeviceTool() (*homeclawtool.GetDeviceTool, error) {
	if f.getDeviceTool != nil {
		return f.getDeviceTool, nil
	}
	store, err := f.GetDeviceStore()
	if err != nil {
		return nil, err
	}
	f.getDeviceTool = homeclawtool.NewGetDeviceTool(store)
	return f.getDeviceTool, nil
}

// GetListSpacesTool returns the singleton ListSpacesTool instance (lazy initialized)
func (f *Factory) GetListSpacesTool() (*homeclawtool.ListSpacesTool, error) {
	if f.listSpacesTool != nil {
		return f.listSpacesTool, nil
	}
	store, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	f.listSpacesTool = homeclawtool.NewListSpacesTool(store)
	return f.listSpacesTool, nil
}

// GetGetSpaceTool returns the singleton GetSpaceTool instance (lazy initialized)
func (f *Factory) GetGetSpaceTool() (*homeclawtool.GetSpaceTool, error) {
	if f.getSpaceTool != nil {
		return f.getSpaceTool, nil
	}
	store, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	f.getSpaceTool = homeclawtool.NewGetSpaceTool(store)
	return f.getSpaceTool, nil
}

// GetSaveSpaceTool returns the singleton SaveSpaceTool instance (lazy initialized)
func (f *Factory) GetSaveSpaceTool() (*homeclawtool.SaveSpaceTool, error) {
	if f.saveSpaceTool != nil {
		return f.saveSpaceTool, nil
	}
	store, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	f.saveSpaceTool = homeclawtool.NewSaveSpaceTool(store)
	return f.saveSpaceTool, nil
}

// GetDeleteSpaceTool returns the singleton DeleteSpaceTool instance (lazy initialized)
func (f *Factory) GetDeleteSpaceTool() (*homeclawtool.DeleteSpaceTool, error) {
	if f.deleteSpaceTool != nil {
		return f.deleteSpaceTool, nil
	}
	store, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	f.deleteSpaceTool = homeclawtool.NewDeleteSpaceTool(store)
	return f.deleteSpaceTool, nil
}

// GetListMembersTool returns the singleton ListMembersTool instance (lazy initialized)
func (f *Factory) GetListMembersTool() (*homeclawtool.ListMembersTool, error) {
	if f.listMembersTool != nil {
		return f.listMembersTool, nil
	}
	store, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}
	f.listMembersTool = homeclawtool.NewListMembersTool(store)
	return f.listMembersTool, nil
}

// GetGetMemberTool returns the singleton GetMemberTool instance (lazy initialized)
func (f *Factory) GetGetMemberTool() (*homeclawtool.GetMemberTool, error) {
	if f.getMemberTool != nil {
		return f.getMemberTool, nil
	}
	store, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}
	f.getMemberTool = homeclawtool.NewGetMemberTool(store)
	return f.getMemberTool, nil
}

// GetSaveMemberTool returns the singleton SaveMemberTool instance (lazy initialized)
func (f *Factory) GetSaveMemberTool() (*homeclawtool.SaveMemberTool, error) {
	if f.saveMemberTool != nil {
		return f.saveMemberTool, nil
	}
	store, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}
	f.saveMemberTool = homeclawtool.NewSaveMemberTool(store)
	return f.saveMemberTool, nil
}

// GetDeleteMemberTool returns the singleton DeleteMemberTool instance (lazy initialized)
func (f *Factory) GetDeleteMemberTool() (*homeclawtool.DeleteMemberTool, error) {
	if f.deleteMemberTool != nil {
		return f.deleteMemberTool, nil
	}
	store, err := f.GetMemberStore()
	if err != nil {
		return nil, err
	}
	f.deleteMemberTool = homeclawtool.NewDeleteMemberTool(store)
	return f.deleteMemberTool, nil
}

// GetListWorkflowsTool returns the singleton ListWorkflowsTool instance (lazy initialized)
func (f *Factory) GetListWorkflowsTool() (*homeclawtool.ListWorkflowsTool, error) {
	if f.listWorkflowsTool != nil {
		return f.listWorkflowsTool, nil
	}
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	f.listWorkflowsTool = homeclawtool.NewListWorkflowsTool(store)
	return f.listWorkflowsTool, nil
}

// GetGetWorkflowTool returns the singleton GetWorkflowTool instance (lazy initialized)
func (f *Factory) GetGetWorkflowTool() (*homeclawtool.GetWorkflowTool, error) {
	if f.getWorkflowTool != nil {
		return f.getWorkflowTool, nil
	}
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	f.getWorkflowTool = homeclawtool.NewGetWorkflowTool(store)
	return f.getWorkflowTool, nil
}

// GetSaveWorkflowTool returns the singleton SaveWorkflowTool instance (lazy initialized)
func (f *Factory) GetSaveWorkflowTool() (*homeclawtool.SaveWorkflowTool, error) {
	if f.saveWorkflowTool != nil {
		return f.saveWorkflowTool, nil
	}
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	f.saveWorkflowTool = homeclawtool.NewSaveWorkflowTool(store)
	return f.saveWorkflowTool, nil
}

// GetDeleteWorkflowTool returns the singleton DeleteWorkflowTool instance (lazy initialized)
func (f *Factory) GetDeleteWorkflowTool() (*homeclawtool.DeleteWorkflowTool, error) {
	if f.deleteWorkflowTool != nil {
		return f.deleteWorkflowTool, nil
	}
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	f.deleteWorkflowTool = homeclawtool.NewDeleteWorkflowTool(store)
	return f.deleteWorkflowTool, nil
}

// GetEnableWorkflowTool returns the singleton EnableWorkflowTool instance (lazy initialized)
func (f *Factory) GetEnableWorkflowTool() (*homeclawtool.EnableWorkflowTool, error) {
	if f.enableWorkflowTool != nil {
		return f.enableWorkflowTool, nil
	}
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	f.enableWorkflowTool = homeclawtool.NewEnableWorkflowTool(store)
	return f.enableWorkflowTool, nil
}

// GetDisableWorkflowTool returns the singleton DisableWorkflowTool instance (lazy initialized)
func (f *Factory) GetDisableWorkflowTool() (*homeclawtool.DisableWorkflowTool, error) {
	if f.disableWorkflowTool != nil {
		return f.disableWorkflowTool, nil
	}
	store, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}
	f.disableWorkflowTool = homeclawtool.NewDisableWorkflowTool(store)
	return f.disableWorkflowTool, nil
}

// GetGetXiaomiAccountTool returns the singleton GetXiaomiAccountTool instance (lazy initialized)
func (f *Factory) GetGetXiaomiAccountTool() (*homeclawtool.GetXiaomiAccountTool, error) {
	if f.getXiaomiAccountTool != nil {
		return f.getXiaomiAccountTool, nil
	}
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	f.getXiaomiAccountTool = homeclawtool.NewGetXiaomiAccountTool(store, oauthClient)
	return f.getXiaomiAccountTool, nil
}

// GetUpdateXiaomiHomeTool returns the singleton UpdateXiaomiHomeTool instance (lazy initialized)
func (f *Factory) GetUpdateXiaomiHomeTool() (*homeclawtool.UpdateXiaomiHomeTool, error) {
	if f.updateXiaomiHomeTool != nil {
		return f.updateXiaomiHomeTool, nil
	}
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	f.updateXiaomiHomeTool = homeclawtool.NewUpdateXiaomiHomeTool(store)
	return f.updateXiaomiHomeTool, nil
}

// GetGetXiaomiOAuthURLTool returns the singleton GetXiaomiOAuthURLTool instance (lazy initialized)
func (f *Factory) GetGetXiaomiOAuthURLTool() (*homeclawtool.GetXiaomiOAuthURLTool, error) {
	if f.getXiaomiOAuthURLTool != nil {
		return f.getXiaomiOAuthURLTool, nil
	}
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	f.getXiaomiOAuthURLTool = homeclawtool.NewGetXiaomiOAuthURLTool(oauthClient)
	return f.getXiaomiOAuthURLTool, nil
}

// GetGetXiaomiAccessTokenTool returns the singleton GetXiaomiAccessTokenTool instance (lazy initialized)
func (f *Factory) GetGetXiaomiAccessTokenTool() (*homeclawtool.GetXiaomiAccessTokenTool, error) {
	if f.getXiaomiAccessTokenTool != nil {
		return f.getXiaomiAccessTokenTool, nil
	}
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}
	f.getXiaomiAccessTokenTool = homeclawtool.NewGetXiaomiAccessTokenTool(store, oauthClient)
	return f.getXiaomiAccessTokenTool, nil
}

// GetSyncXiaomiHomesTool returns the singleton SyncXiaomiHomesTool instance (lazy initialized)
func (f *Factory) GetSyncXiaomiHomesTool() (*homeclawtool.SyncXiaomiHomesTool, error) {
	if f.syncXiaomiHomesTool != nil {
		return f.syncXiaomiHomesTool, nil
	}
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	f.syncXiaomiHomesTool = homeclawtool.NewSyncXiaomiHomesTool(store, f)
	return f.syncXiaomiHomesTool, nil
}

// GetSyncXiaomiRoomsTool returns the singleton SyncXiaomiRoomsTool instance (lazy initialized)
func (f *Factory) GetSyncXiaomiRoomsTool() (*homeclawtool.SyncXiaomiRoomsTool, error) {
	if f.syncXiaomiRoomsTool != nil {
		return f.syncXiaomiRoomsTool, nil
	}
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	spaceStore, err := f.GetSpaceStore()
	if err != nil {
		return nil, err
	}
	f.syncXiaomiRoomsTool = homeclawtool.NewSyncXiaomiRoomsTool(store, spaceStore, f)
	return f.syncXiaomiRoomsTool, nil
}

// GetSyncXiaomiDevicesTool returns the singleton SyncXiaomiDevicesTool instance (lazy initialized)
func (f *Factory) GetSyncXiaomiDevicesTool() (*homeclawtool.SyncXiaomiDevicesTool, error) {
	if f.syncXiaomiDevicesTool != nil {
		return f.syncXiaomiDevicesTool, nil
	}
	store, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}
	deviceStore, err := f.GetDeviceStore()
	if err != nil {
		return nil, err
	}
	f.syncXiaomiDevicesTool = homeclawtool.NewSyncXiaomiDevicesTool(store, deviceStore, f)
	return f.syncXiaomiDevicesTool, nil
}

// GetCloudClient returns the singleton CloudClient instance (lazy initialized)
// The CloudClient manages its own token refresh internally
func (f *Factory) GetCloudClient() (*miio.CloudClient, error) {
	if f.cloudClient != nil {
		return f.cloudClient, nil
	}

	acc, err := f.GetXiaomiAccountStore()
	if err != nil {
		return nil, err
	}

	account, err := acc.Get()
	if err != nil {
		return nil, fmt.Errorf("failed to get xiaomi account: %w", err)
	}

	oauthClient, err := f.GetMIoTOAuthClient()
	if err != nil {
		return nil, err
	}

	f.cloudClient, err = miio.NewCloudClient("cn", oauthClient.GetClientID(), account.AccessToken)
	if err != nil {
		return nil, err
	}
	return f.cloudClient, nil
}

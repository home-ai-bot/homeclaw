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
	homeclawconfig "github.com/sipeed/picoclaw/pkg/homeclaw/config"
	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/event"
	"github.com/sipeed/picoclaw/pkg/homeclaw/intent"
	"github.com/sipeed/picoclaw/pkg/homeclaw/llm"
	"github.com/sipeed/picoclaw/pkg/homeclaw/third"
	homeclawtool "github.com/sipeed/picoclaw/pkg/homeclaw/tool"
	"github.com/sipeed/picoclaw/pkg/homeclaw/workflow"
	"github.com/sipeed/picoclaw/pkg/media"
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
	Cfg       *config.Config
	bus       *bus.MessageBus
	Hcfg      *homeclawconfig.HomeclawConfig

	// Singleton instances - lazy loaded
	jsonStore      *data.JSONStore
	deviceStore    data.DeviceStore
	spaceStore     data.SpaceStore
	workflowStore  data.WorkflowStore
	homeStore      data.HomeStore
	deviceOpStore  data.DeviceOpStore
	eventCenter    *event.Center
	classifier     intent.IntentClassifier
	router         *intent.Router
	workflowEngine workflow.Engine
	toolRegistry   *tools.ToolRegistry

	// Initialization tracking
	storeOnce sync.Once
	storeErr  error

	// Tool singleton instances - lazy loaded
	listWorkflowsTool   *homeclawtool.ListWorkflowsTool
	getWorkflowTool     *homeclawtool.GetWorkflowTool
	saveWorkflowTool    *homeclawtool.SaveWorkflowTool
	deleteWorkflowTool  *homeclawtool.DeleteWorkflowTool
	enableWorkflowTool  *homeclawtool.EnableWorkflowTool
	disableWorkflowTool *homeclawtool.DisableWorkflowTool

	// LLM tool singleton - lazy loaded
	llmTool *homeclawtool.LLMTool

	// Video tool singleton - lazy loaded
	videoTool *homeclawtool.VideoTool

	// Common tool singleton - lazy loaded
	commonTool *homeclawtool.CommonTool

	// CLI tool singleton - lazy loaded
	cliTool   *homeclawtool.CLITool
	cliToolMu sync.Mutex

	// Auth store - lazy loaded
	authStore data.AuthStore
	authOnce  sync.Once
	authErr   error

	// Media store for sending images to channels
	mediaStore media.MediaStore
}

// NewFactory creates a new Factory instance.
// workspace is the data root used for all HomeClaw data files.
// Returns error when HomeClaw is disabled or homeclaw.json is absent.
func NewFactory(workspace string, picoclawCfg *config.Config, msgBus *bus.MessageBus) (*Factory, error) {
	hcfg, err := homeclawconfig.LoadHomeclawConfig()
	if err != nil {
		return nil, fmt.Errorf("homeclaw config load error: %w", err)
	}
	if hcfg == nil || !hcfg.Enabled {
		return nil, ErrDisabled
	}

	return &Factory{
		Workspace: workspace,
		Cfg:       picoclawCfg,
		bus:       msgBus,
		Hcfg:      hcfg,
	}, nil
}

// GetHomeclawConfig returns the HomeClaw configuration
func (f *Factory) GetHomeclawConfig() *homeclawconfig.HomeclawConfig {
	return f.Hcfg
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

// GetHomeStore returns the singleton HomeStore instance (lazy initialized)
func (f *Factory) GetHomeStore() (data.HomeStore, error) {
	if f.homeStore != nil {
		return f.homeStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, err
	}

	f.homeStore, err = data.NewHomeStore(store)
	if err != nil {
		return nil, fmt.Errorf("home store init failed: %w", err)
	}
	return f.homeStore, nil
}

// GetDeviceOpStore returns the singleton DeviceOpStore instance (lazy initialized)
func (f *Factory) GetDeviceOpStore() (data.DeviceOpStore, error) {
	if f.deviceOpStore != nil {
		return f.deviceOpStore, nil
	}

	store, err := f.GetJSONStore()
	if err != nil {
		return nil, err
	}

	// Get device store first
	deviceStore, err := f.GetDeviceStore()
	if err != nil {
		return nil, err
	}

	f.deviceOpStore, err = data.NewDeviceOpStore(store, deviceStore)
	if err != nil {
		return nil, fmt.Errorf("device op store init failed: %w", err)
	}
	return f.deviceOpStore, nil
}

// GetAuthStore returns the singleton AuthStore instance (lazy initialized).
// It points to workspace/auth, which stores credentials for all brands.
func (f *Factory) GetAuthStore() (data.AuthStore, error) {
	f.authOnce.Do(func() {
		// Get workspace path from config
		workspace := ""
		if f.Cfg != nil {
			workspace = f.Cfg.WorkspacePath()
		}
		if workspace == "" {
			f.authErr = fmt.Errorf("workspace path not configured")
			return
		}

		authDir := filepath.Join(workspace, "auth")
		store, err := data.NewJSONStore(authDir)
		if err != nil {
			f.authErr = fmt.Errorf("create auth json store: %w", err)
			return
		}

		f.authStore, f.authErr = data.NewAuthStore(store)
	})
	return f.authStore, f.authErr
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

// getSmallProvider creates an LLM provider for the small model.
// It resolves the model dynamically based on SmallModel config or falls back to default.
func (f *Factory) getSmallProvider() (providers.LLMProvider, string, error) {
	modelName := f.Hcfg.SmallModel
	if modelName == "" {
		// Fall back to PicoClaw's default model
		modelName = f.Cfg.Agents.Defaults.ModelName
	}

	// Use GetModelConfig to support load balancing (round-robin)
	modelCfg, err := f.Cfg.GetModelConfig(modelName)
	if err != nil {
		return nil, "", fmt.Errorf("small model %q: %w", modelName, err)
	}

	p, modelID, err := providers.CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, "", fmt.Errorf("small model %q: %w", modelName, err)
	}
	return p, modelID, nil
}

// GetSmallLLM returns an LLM struct for the small model.
// The provider is created dynamically on each call.
func (f *Factory) GetSmallLLM() (*llm.LLM, error) {
	provider, modelID, err := f.getSmallProvider()
	if err != nil {
		return nil, err
	}
	return &llm.LLM{
		Provider: provider,
		Model:    modelID,
	}, nil
}

// GetIntentClassifier returns the singleton IntentClassifier instance (lazy initialized)
func (f *Factory) GetIntentClassifier() (intent.IntentClassifier, error) {
	if f.classifier != nil {
		return f.classifier, nil
	}

	provider, modelID, err := f.getSmallProvider()
	if err != nil {
		return nil, err
	}

	f.classifier = intent.NewLLMClassifier(provider, f.Hcfg, modelID)
	return f.classifier, nil
}

// GetIntentRouter returns the singleton IntentRouter instance (lazy initialized)
func (f *Factory) GetIntentRouter() (*intent.Router, error) {
	if f.router != nil {
		return f.router, nil
	}

	if !f.Hcfg.IntentEnabled {
		return nil, fmt.Errorf("intent processing is disabled")
	}

	provider, modelID, err := f.getSmallProvider()
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

	chatHandler := &intent.ChatIntent{}

	workflowStore, err := f.GetWorkflowStore()
	if err != nil {
		return nil, err
	}

	workflowEngine := f.GetWorkflowEngine()
	deviceControlHandler := intent.NewDeviceControlIntent(workflowStore, workflowEngine, provider, modelID)
	f.router = intent.NewRouter(
		chatHandler,
		deviceControlHandler,
		intent.NewDeviceMgmtIntent(deviceStore, spaceStore),
		intent.NewSpaceMgmtIntent(spaceStore),
		&intent.SystemConfigIntent{},
	)

	return f.router, nil
}

// getBigProvider creates an LLM provider for the big model.
// It resolves the model dynamically based on BigModel config or falls back to default.
func (f *Factory) getBigProvider() (providers.LLMProvider, string, error) {
	modelName := f.Hcfg.BigModel
	if modelName == "" {
		// Fall back to PicoClaw's default model
		modelName = f.Cfg.Agents.Defaults.ModelName
	}

	// Use GetModelConfig to support load balancing (round-robin)
	modelCfg, err := f.Cfg.GetModelConfig(modelName)
	if err != nil {
		return nil, "", fmt.Errorf("big model %q: %w", modelName, err)
	}

	p, modelID, err := providers.CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, "", fmt.Errorf("big model %q: %w", modelName, err)
	}
	return p, modelID, nil
}

// GetBigLLM returns an LLM struct for the big model.
// The provider is created dynamically on each call.
func (f *Factory) GetBigLLM() (*llm.LLM, error) {
	provider, modelID, err := f.getBigProvider()
	if err != nil {
		return nil, err
	}
	return &llm.LLM{
		Provider: provider,
		Model:    modelID,
	}, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// Tool factory methods
// ─────────────────────────────────────────────────────────────────────────────

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

// ─────────────────────────────────────────────────────────────────────────────
// Intent model name accessor (implements tool.IntentProviderFactory)
// ─────────────────────────────────────────────────────────────────────────────

// GetSmallModelName returns the model name used for small/intent tasks.
func (f *Factory) GetSmallModelName() string {
	if f.Hcfg.SmallModel != "" {
		return f.Hcfg.SmallModel
	}
	// Fall back to PicoClaw's default model
	return f.Cfg.Agents.Defaults.GetModelName()
}

// ─────────────────────────────────────────────────────────────────────────────
// Video / RTSP tools
// ─────────────────────────────────────────────────────────────────────────────

// GetVideoTool returns the singleton VideoTool instance (lazy initialized).
// It provides unified video operations: capImage, capAnalyze.
// The LLM is optional — capImage works without it, capAnalyze will fail at runtime if LLM is unavailable.
func (f *Factory) GetVideoTool() (*homeclawtool.VideoTool, error) {
	if f.videoTool != nil {
		return f.videoTool, nil
	}

	// Get the small LLM instance (optional — capImage doesn't need it)
	smallLLM, _ := f.GetSmallLLM()

	f.videoTool = homeclawtool.NewVideoTool(smallLLM)
	// Inject media store if available
	if f.mediaStore != nil {
		f.videoTool.SetMediaStore(f.mediaStore)
	}
	return f.videoTool, nil
}

// SetMediaStore sets the media store for tools that need to send images to channels.
// If the VideoTool has already been created, the store is injected immediately.
func (f *Factory) SetMediaStore(store media.MediaStore) {
	f.mediaStore = store
	// Propagate to already-created VideoTool if exists
	if f.videoTool != nil {
		f.videoTool.SetMediaStore(store)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// LLM tools
// ─────────────────────────────────────────────────────────────────────────────

// GetLLMTool returns the singleton LLMTool instance (lazy initialized).
// It provides unified LLM operations: image analysis, text processing, device spec analysis.
func (f *Factory) GetLLMTool() (*homeclawtool.LLMTool, error) {
	if f.llmTool != nil {
		return f.llmTool, nil
	}

	// Get the small LLM instance
	smallLLM, err := f.GetSmallLLM()
	if err != nil {
		return nil, fmt.Errorf("failed to get small LLM for LLMTool: %w", err)
	}

	// Get device operation store
	deviceOpStore, err := f.GetDeviceOpStore()
	if err != nil {
		return nil, fmt.Errorf("failed to get device op store for LLMTool: %w", err)
	}

	// Get device store
	deviceStore, err := f.GetDeviceStore()
	if err != nil {
		return nil, fmt.Errorf("failed to get device store for LLMTool: %w", err)
	}

	f.llmTool = homeclawtool.NewLLMToolWithStores(smallLLM, f.Workspace, deviceOpStore, deviceStore)

	// Note: clients will be injected later by the caller if needed for device spec analysis
	// This avoids import cycle between pkg/homeclaw/ioc and pkg/homeclaw/third/ioc

	return f.llmTool, nil
}

// GetCommonTool returns the singleton CommonTool instance (lazy initialized).
// It provides common utility methods that are brand-agnostic.
func (f *Factory) GetCommonTool() (*homeclawtool.CommonTool, error) {
	if f.commonTool != nil {
		return f.commonTool, nil
	}

	deviceStore, err := f.GetDeviceStore()
	if err != nil {
		return nil, fmt.Errorf("failed to get device store for CommonTool: %w", err)
	}

	homeStore, err := f.GetHomeStore()
	if err != nil {
		return nil, fmt.Errorf("failed to get home store for CommonTool: %w", err)
	}

	f.commonTool = homeclawtool.NewCommonTool(deviceStore, homeStore)
	return f.commonTool, nil
}

// GetCLITool returns the singleton CLITool instance (lazy initialized).
// It creates the tool with all required stores; clients and authStore are set separately.
func (f *Factory) GetCLITool() (*homeclawtool.CLITool, error) {
	f.cliToolMu.Lock()
	defer f.cliToolMu.Unlock()

	if f.cliTool != nil {
		return f.cliTool, nil
	}

	homeStore, err := f.GetHomeStore()
	if err != nil {
		return nil, fmt.Errorf("get home store: %w", err)
	}

	spaceStore, err := f.GetSpaceStore()
	if err != nil {
		return nil, fmt.Errorf("get space store: %w", err)
	}

	deviceStore, err := f.GetDeviceStore()
	if err != nil {
		return nil, fmt.Errorf("get device store: %w", err)
	}

	deviceOpStore, err := f.GetDeviceOpStore()
	if err != nil {
		return nil, fmt.Errorf("get device op store: %w", err)
	}

	authStore, err := f.GetAuthStore()
	if err != nil {
		// Log warning but continue without authStore
		// Auth operations won't work but device control will
		authStore = nil
	}

	f.cliTool = homeclawtool.NewCLITool(homeStore, spaceStore, deviceStore, deviceOpStore, authStore)
	return f.cliTool, nil
}

// SetCLIToolClients sets the brand clients for the CLI tool.
// This should be called after GetCLITool to enable device control operations.
func (f *Factory) SetCLIToolClients(clients map[string]third.Client) {
	f.cliToolMu.Lock()
	defer f.cliToolMu.Unlock()

	if f.cliTool != nil {
		f.cliTool.SetClients(clients)
	}
}

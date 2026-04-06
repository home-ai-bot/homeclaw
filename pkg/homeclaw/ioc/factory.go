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
	eventCenter    *event.Center
	classifier     intent.IntentClassifier
	router         *intent.Router
	workflowEngine workflow.Engine
	toolRegistry   *tools.ToolRegistry

	// Provider for intent classification or other small use
	smallProvider providers.LLMProvider
	smallModel    string

	// Provider for other purposes or large use
	bigProvider providers.LLMProvider
	bigModel    string
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

// getIntentProvider returns the LLM provider for intent classification (lazy initialized)
func (f *Factory) getIntentProvider() (providers.LLMProvider, error) {
	if f.smallProvider != nil {
		return f.smallProvider, nil
	}

	mc := f.Hcfg.IntentModel

	if mc.IsModelName() {
		for i := range f.Cfg.ModelList {
			if f.Cfg.ModelList[i].ModelName == mc.ModelName {
				p, modelID, err := providers.CreateProviderFromConfig(f.Cfg.ModelList[i])
				if err != nil {
					return nil, fmt.Errorf("intent model_ref %q: %w", mc.ModelName, err)
				}
				f.smallProvider = p
				f.smallModel = modelID
				return f.smallProvider, nil
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
		APIKeys:   config.SecureStrings{config.NewSecureString(mc.APIKey)},
	}
	p, _, err := providers.CreateProviderFromConfig(modelCfg)
	if err != nil {
		return nil, fmt.Errorf("intent inline provider: %w", err)
	}
	f.smallProvider = p
	f.smallModel = mc.ModelName
	return f.smallProvider, nil
}

// GetLocalLLM returns an LLM struct wrapping the intent provider and model.
// This provides a convenient interface for simple chat operations.
func (f *Factory) GetLocalLLM() (*llm.LLM, error) {
	provider, err := f.getIntentProvider()
	if err != nil {
		return nil, err
	}
	return &llm.LLM{
		Provider: provider,
		Model:    f.GetIntentModelName(),
	}, nil
}

// GetIntentClassifier returns the singleton IntentClassifier instance (lazy initialized)
func (f *Factory) GetIntentClassifier() (intent.IntentClassifier, error) {
	if f.classifier != nil {
		return f.classifier, nil
	}

	provider, err := f.getIntentProvider()
	if err != nil {
		return nil, err
	}

	f.classifier = intent.NewLLMClassifier(provider, f.Hcfg, f.Hcfg.IntentModel.ModelName)
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

	provider, err := f.getIntentProvider()
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
	deviceControlHandler := intent.NewDeviceControlIntent(workflowStore, workflowEngine, provider, f.Hcfg.IntentModel.ModelName)
	f.router = intent.NewRouter(
		chatHandler,
		deviceControlHandler,
		intent.NewDeviceMgmtIntent(deviceStore, spaceStore),
		intent.NewSpaceMgmtIntent(spaceStore),
		&intent.SystemConfigIntent{},
	)

	return f.router, nil
}

func (f *Factory) getBigProvider() (providers.LLMProvider, error) {
	if f.bigProvider != nil {
		return f.bigProvider, nil
	}
	defaultModelName := f.Cfg.Agents.Defaults.ModelName

	for i := range f.Cfg.ModelList {
		if f.Cfg.ModelList[i].ModelName == defaultModelName {
			p, modelID, err := providers.CreateProviderFromConfig(f.Cfg.ModelList[i])
			if err != nil {
				return nil, fmt.Errorf("big model create err %q: %w", defaultModelName, err)
			}
			f.bigProvider = p
			f.bigModel = modelID
			return f.bigProvider, nil
		}
	}
	return nil, fmt.Errorf(" %q not found in model_list", defaultModelName)
}

// GetBigLLM returns an LLM struct wrapping the big provider and model.
// This provides a convenient interface for simple chat operations.
func (f *Factory) GetBigLLM() (*llm.LLM, error) {
	provider, err := f.getBigProvider()
	if err != nil {
		return nil, err
	}
	return &llm.LLM{
		Provider: provider,
		Model:    f.bigModel,
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

// GetIntentModelName returns the model name used by the intent classifier.
// It triggers lazy initialization of the provider if not yet done.
func (f *Factory) GetIntentModelName() string {
	if f.smallModel != "" {
		return f.smallModel
	}
	// Trigger provider init to populate f.modelName
	_, _ = f.getIntentProvider()
	return f.smallModel
}

// ─────────────────────────────────────────────────────────────────────────────
// Video / RTSP tools
// ─────────────────────────────────────────────────────────────────────────────

// GetVideoTool returns the singleton VideoTool instance (lazy initialized).
// It provides unified video operations: capImage, capAnalyze.
func (f *Factory) GetVideoTool() (*homeclawtool.VideoTool, error) {
	if f.videoTool != nil {
		return f.videoTool, nil
	}

	// Get the local LLM instance
	localLLM, err := f.GetLocalLLM()
	if err != nil {
		return nil, fmt.Errorf("failed to get local LLM for VideoTool: %w", err)
	}

	f.videoTool = homeclawtool.NewVideoTool(localLLM)
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
// It provides unified LLM operations: image analysis, text processing.
func (f *Factory) GetLLMTool() (*homeclawtool.LLMTool, error) {
	if f.llmTool != nil {
		return f.llmTool, nil
	}

	// Get the local LLM instance
	localLLM, err := f.GetLocalLLM()
	if err != nil {
		return nil, fmt.Errorf("failed to get local LLM for LLMTool: %w", err)
	}

	f.llmTool = homeclawtool.NewLLMTool(localLLM)
	return f.llmTool, nil
}

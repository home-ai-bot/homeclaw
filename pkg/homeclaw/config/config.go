// Package config provides HomeClaw-specific configuration, loaded independently
// from PicoClaw's main config.json to avoid upstream coupling.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/AlexxIT/go2rtc/pkg/yaml"
)

const defaultConfigFileName = "homeclaw.json"
const defaultGo2RtcFileName = "go2rtc.yaml"

// DefaultConfidenceThreshold is the default intent confidence threshold.
const DefaultConfidenceThreshold = 0.7

// HomeclawConfig is the top-level HomeClaw configuration.
// It is stored in a standalone homeclaw.json file and loaded independently
// from PicoClaw's config.json so that upstream changes to PicoClaw do not
// affect HomeClaw configuration.
type HomeclawConfig struct {
	// Enabled controls whether HomeClaw intent processing is active.
	// When false (or homeclaw.json is absent), handleIntent is a no-op.
	Enabled bool `json:"enabled"`

	// IntentEnabled controls whether the intent classification and dispatching
	// logic (RunIntent) should be executed. When false, RunIntent will skip
	// processing and return immediately, falling through to the large model.
	IntentEnabled bool `json:"intent_enabled"`

	// ConfidenceThreshold is the minimum intent confidence score required to
	// dispatch to an Intent handler. Inputs scoring below this value fall through
	// to the large-model agent loop. Default: 0.7.
	ConfidenceThreshold float64 `json:"confidence_threshold"`

	// IntentModel specifies the small model used for intent classification and
	// workflow matching.
	IntentModel IntentModelConfig `json:"intent_model"`

	Rtsp string `json:"rtsp"`
}

// IntentModelConfig holds connection details for the small intent-classification model.
// Two usage modes are supported (mutually exclusive):
//
//  1. ModelRef – reference the model_name of a model already declared in PicoClaw's
//     model_list; the AgentLoop will resolve the full config from there.
//  2. Inline (APIBase + APIKey + Model) – fully self-contained; does not require
//     any entry in PicoClaw's model_list.
type IntentModelConfig struct {
	// ModelName references a model_name entry in PicoClaw's model_list.
	// When set, APIBase / APIKey / Model below are ignored.
	ModelName string `json:"model_name,omitempty"`

	// APIBase is the OpenAI-compatible API endpoint, e.g. "http://localhost:11434/v1".
	APIBase string `json:"api_base,omitempty"`

	// APIKey is the API authentication key.
	APIKey string `json:"api_key,omitempty"`

	// Model is the protocol-prefixed model identifier, e.g. "openai/qwen2.5:1.5b".
	Model string `json:"model,omitempty"`
}

// IsModelRef returns true when the config uses a PicoClaw model_list reference.
func (m IntentModelConfig) IsModelName() bool {
	return m.ModelName != ""
}

// Validate checks that the IntentModelConfig has sufficient information to
// build a provider.
func (m IntentModelConfig) Validate() error {
	if m.ModelName != "" {
		return nil
	}
	if m.Model == "" {
		return fmt.Errorf("intent_model: either model_ref or model must be set")
	}
	return nil
}

// applyDefaults fills in zero-value fields with their defaults.
func (c *HomeclawConfig) applyDefaults() {
	if c.ConfidenceThreshold <= 0 {
		c.ConfidenceThreshold = DefaultConfidenceThreshold
	}
}

// Load reads and parses a homeclaw.json file from the given path.
func load(path string) (*HomeclawConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("homeclaw config: read %s: %w", path, err)
	}

	var cfg HomeclawConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("homeclaw config: parse %s: %w", path, err)
	}

	cfg.applyDefaults()
	return &cfg, nil
}

// LoadFromDir looks for homeclaw.json inside dir and loads it.
// Returns (nil, nil) when the file does not exist, so callers can treat
// a missing file as "HomeClaw not configured" without an error.
func LoadHomeclawConfig() (*HomeclawConfig, error) {
	path := filepath.Join(GetPicoclawHome(), defaultConfigFileName)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("homeclaw config: stat %s: %w", path, err)
	}
	return load(path)
}

var configMu sync.Mutex

func LoadYamlConfig(filepath string, v any) error {
	if filepath == "" {
		return errors.New("config file path is empty")
	}

	b, err := os.ReadFile(filepath)
	if err != nil {
		return err
	}

	return yaml.Unmarshal(b, v)
}
func PatchConfig(filepath string, path []string, value any) error {
	if filepath == "" {
		return errors.New("config file disabled")
	}

	configMu.Lock()
	defer configMu.Unlock()

	// empty config is OK
	b, _ := os.ReadFile(filepath)

	b, err := yaml.Patch(b, path, value)
	if err != nil {
		return err
	}

	return os.WriteFile(filepath, b, 0644)
}
func GetGo2RTCPath() string {
	return filepath.Join(GetPicoclawHome(), defaultGo2RtcFileName)
}
func LoadGo2RTCConfig(v any) error {
	return LoadYamlConfig(GetGo2RTCPath(), v)
}
func PatchGo2RTCConfig(path []string, value any) error {
	return PatchConfig(GetGo2RTCPath(), path, value)
}

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

	// SmallModel specifies the model_name (from PicoClaw's model_list) used for
	// intent classification and other small tasks. If empty, falls back to the
	// default model from PicoClaw config.
	SmallModel string `json:"small_model,omitempty"`

	// BigModel specifies the model_name (from PicoClaw's model_list) used for
	// complex tasks. If empty, falls back to the default model from PicoClaw config.
	BigModel string `json:"big_model,omitempty"`
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
// If the file does not exist, it creates a default config and saves it.
func LoadHomeclawConfig() (*HomeclawConfig, error) {
	path := filepath.Join(GetPicoclawHome(), defaultConfigFileName)
	_, err := os.Stat(path)
	if os.IsNotExist(err) {
		// Create default config and save it
		cfg := DefaultHomeclawConfig()
		if err := SaveHomeclawConfig(path, cfg); err != nil {
			return nil, fmt.Errorf("homeclaw config: create default %s: %w", path, err)
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("homeclaw config: stat %s: %w", path, err)
	}
	return load(path)
}

// DefaultHomeclawConfig returns a default HomeclawConfig with sensible defaults.
func DefaultHomeclawConfig() *HomeclawConfig {
	return &HomeclawConfig{
		Enabled:             true,
		IntentEnabled:       false,
		ConfidenceThreshold: DefaultConfidenceThreshold,
		SmallModel:          "",
		BigModel:            "",
	}
}

// SaveHomeclawConfig saves the HomeclawConfig to the specified path.
func SaveHomeclawConfig(path string, cfg *HomeclawConfig) error {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}

	return nil
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

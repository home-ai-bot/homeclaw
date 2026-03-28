package util

import (
	"encoding/json"
	"fmt"

	"github.com/sipeed/picoclaw/pkg/homeclaw/config"
)

// Action represents a device action with method and parameters
type Action struct {
	Method string                 `json:"method"`
	Param  map[string]interface{} `json:"param"`
}

// ValidMethods contains the allowed method values
var ValidMethods = map[string]bool{
	"Action":  true,
	"GetProp": true,
	"SetProp": true,
}

// ValidJsonResult contains the parsed actions and any validation errors
type ValidJsonResult struct {
	ActionErrors  []string
	MethondErrors []string
}

// ValidJson parses a JSON string containing action definitions and validates
// that each action name is a valid ActionType and each method is valid.
// Returns the parsed result and an error if JSON parsing fails.
func ValidJson(jsonStr string) (*ValidJsonResult, error) {
	var actions []map[string]Action
	if err := json.Unmarshal([]byte(jsonStr), &actions); err != nil {
		return nil, fmt.Errorf("JSON parse failed: %w", err)
	}

	result := &ValidJsonResult{
		ActionErrors:  []string{},
		MethondErrors: []string{},
	}

	// Validate each action name and method
	for _, group := range actions {
		for name, action := range group {
			// Validate action name against ActionType enum
			actionType := config.ActionType(name)
			if _, exists := config.ActionTypeNames[actionType]; !exists {
				result.ActionErrors = append(result.ActionErrors, name)
			}
			// Validate method
			if !ValidMethods[action.Method] {
				result.MethondErrors = append(result.MethondErrors, fmt.Sprintf("%s: %s", name, action.Method))
			}
		}
	}

	return result, nil
}

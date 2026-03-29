package util

import (
	"encoding/json"
	"fmt"
	"strings"
)

// MIoTSpec represents the full MIoT specification document
type MIoTSpec struct {
	Type        string        `json:"type"`
	Description string        `json:"description"`
	Services    []MIoTService `json:"services"`
}

// MIoTService represents a service in the MIoT spec
type MIoTService struct {
	IID         int            `json:"iid"`
	Type        string         `json:"type"`
	Description string         `json:"description"`
	Properties  []MIoTProperty `json:"properties,omitempty"`
	Actions     []MIoTAction   `json:"actions,omitempty"`
	Events      []MIoTEvent    `json:"events,omitempty"`
}

// MIoTProperty represents a property in a MIoT service
type MIoTProperty struct {
	IID         int        `json:"iid"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	Format      string     `json:"format,omitempty"`
	Access      []string   `json:"access,omitempty"`
	Unit        string     `json:"unit,omitempty"`
	ValueList   []ValueDef `json:"value-list,omitempty"`
	ValueRange  []any      `json:"value-range,omitempty"`
}

// MIoTAction represents an action in a MIoT service
type MIoTAction struct {
	IID         int    `json:"iid"`
	Type        string `json:"type"`
	Description string `json:"description"`
	In          []any  `json:"in,omitempty"`
	Out         []any  `json:"out,omitempty"`
}

// ActionParam represents input/output parameter for an action
type ActionParam struct {
	PIID int `json:"piid"`
}

// MIoTEvent represents an event in a MIoT service
type MIoTEvent struct {
	IID         int    `json:"iid"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Arguments   []int  `json:"arguments,omitempty"`
}

// ValueDef represents a value definition in value-list
type ValueDef struct {
	Value       any    `json:"value"`
	Description string `json:"description"`
}

// SimplifiedSpec represents a simplified version of MIoT spec
// containing only important services, properties, and actions
type SimplifiedSpec struct {
	Type        string              `json:"type"`
	Description string              `json:"description"`
	Services    []SimplifiedService `json:"services"`
}

// SimplifiedService represents a simplified service
type SimplifiedService struct {
	IID         int                  `json:"iid"`
	Type        string               `json:"type"`
	Description string               `json:"description"`
	Properties  []SimplifiedProperty `json:"properties,omitempty"`
	Actions     []SimplifiedAction   `json:"actions,omitempty"`
}

// SimplifiedProperty represents a simplified property
type SimplifiedProperty struct {
	IID         int        `json:"iid"`
	Type        string     `json:"type"`
	Description string     `json:"description"`
	Format      string     `json:"format,omitempty"`
	Access      []string   `json:"access,omitempty"`
	Unit        string     `json:"unit,omitempty"`
	ValueList   []ValueDef `json:"value-list,omitempty"`
	ValueRange  []any      `json:"value-range,omitempty"`
}

// SimplifiedAction represents a simplified action
type SimplifiedAction struct {
	IID         int    `json:"iid"`
	Type        string `json:"type"`
	Description string `json:"description"`
	In          []any  `json:"in,omitempty"`
	Out         []any  `json:"out,omitempty"`
}

// ParseOptions configures how the spec should be parsed and filtered
type ParseOptions struct {
	// SkipDeviceInfo skips the device-information service (siid=1)
	SkipDeviceInfo bool
	// SkipBattery skips battery service
	SkipBattery bool
	// KeepOnlyReadWriteProps only keeps properties with read/write/notify access
	KeepOnlyReadWriteProps bool
	// KeepOnlyControllableServices keeps only services that have writable properties or actions
	KeepOnlyControllableServices bool
	// ExtractTypeShortName extracts short type name from URN
	ExtractTypeShortName bool
}

// DefaultParseOptions returns default parsing options
func DefaultParseOptions() *ParseOptions {
	return &ParseOptions{
		SkipDeviceInfo:               true,
		SkipBattery:                  false,
		KeepOnlyReadWriteProps:       false,
		KeepOnlyControllableServices: false,
		ExtractTypeShortName:         true,
	}
}

// SpecParser parses and simplifies MIoT spec JSON
type SpecParser struct {
	options *ParseOptions
}

// NewSpecParser creates a new SpecParser with options
func NewSpecParser(opts *ParseOptions) *SpecParser {
	if opts == nil {
		opts = DefaultParseOptions()
	}
	return &SpecParser{options: opts}
}

// Parse parses the raw spec JSON and returns a simplified version
func (p *SpecParser) Parse(specJSON string) (*SimplifiedSpec, error) {
	var spec MIoTSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil, fmt.Errorf("failed to parse spec JSON: %w", err)
	}

	simplified := &SimplifiedSpec{
		Type:        spec.Type,
		Description: spec.Description,
		Services:    make([]SimplifiedService, 0),
	}

	if p.options.ExtractTypeShortName {
		simplified.Type = extractShortType(spec.Type)
	}

	for _, svc := range spec.Services {
		// Skip device-information service if configured
		if p.options.SkipDeviceInfo && isDeviceInfoService(svc.Type) {
			continue
		}

		// Skip battery service if configured
		if p.options.SkipBattery && isBatteryService(svc.Type) {
			continue
		}

		simplifiedSvc := p.simplifyService(svc)

		// Skip services with no properties and no actions if configured
		if p.options.KeepOnlyControllableServices {
			if len(simplifiedSvc.Properties) == 0 && len(simplifiedSvc.Actions) == 0 {
				continue
			}
		}

		simplified.Services = append(simplified.Services, simplifiedSvc)
	}

	return simplified, nil
}

// ParseRaw parses the raw spec JSON and returns the full MIoT spec
func (p *SpecParser) ParseRaw(specJSON string) (*MIoTSpec, error) {
	var spec MIoTSpec
	if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
		return nil, fmt.Errorf("failed to parse spec JSON: %w", err)
	}
	return &spec, nil
}

// simplifyService converts a MIoTService to SimplifiedService
func (p *SpecParser) simplifyService(svc MIoTService) SimplifiedService {
	simplified := SimplifiedService{
		IID:         svc.IID,
		Type:        svc.Type,
		Description: svc.Description,
		Properties:  make([]SimplifiedProperty, 0),
		Actions:     make([]SimplifiedAction, 0),
	}

	if p.options.ExtractTypeShortName {
		simplified.Type = extractShortType(svc.Type)
	}

	// Process properties
	for _, prop := range svc.Properties {
		if p.options.KeepOnlyReadWriteProps && !hasReadWriteAccess(prop.Access) {
			continue
		}

		simplifiedProp := SimplifiedProperty{
			IID:         prop.IID,
			Type:        prop.Type,
			Description: prop.Description,
			Format:      prop.Format,
			Access:      prop.Access,
			Unit:        prop.Unit,
			ValueList:   prop.ValueList,
			ValueRange:  prop.ValueRange,
		}

		if p.options.ExtractTypeShortName {
			simplifiedProp.Type = extractShortType(prop.Type)
		}

		simplified.Properties = append(simplified.Properties, simplifiedProp)
	}

	// Process actions
	for _, action := range svc.Actions {
		simplifiedAction := SimplifiedAction{
			IID:         action.IID,
			Type:        action.Type,
			Description: action.Description,
			In:          action.In,
			Out:         action.Out,
		}

		if p.options.ExtractTypeShortName {
			simplifiedAction.Type = extractShortType(action.Type)
		}

		simplified.Actions = append(simplified.Actions, simplifiedAction)
	}

	return simplified
}

// ToJSON converts SimplifiedSpec to JSON string
func (s *SimplifiedSpec) ToJSON() (string, error) {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal spec: %w", err)
	}
	return string(data), nil
}

// ToCompactJSON converts SimplifiedSpec to compact JSON string
func (s *SimplifiedSpec) ToCompactJSON() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", fmt.Errorf("failed to marshal spec: %w", err)
	}
	return string(data), nil
}

// GetControllableProps returns all properties that can be controlled (write access)
func (s *SimplifiedSpec) GetControllableProps() []SimplifiedProperty {
	var props []SimplifiedProperty
	for _, svc := range s.Services {
		for _, prop := range svc.Properties {
			if hasWriteAccess(prop.Access) {
				props = append(props, prop)
			}
		}
	}
	return props
}

// GetReadableProps returns all properties that can be read
func (s *SimplifiedSpec) GetReadableProps() []SimplifiedProperty {
	var props []SimplifiedProperty
	for _, svc := range s.Services {
		for _, prop := range svc.Properties {
			if hasReadAccess(prop.Access) {
				props = append(props, prop)
			}
		}
	}
	return props
}

// GetAllActions returns all actions from all services
func (s *SimplifiedSpec) GetAllActions() []SimplifiedAction {
	var actions []SimplifiedAction
	for _, svc := range s.Services {
		actions = append(actions, svc.Actions...)
	}
	return actions
}

// FindServiceByType finds a service by its type (short name or full URN)
func (s *SimplifiedSpec) FindServiceByType(typeStr string) *SimplifiedService {
	typeStr = strings.ToLower(typeStr)
	for i, svc := range s.Services {
		if strings.Contains(strings.ToLower(svc.Type), typeStr) {
			return &s.Services[i]
		}
	}
	return nil
}

// FindPropertyByType finds a property by its type across all services
func (s *SimplifiedSpec) FindPropertyByType(typeStr string) (*SimplifiedService, *SimplifiedProperty) {
	typeStr = strings.ToLower(typeStr)
	for i, svc := range s.Services {
		for j, prop := range svc.Properties {
			if strings.Contains(strings.ToLower(prop.Type), typeStr) {
				return &s.Services[i], &svc.Properties[j]
			}
		}
	}
	return nil, nil
}

// Helper functions

// isDeviceInfoService checks if a service is the device-information service
func isDeviceInfoService(svcType string) bool {
	return strings.Contains(svcType, "device-information")
}

// isBatteryService checks if a service is the battery service
func isBatteryService(svcType string) bool {
	return strings.Contains(svcType, ":battery:")
}

// hasReadWriteAccess checks if a property has read or write access
func hasReadWriteAccess(access []string) bool {
	for _, a := range access {
		if a == "read" || a == "write" || a == "notify" {
			return true
		}
	}
	return false
}

// hasWriteAccess checks if a property has write access
func hasWriteAccess(access []string) bool {
	for _, a := range access {
		if a == "write" {
			return true
		}
	}
	return false
}

// hasReadAccess checks if a property has read access
func hasReadAccess(access []string) bool {
	for _, a := range access {
		if a == "read" {
			return true
		}
	}
	return false
}

// extractShortType extracts the short type name from a MIoT URN
// Example: "urn:miot-spec-v2:service:magnet-sensor:00007827:isa-dw2hl:1" -> "magnet-sensor"
func extractShortType(urn string) string {
	parts := strings.Split(urn, ":")
	if len(parts) >= 4 {
		return parts[3]
	}
	return urn
}

// ParseSpecJSON is a convenience function to parse spec JSON with default options
func ParseSpecJSON(specJSON string) (*SimplifiedSpec, error) {
	parser := NewSpecParser(nil)
	return parser.Parse(specJSON)
}

// ParseSpecJSONWithOptions parses spec JSON with custom options
func ParseSpecJSONWithOptions(specJSON string, opts *ParseOptions) (*SimplifiedSpec, error) {
	parser := NewSpecParser(opts)
	return parser.Parse(specJSON)
}

// ExtractImportantServices extracts only the important services from a spec
// This filters out device-information and keeps only services with meaningful properties/actions
func ExtractImportantServices(specJSON string) (*SimplifiedSpec, error) {
	opts := &ParseOptions{
		SkipDeviceInfo:               true,
		SkipBattery:                  false,
		KeepOnlyReadWriteProps:       true,
		KeepOnlyControllableServices: false,
		ExtractTypeShortName:         true,
	}
	return ParseSpecJSONWithOptions(specJSON, opts)
}

// SpecSummary provides a quick summary of a spec
type SpecSummary struct {
	DeviceType        string   `json:"device_type"`
	Description       string   `json:"description"`
	ServiceCount      int      `json:"service_count"`
	PropertyCount     int      `json:"property_count"`
	ActionCount       int      `json:"action_count"`
	ControllableProps int      `json:"controllable_props"`
	ServiceTypes      []string `json:"service_types"`
}

// GetSummary returns a summary of the simplified spec
func (s *SimplifiedSpec) GetSummary() *SpecSummary {
	summary := &SpecSummary{
		DeviceType:   s.Type,
		Description:  s.Description,
		ServiceCount: len(s.Services),
		ServiceTypes: make([]string, 0),
	}

	for _, svc := range s.Services {
		summary.ServiceTypes = append(summary.ServiceTypes, svc.Type)
		summary.PropertyCount += len(svc.Properties)
		summary.ActionCount += len(svc.Actions)
		for _, prop := range svc.Properties {
			if hasWriteAccess(prop.Access) {
				summary.ControllableProps++
			}
		}
	}

	return summary
}

// DeviceCommand represents a command that can be sent to a device
type DeviceCommand struct {
	Desc      string `json:"desc"`
	Method    string `json:"method"`
	Param     any    `json:"param"`
	ParamDesc string `json:"param_desc"`
}

// SetPropParam represents parameters for SetProp command
type SetPropParam struct {
	DID   string `json:"did"`
	SIID  int    `json:"siid"`
	PIID  int    `json:"piid"`
	Value string `json:"value"`
}

// ActionCommandParam represents parameters for Action command
type ActionCommandParam struct {
	DID  string `json:"did"`
	SIID int    `json:"siid"`
	AIID int    `json:"aiid"`
	In   []any  `json:"in"`
}

// CommandParam is kept for backward compatibility
type CommandParam struct {
	DID   string `json:"did"`
	SIID  int    `json:"siid"`
	PIID  int    `json:"piid,omitempty"`
	AIID  int    `json:"aiid,omitempty"`
	Value string `json:"value,omitempty"`
	In    []any  `json:"in,omitempty"`
}

// invalidServiceTypes defines service types to be filtered out
var invalidServiceTypes = []string{
	"device-information",
	"battery",
	"identify",
	"indicator-light",
	"physical-controls-locked",
}

// isInvalidService checks if a service should be filtered out
func isInvalidService(svcType string) bool {
	svcTypeLower := strings.ToLower(svcType)
	for _, invalid := range invalidServiceTypes {
		if strings.Contains(svcTypeLower, invalid) {
			return true
		}
	}
	return false
}

// GenerateDeviceCommands generates a list of device commands from the spec
// It keeps only writable properties and all actions, filtering out invalid services
func (s *SimplifiedSpec) GenerateDeviceCommands(did string) []DeviceCommand {
	var commands []DeviceCommand

	for _, svc := range s.Services {
		// Skip invalid services
		if isInvalidService(svc.Type) {
			continue
		}

		// Generate SetProp commands for writable properties
		for _, prop := range svc.Properties {
			if hasWriteAccess(prop.Access) {
				paramDesc := prop.Format
				if len(prop.ValueList) > 0 {
					// Build value list description
					var valueDescs []string
					for _, v := range prop.ValueList {
						valueDescs = append(valueDescs, fmt.Sprintf("%v:%s", v.Value, v.Description))
					}
					paramDesc = strings.Join(valueDescs, ",")
				} else if len(prop.ValueRange) >= 2 {
					// Build value range description
					paramDesc = fmt.Sprintf("%v-%v", prop.ValueRange[0], prop.ValueRange[1])
					if len(prop.ValueRange) >= 3 {
						paramDesc += fmt.Sprintf(" step:%v", prop.ValueRange[2])
					}
					if prop.Unit != "" {
						paramDesc += " " + prop.Unit
					}
				}

				cmd := DeviceCommand{
					Desc:   fmt.Sprintf("%s-%s", svc.Description, prop.Description),
					Method: "SetProp",
					Param: SetPropParam{
						DID:   did,
						SIID:  svc.IID,
						PIID:  prop.IID,
						Value: "$value$",
					},
					ParamDesc: paramDesc,
				}
				commands = append(commands, cmd)
			}
		}

		// Generate Action commands for all actions
		for _, action := range svc.Actions {
			paramDesc := ""
			if len(action.In) > 0 {
				var inParams []string
				for _, p := range action.In {
					switch v := p.(type) {
					case float64:
						// Integer piid
						inParams = append(inParams, fmt.Sprintf("piid:%d", int(v)))
					case map[string]any:
						// Object with piid
						if piid, ok := v["piid"]; ok {
							inParams = append(inParams, fmt.Sprintf("piid:%v", piid))
						}
					}
				}
				paramDesc = strings.Join(inParams, ",")
			}

			// Ensure In is always set (never nil) for proper JSON serialization
			inParams := action.In
			if inParams == nil {
				inParams = []any{}
			}

			cmd := DeviceCommand{
				Desc:   fmt.Sprintf("%s-%s", svc.Description, action.Description),
				Method: "Action",
				Param: ActionCommandParam{
					DID:  did,
					SIID: svc.IID,
					AIID: action.IID,
					In:   inParams,
				},
				ParamDesc: paramDesc,
			}
			commands = append(commands, cmd)
		}
	}

	return commands
}

// GenerateDeviceCommandsJSON generates device commands as JSON string
func (s *SimplifiedSpec) GenerateDeviceCommandsJSON(did string) (string, error) {
	commands := s.GenerateDeviceCommands(did)
	data, err := json.MarshalIndent(commands, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to marshal commands: %w", err)
	}
	return string(data), nil
}

// GenerateDeviceCommandsCompactJSON generates device commands as compact JSON string
func (s *SimplifiedSpec) GenerateDeviceCommandsCompactJSON(did string) (string, error) {
	commands := s.GenerateDeviceCommands(did)
	data, err := json.Marshal(commands)
	if err != nil {
		return "", fmt.Errorf("failed to marshal commands: %w", err)
	}
	return string(data), nil
}

// ExtractDeviceCommands is a convenience function to extract device commands from spec JSON
func ExtractDeviceCommands(specJSON string, did string) ([]DeviceCommand, error) {
	spec, err := ParseSpecJSON(specJSON)
	if err != nil {
		return nil, err
	}
	return spec.GenerateDeviceCommands(did), nil
}

// ExtractDeviceCommandsJSON is a convenience function to extract device commands as JSON
func ExtractDeviceCommandsJSON(specJSON string, did string) (string, error) {
	spec, err := ParseSpecJSON(specJSON)
	if err != nil {
		return "", err
	}
	return spec.GenerateDeviceCommandsJSON(did)
}

package util

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testMagnetSensorSpec = `{
    "type": "urn:miot-spec-v2:device:magnet-sensor:0000A016:isa-dw2hl:1",
    "description": "Magnet Sensor",
    "services": [
        {
            "iid": 1,
            "type": "urn:miot-spec-v2:service:device-information:00007801:isa-dw2hl:1",
            "description": "Device Information",
            "properties": [
                {
                    "iid": 1,
                    "type": "urn:miot-spec-v2:property:manufacturer:00000001:isa-dw2hl:1",
                    "description": "Device Manufacturer",
                    "format": "string",
                    "access": ["read"]
                },
                {
                    "iid": 2,
                    "type": "urn:miot-spec-v2:property:model:00000002:isa-dw2hl:1",
                    "description": "Device Model",
                    "format": "string",
                    "access": ["read"]
                },
                {
                    "iid": 3,
                    "type": "urn:miot-spec-v2:property:serial-number:00000003:isa-dw2hl:1",
                    "description": "Device Serial Number",
                    "format": "string",
                    "access": ["read"]
                },
                {
                    "iid": 4,
                    "type": "urn:miot-spec-v2:property:firmware-revision:00000005:isa-dw2hl:1",
                    "description": "Current Firmware Version",
                    "format": "string",
                    "access": ["read"]
                }
            ]
        },
        {
            "iid": 2,
            "type": "urn:miot-spec-v2:service:magnet-sensor:00007827:isa-dw2hl:1",
            "description": "Magnet Sensor",
            "properties": [
                {
                    "iid": 1,
                    "type": "urn:miot-spec-v2:property:illumination:0000004E:isa-dw2hl:1",
                    "description": "Illumination",
                    "format": "uint8",
                    "access": ["read", "notify"],
                    "value-list": [
                        {"value": 1, "description": "Weak"},
                        {"value": 2, "description": "Strong"}
                    ]
                },
                {
                    "iid": 2,
                    "type": "urn:miot-spec-v2:property:contact-state:0000007C:isa-dw2hl:1",
                    "description": "Contact State",
                    "format": "bool",
                    "access": ["read", "notify"]
                }
            ]
        },
        {
            "iid": 3,
            "type": "urn:miot-spec-v2:service:battery:00007805:isa-dw2hl:1",
            "description": "Battery",
            "properties": [
                {
                    "iid": 1,
                    "type": "urn:miot-spec-v2:property:battery-level:00000014:isa-dw2hl:1",
                    "description": "Battery Level",
                    "format": "uint8",
                    "access": ["read", "notify"],
                    "unit": "percentage",
                    "value-range": [0, 100, 1]
                }
            ]
        }
    ]
}`

func TestParseSpecJSON(t *testing.T) {
	spec, err := ParseSpecJSON(testMagnetSensorSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	// Should skip device-information service
	if len(spec.Services) != 2 {
		t.Errorf("Expected 2 services (without device-info), got %d", len(spec.Services))
	}

	// Check device type is extracted
	if spec.Type != "magnet-sensor" {
		t.Errorf("Expected type 'magnet-sensor', got '%s'", spec.Type)
	}

	// Check description
	if spec.Description != "Magnet Sensor" {
		t.Errorf("Expected description 'Magnet Sensor', got '%s'", spec.Description)
	}
}

func TestParseSpecJSONWithOptions_SkipBattery(t *testing.T) {
	opts := &ParseOptions{
		SkipDeviceInfo:       true,
		SkipBattery:          true,
		ExtractTypeShortName: true,
	}

	spec, err := ParseSpecJSONWithOptions(testMagnetSensorSpec, opts)
	if err != nil {
		t.Fatalf("ParseSpecJSONWithOptions failed: %v", err)
	}

	// Should only have magnet-sensor service
	if len(spec.Services) != 1 {
		t.Errorf("Expected 1 service (without device-info and battery), got %d", len(spec.Services))
	}

	if spec.Services[0].Type != "magnet-sensor" {
		t.Errorf("Expected service type 'magnet-sensor', got '%s'", spec.Services[0].Type)
	}
}

func TestParseSpecJSONWithOptions_KeepAll(t *testing.T) {
	opts := &ParseOptions{
		SkipDeviceInfo:       false,
		SkipBattery:          false,
		ExtractTypeShortName: false,
	}

	spec, err := ParseSpecJSONWithOptions(testMagnetSensorSpec, opts)
	if err != nil {
		t.Fatalf("ParseSpecJSONWithOptions failed: %v", err)
	}

	// Should have all 3 services
	if len(spec.Services) != 3 {
		t.Errorf("Expected 3 services, got %d", len(spec.Services))
	}

	// Check full URN is preserved
	if spec.Type != "urn:miot-spec-v2:device:magnet-sensor:0000A016:isa-dw2hl:1" {
		t.Errorf("Expected full URN type, got '%s'", spec.Type)
	}
}

func TestSimplifiedSpec_FindServiceByType(t *testing.T) {
	spec, err := ParseSpecJSON(testMagnetSensorSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	// Find magnet-sensor service
	svc := spec.FindServiceByType("magnet-sensor")
	if svc == nil {
		t.Fatal("Expected to find magnet-sensor service")
	}

	if svc.Description != "Magnet Sensor" {
		t.Errorf("Expected description 'Magnet Sensor', got '%s'", svc.Description)
	}

	// Find non-existent service
	notFound := spec.FindServiceByType("non-existent")
	if notFound != nil {
		t.Error("Expected nil for non-existent service")
	}
}

func TestSimplifiedSpec_FindPropertyByType(t *testing.T) {
	spec, err := ParseSpecJSON(testMagnetSensorSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	// Find contact-state property
	svc, prop := spec.FindPropertyByType("contact-state")
	if svc == nil || prop == nil {
		t.Fatal("Expected to find contact-state property")
	}

	if prop.Description != "Contact State" {
		t.Errorf("Expected description 'Contact State', got '%s'", prop.Description)
	}

	if prop.Format != "bool" {
		t.Errorf("Expected format 'bool', got '%s'", prop.Format)
	}
}

func TestSimplifiedSpec_GetReadableProps(t *testing.T) {
	spec, err := ParseSpecJSON(testMagnetSensorSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	readableProps := spec.GetReadableProps()
	// magnet-sensor has 2 props (illumination, contact-state)
	// battery has 1 prop (battery-level)
	if len(readableProps) != 3 {
		t.Errorf("Expected 3 readable properties, got %d", len(readableProps))
	}
}

func TestSimplifiedSpec_GetSummary(t *testing.T) {
	spec, err := ParseSpecJSON(testMagnetSensorSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	summary := spec.GetSummary()

	if summary.DeviceType != "magnet-sensor" {
		t.Errorf("Expected device type 'magnet-sensor', got '%s'", summary.DeviceType)
	}

	if summary.ServiceCount != 2 {
		t.Errorf("Expected 2 services, got %d", summary.ServiceCount)
	}

	if summary.PropertyCount != 3 {
		t.Errorf("Expected 3 properties, got %d", summary.PropertyCount)
	}

	if summary.ActionCount != 0 {
		t.Errorf("Expected 0 actions, got %d", summary.ActionCount)
	}
}

func TestSimplifiedSpec_ToJSON(t *testing.T) {
	spec, err := ParseSpecJSON(testMagnetSensorSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	jsonStr, err := spec.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Verify it's valid JSON
	var parsed SimplifiedSpec
	if err := json.Unmarshal([]byte(jsonStr), &parsed); err != nil {
		t.Fatalf("ToJSON produced invalid JSON: %v", err)
	}

	if parsed.Type != spec.Type {
		t.Errorf("Expected type '%s', got '%s'", spec.Type, parsed.Type)
	}
}

func TestExtractImportantServices(t *testing.T) {
	spec, err := ExtractImportantServices(testMagnetSensorSpec)
	if err != nil {
		t.Fatalf("ExtractImportantServices failed: %v", err)
	}

	// Should skip device-information
	if len(spec.Services) != 2 {
		t.Errorf("Expected 2 services, got %d", len(spec.Services))
	}

	// All properties should have read/write/notify access
	for _, svc := range spec.Services {
		for _, prop := range svc.Properties {
			if !hasReadWriteAccess(prop.Access) {
				t.Errorf("Property %s should have read/write/notify access", prop.Type)
			}
		}
	}
}

func TestExtractShortType(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"urn:miot-spec-v2:device:magnet-sensor:0000A016:isa-dw2hl:1", "magnet-sensor"},
		{"urn:miot-spec-v2:service:battery:00007805:isa-dw2hl:1", "battery"},
		{"urn:miot-spec-v2:property:contact-state:0000007C:isa-dw2hl:1", "contact-state"},
		{"short-type", "short-type"},
		{"", ""},
	}

	for _, tt := range tests {
		result := extractShortType(tt.input)
		if result != tt.expected {
			t.Errorf("extractShortType(%s) = %s, expected %s", tt.input, result, tt.expected)
		}
	}
}

const testLightSpec = `{
    "type": "urn:miot-spec-v2:device:light:0000A001:yeelink-lamp9:1",
    "description": "Light",
    "services": [
        {
            "iid": 1,
            "type": "urn:miot-spec-v2:service:device-information:00007801:yeelink-lamp9:1",
            "description": "Device Information",
            "properties": [
                {"iid": 1, "type": "urn:miot-spec-v2:property:manufacturer:00000001:yeelink-lamp9:1", "description": "Device Manufacturer", "format": "string", "access": ["read"]}
            ]
        },
        {
            "iid": 2,
            "type": "urn:miot-spec-v2:service:light:00007802:yeelink-lamp9:1",
            "description": "Light",
            "properties": [
                {"iid": 1, "type": "urn:miot-spec-v2:property:on:00000006:yeelink-lamp9:1", "description": "Switch Status", "format": "bool", "access": ["read", "write", "notify"]},
                {"iid": 2, "type": "urn:miot-spec-v2:property:brightness:0000000D:yeelink-lamp9:1", "description": "Brightness", "format": "uint8", "access": ["read", "write", "notify"], "value-range": [1, 100, 1]},
                {"iid": 3, "type": "urn:miot-spec-v2:property:color-temperature:0000000F:yeelink-lamp9:1", "description": "Color Temperature", "format": "uint32", "access": ["read", "write", "notify"], "unit": "kelvin", "value-range": [2700, 6500, 1]}
            ],
            "actions": [
                {"iid": 1, "type": "urn:miot-spec-v2:action:toggle:00002811:yeelink-lamp9:1", "description": "Toggle"}
            ]
        }
    ]
}`

func TestParseSpecWithActions(t *testing.T) {
	spec, err := ParseSpecJSON(testLightSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	// Should skip device-information service
	if len(spec.Services) != 1 {
		t.Errorf("Expected 1 service, got %d", len(spec.Services))
	}

	lightSvc := spec.Services[0]
	if lightSvc.Type != "light" {
		t.Errorf("Expected service type 'light', got '%s'", lightSvc.Type)
	}

	// Check properties
	if len(lightSvc.Properties) != 3 {
		t.Errorf("Expected 3 properties, got %d", len(lightSvc.Properties))
	}

	// Check actions
	if len(lightSvc.Actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(lightSvc.Actions))
	}

	if lightSvc.Actions[0].Type != "toggle" {
		t.Errorf("Expected action type 'toggle', got '%s'", lightSvc.Actions[0].Type)
	}
}

func TestSimplifiedSpec_GetControllableProps(t *testing.T) {
	spec, err := ParseSpecJSON(testLightSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	controllable := spec.GetControllableProps()
	// Light has 3 writable properties (on, brightness, color-temperature)
	if len(controllable) != 3 {
		t.Errorf("Expected 3 controllable properties, got %d", len(controllable))
	}
}

func TestSimplifiedSpec_GetAllActions(t *testing.T) {
	spec, err := ParseSpecJSON(testLightSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	actions := spec.GetAllActions()
	if len(actions) != 1 {
		t.Errorf("Expected 1 action, got %d", len(actions))
	}

	if actions[0].Description != "Toggle" {
		t.Errorf("Expected action description 'Toggle', got '%s'", actions[0].Description)
	}
}

const testVacuumSpec = `{
    "type": "urn:miot-spec-v2:device:vacuum:0000A006:roborock-m1s:2",
    "description": "Robot Vacuum",
    "services": [
        {
            "iid": 2,
            "type": "urn:miot-spec-v2:service:vacuum:00007810:roborock-m1s:2",
            "description": "Robot Cleaner",
            "properties": [
                {"iid": 1, "type": "urn:miot-spec-v2:property:mode:00000008:roborock-m1s:1", "description": "Mode", "format": "uint8", "access": ["read", "write", "notify"], "value-list": [{"value": 1, "description": "Silent"}, {"value": 2, "description": "Basic"}]}
            ],
            "actions": [
                {"iid": 1, "type": "urn:miot-spec-v2:action:start-sweep:00002804:roborock-m1s:1", "description": "Start Sweep", "in": [], "out": []},
                {"iid": 3, "type": "urn:miot-spec-v2:action:start-room-sweep:00002826:roborock-m1s:1", "description": "Start Room Sweep", "in": [2], "out": []}
            ]
        }
    ]
}`

func TestGenerateDeviceCommandsWithIn(t *testing.T) {
	spec, err := ParseSpecJSON(testVacuumSpec)
	if err != nil {
		t.Fatalf("ParseSpecJSON failed: %v", err)
	}

	// Check actions have In preserved
	if len(spec.Services) != 1 {
		t.Fatalf("Expected 1 service, got %d", len(spec.Services))
	}

	actions := spec.Services[0].Actions
	if len(actions) != 2 {
		t.Fatalf("Expected 2 actions, got %d", len(actions))
	}

	// First action should have empty in
	t.Logf("Action 0 In: %v (type: %T)", actions[0].In, actions[0].In)
	t.Logf("Action 1 In: %v (type: %T)", actions[1].In, actions[1].In)

	// Generate commands
	commands := spec.GenerateDeviceCommands("test-did")

	// Find the action commands
	jsonData, err := json.MarshalIndent(commands, "", "  ")
	if err != nil {
		t.Fatalf("Failed to marshal commands: %v", err)
	}
	t.Logf("Commands JSON:\n%s", string(jsonData))

	// Verify the "Start Room Sweep" action has in: [2]
	for _, cmd := range commands {
		if cmd.Method == "Action" && strings.Contains(cmd.Desc, "Start Room Sweep") {
			actionParam, ok := cmd.Param.(ActionCommandParam)
			if !ok {
				t.Errorf("Expected ActionCommandParam type, got %T", cmd.Param)
				continue
			}
			if len(actionParam.In) != 1 {
				t.Errorf("Expected In to have 1 element, got %d", len(actionParam.In))
			}
		}
	}
}

// TestProcessSpecFilesFromDirectory reads all JSON files from a directory,
// processes them using the spec parser, and saves simplified results to new files
func TestProcessSpecFilesFromDirectory(t *testing.T) {
	specDir := `D:\green\claw\.picoclaw\workspace\spec`

	// Check if directory exists
	if _, err := os.Stat(specDir); os.IsNotExist(err) {
		t.Skipf("Spec directory does not exist: %s", specDir)
		return
	}

	// Read all JSON files from the directory
	entries, err := os.ReadDir(specDir)
	if err != nil {
		t.Fatalf("Failed to read spec directory: %v", err)
	}

	processedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		fileName := entry.Name()
		// Skip non-JSON files and already processed files
		if !strings.HasSuffix(strings.ToLower(fileName), ".json") {
			continue
		}
		if strings.HasSuffix(fileName, "_new.json") {
			continue
		}

		inputPath := filepath.Join(specDir, fileName)
		outputFileName := strings.TrimSuffix(fileName, ".json") + "_new.json"
		outputPath := filepath.Join(specDir, outputFileName)

		// Read the spec JSON file
		specData, err := os.ReadFile(inputPath)
		if err != nil {
			t.Errorf("Failed to read file %s: %v", fileName, err)
			continue
		}

		// Parse and generate device commands
		spec, err := ParseSpecJSON(string(specData))
		if err != nil {
			t.Errorf("Failed to parse spec %s: %v", fileName, err)
			continue
		}

		// Generate device commands (using empty did as placeholder)
		commands := spec.GenerateDeviceCommands("")

		// Convert to JSON
		outputData, err := json.MarshalIndent(commands, "", "  ")
		if err != nil {
			t.Errorf("Failed to marshal commands for %s: %v", fileName, err)
			continue
		}

		// Write to new file
		err = os.WriteFile(outputPath, outputData, 0644)
		if err != nil {
			t.Errorf("Failed to write output file %s: %v", outputFileName, err)
			continue
		}

		t.Logf("Processed %s -> %s (%d commands)", fileName, outputFileName, len(commands))
		processedCount++
	}

	t.Logf("Total processed: %d files", processedCount)
}

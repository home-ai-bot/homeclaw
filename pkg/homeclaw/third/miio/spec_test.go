package miio

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestSpecFetcherGetSpec(t *testing.T) {
	// Create temp dir for testing
	tempDir := t.TempDir()
	fetcher := NewSpecFetcher(tempDir)

	jsonData := `{"type":"urn:miot-spec-v2:device:magnet-sensor:0000A016:isa-dw2hl:1","description":"Magnet Sensor","services":[{"iid":1,"type":"urn:miot-spec-v2:service:device-information:00007801:isa-dw2hl:1","description":"Device Information","properties":[{"iid":1,"type":"urn:miot-spec-v2:property:manufacturer:00000001:isa-dw2hl:1","description":"Device Manufacturer","format":"string","access":["read"]},{"iid":2,"type":"urn:miot-spec-v2:property:model:00000002:isa-dw2hl:1","description":"Device Model","format":"string","access":["read"]},{"iid":3,"type":"urn:miot-spec-v2:property:serial-number:00000003:isa-dw2hl:1","description":"Device Serial Number","format":"string","access":["read"]},{"iid":4,"type":"urn:miot-spec-v2:property:firmware-revision:00000005:isa-dw2hl:1","description":"Current Firmware Version","format":"string","access":["read"]}]},{"iid":2,"type":"urn:miot-spec-v2:service:magnet-sensor:00007827:isa-dw2hl:1","description":"Magnet Sensor","properties":[{"iid":1,"type":"urn:miot-spec-v2:property:illumination:0000004E:isa-dw2hl:1","description":"Illumination","format":"uint8","access":["read","notify"],"value-list":[{"value":1,"description":"Weak"},{"value":2,"description":"Strong"}]},{"iid":2,"type":"urn:miot-spec-v2:property:contact-state:0000007C:isa-dw2hl:1","description":"Contact State","format":"bool","access":["read","notify"]}]},{"iid":3,"type":"urn:miot-spec-v2:service:battery:00007805:isa-dw2hl:1","description":"Battery","properties":[{"iid":1,"type":"urn:miot-spec-v2:property:battery-level:00000014:isa-dw2hl:1","description":"Battery Level","format":"uint8","access":["read","notify"],"unit":"percentage","value-range":[0,100,1]}]}]}`

	// Test empty URN
	if _, err := fetcher.GetSpec(""); err == nil {
		t.Error("Expected error for empty URN")
	}

	// Manually save JSON to cache to test loading
	cacheDir := filepath.Join(tempDir, SpecCacheDir)
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("Failed to create cache dir: %v", err)
	}

	urn := "urn:miot-spec-v2:device:magnet-sensor:0000A016:isa-dw2hl:1"
	filename := fetcher.getCacheFilename(urn)
	cacheFile := filepath.Join(cacheDir, filename)
	if err := os.WriteFile(cacheFile, []byte(jsonData), 0644); err != nil {
		t.Fatalf("Failed to write cache file: %v", err)
	}

	// Test loading from local cache
	data, err := fetcher.GetSpec(urn)
	if err != nil {
		t.Fatalf("GetSpec failed: %v", err)
	}

	// Verify data is valid JSON
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(data), &result); err != nil {
		t.Fatalf("Returned data is not valid JSON: %v", err)
	}

	// Verify content
	if result["type"] != urn {
		t.Errorf("Expected type %s, got %v", urn, result["type"])
	}
	if result["description"] != "Magnet Sensor" {
		t.Errorf("Expected description 'Magnet Sensor', got %v", result["description"])
	}

	services, ok := result["services"].([]interface{})
	if !ok || len(services) != 3 {
		t.Errorf("Expected 3 services, got %v", services)
	}

	// Test memory cache hit (second call should return same data from memory)
	data2, err := fetcher.GetSpec(urn)
	if err != nil {
		t.Fatalf("GetSpec second call failed: %v", err)
	}
	if string(data) != string(data2) {
		t.Error("Memory cache returned different data")
	}
}

// Package tool provides HomeClaw LLM tools for Mi Home (miio) device management.
// This file contains four tools:
//
//  1. hc_mijia_login    – Log in to Mi Home cloud via username+password (no 2FA),
//     fetch device list + tokens, persist devices into the device store.
//  2. hc_mijia_set_token – Inject a manually obtained serviceToken + userId + ssecurity
//     to connect to the Mi Home cloud when 2FA prevents automatic login.
//  3. hc_miio_get_props  – Read one or more MIoT properties from a local device.
//  4. hc_miio_set_props  – Set one or more MIoT properties on a local device.
package tool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/sipeed/picoclaw/pkg/homeclaw/data"
	"github.com/sipeed/picoclaw/pkg/homeclaw/miio"
	"github.com/sipeed/picoclaw/pkg/tools"
)

// ─────────────────────────────────────────────────────────────────────────────
// hc_mijia_login
// ─────────────────────────────────────────────────────────────────────────────

// MijiaLoginTool logs in to the Mi Home cloud, retrieves the device list with
// per-device tokens, and saves each device into the local HomeClaw device store.
// When the account has 2FA enabled, the tool returns clear instructions for the
// user to manually provide the credentials via hc_mijia_set_token instead.
type MijiaLoginTool struct {
	store data.DeviceStore
}

// NewMijiaLoginTool creates a MijiaLoginTool backed by the given DeviceStore.
func NewMijiaLoginTool(store data.DeviceStore) *MijiaLoginTool {
	return &MijiaLoginTool{store: store}
}

func (t *MijiaLoginTool) Name() string { return "hc_mijia_login" }

func (t *MijiaLoginTool) Description() string {
	return "Log in to the Mi Home (Xiaomi) cloud account using username and password, " +
		"fetch the full device list with local IP addresses and miio tokens, then save " +
		"all discovered devices into the HomeClaw device store.\n\n" +
		"If the account has 2FA/identity-verification enabled, this tool will return " +
		"instructions asking the user to manually provide their session credentials " +
		"via the hc_mijia_set_token tool instead."
}

func (t *MijiaLoginTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"username": map[string]any{
				"type":        "string",
				"description": "Mi Home account username (email or phone number)",
			},
			"password": map[string]any{
				"type":        "string",
				"description": "Mi Home account password",
			},
			"country": map[string]any{
				"type":        "string",
				"description": "Server region: cn (default), us, de, sg, ru, tw, i2",
				"enum":        []string{"cn", "us", "de", "sg", "ru", "tw", "i2"},
			},
		},
		"required": []string{"username", "password"},
	}
}

func (t *MijiaLoginTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	username, _ := args["username"].(string)
	password, _ := args["password"].(string)
	country, _ := args["country"].(string)

	if username == "" || password == "" {
		return &tools.ToolResult{ForLLM: "username and password are required", IsError: true}
	}
	if country == "" {
		country = "cn"
	}

	client := miio.NewCloudClient(country)
	res, err := client.LoginWithResult(username, password)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("Mi Home login failed: %v", err), IsError: true}
	}

	// ── 2FA required ──────────────────────────────────────────────────────────
	if res.NeedVerify {
		return &tools.ToolResult{
			ForLLM: fmt.Sprintf(
				"Login requires 2FA verification. Please open the following URL on your phone to approve:\n%s\n\n"+
					"After approving, try logging in again.", res.NotifyURL),
			IsError: true,
		}
	}

	return t.importDevices(client)
}

// importDevices fetches the cloud device list and saves devices into the store.
func (t *MijiaLoginTool) importDevices(client *miio.CloudClient) *tools.ToolResult {
	cloudDevices, err := client.GetDevices()
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("failed to fetch device list: %v", err), IsError: true}
	}

	now := time.Now()
	saved := 0
	skipped := 0
	var summary []map[string]string

	for _, cd := range cloudDevices {
		if cd.Token == "" {
			skipped++
			continue
		}
		device := data.Device{
			ID:           cd.Did,
			Name:         cd.Name,
			Brand:        "mijia",
			Protocol:     "miio",
			Model:        cd.Model,
			IP:           cd.IP,
			Token:        cd.Token,
			Capabilities: []string{},
			State:        map[string]interface{}{},
			AddedAt:      now,
			LastSeen:     now,
		}
		if err := t.store.Save(device); err != nil {
			skipped++
			continue
		}
		saved++
		summary = append(summary, map[string]string{
			"id":    cd.Did,
			"name":  cd.Name,
			"model": cd.Model,
			"ip":    cd.IP,
		})
	}

	result := map[string]any{
		"saved":   saved,
		"skipped": skipped,
		"devices": summary,
	}
	b, _ := json.Marshal(result)
	return tools.NewToolResult(string(b))
}

// MiioGetPropsTool reads MIoT properties from a device on the local network.
type MiioGetPropsTool struct {
	store data.DeviceStore
}

// NewMiioGetPropsTool creates a MiioGetPropsTool.
func NewMiioGetPropsTool(store data.DeviceStore) *MiioGetPropsTool {
	return &MiioGetPropsTool{store: store}
}

func (t *MiioGetPropsTool) Name() string { return "hc_miio_get_props" }

func (t *MiioGetPropsTool) Description() string {
	return "Read one or more MIoT properties from a Xiaomi miio device on the local " +
		"network. Looks up the device IP and token from the HomeClaw device store. " +
		"Returns the property values returned by the device."
}

func (t *MiioGetPropsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"device_id": map[string]any{
				"type":        "string",
				"description": "HomeClaw device ID (did) of the target device",
			},
			"props": map[string]any{
				"type":        "array",
				"description": "List of MIoT property descriptors to read",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"siid": map[string]any{
							"type":        "integer",
							"description": "Service instance ID",
						},
						"piid": map[string]any{
							"type":        "integer",
							"description": "Property instance ID",
						},
					},
					"required": []string{"siid", "piid"},
				},
			},
		},
		"required": []string{"device_id", "props"},
	}
}

func (t *MiioGetPropsTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	deviceID, _ := args["device_id"].(string)
	if deviceID == "" {
		return &tools.ToolResult{ForLLM: "device_id is required", IsError: true}
	}

	propsRaw, ok := args["props"]
	if !ok {
		return &tools.ToolResult{ForLLM: "props is required", IsError: true}
	}
	propsSlice, ok := propsRaw.([]any)
	if !ok || len(propsSlice) == 0 {
		return &tools.ToolResult{ForLLM: "props must be a non-empty array", IsError: true}
	}

	device, err := t.store.GetByID(deviceID)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("device %q not found: %v", deviceID, err), IsError: true}
	}
	if device.IP == "" || device.Token == "" {
		return &tools.ToolResult{ForLLM: "device has no IP or token; run hc_mijia_login first", IsError: true}
	}

	// Build property list with did.
	props, buildErr := buildPropList(propsSlice, deviceID)
	if buildErr != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("invalid props: %v", buildErr), IsError: true}
	}

	cli, err := miio.NewClient(device.IP, device.Token)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("miio client error: %v", err), IsError: true}
	}

	result, err := cli.GetProperties(props)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("miio get_properties failed: %v", err), IsError: true}
	}
	return tools.NewToolResult(string(result))
}

// ─────────────────────────────────────────────────────────────────────────────
// hc_miio_set_props
// ─────────────────────────────────────────────────────────────────────────────

// MiioSetPropsTool sets MIoT properties on a device on the local network.
type MiioSetPropsTool struct {
	store data.DeviceStore
}

// NewMiioSetPropsTool creates a MiioSetPropsTool.
func NewMiioSetPropsTool(store data.DeviceStore) *MiioSetPropsTool {
	return &MiioSetPropsTool{store: store}
}

func (t *MiioSetPropsTool) Name() string { return "hc_miio_set_props" }

func (t *MiioSetPropsTool) Description() string {
	return "Set one or more MIoT properties on a Xiaomi miio device on the local " +
		"network (e.g. turn on/off, set brightness, change color temperature). " +
		"Looks up the device IP and token from the HomeClaw device store."
}

func (t *MiioSetPropsTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"device_id": map[string]any{
				"type":        "string",
				"description": "HomeClaw device ID (did) of the target device",
			},
			"props": map[string]any{
				"type":        "array",
				"description": "List of MIoT property descriptors with values to set",
				"items": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"siid": map[string]any{
							"type":        "integer",
							"description": "Service instance ID",
						},
						"piid": map[string]any{
							"type":        "integer",
							"description": "Property instance ID",
						},
						"value": map[string]any{
							"description": "Value to set for this property",
						},
					},
					"required": []string{"siid", "piid", "value"},
				},
			},
		},
		"required": []string{"device_id", "props"},
	}
}

func (t *MiioSetPropsTool) Execute(_ context.Context, args map[string]any) *tools.ToolResult {
	deviceID, _ := args["device_id"].(string)
	if deviceID == "" {
		return &tools.ToolResult{ForLLM: "device_id is required", IsError: true}
	}

	propsRaw, ok := args["props"]
	if !ok {
		return &tools.ToolResult{ForLLM: "props is required", IsError: true}
	}
	propsSlice, ok := propsRaw.([]any)
	if !ok || len(propsSlice) == 0 {
		return &tools.ToolResult{ForLLM: "props must be a non-empty array", IsError: true}
	}

	device, err := t.store.GetByID(deviceID)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("device %q not found: %v", deviceID, err), IsError: true}
	}
	if device.IP == "" || device.Token == "" {
		return &tools.ToolResult{ForLLM: "device has no IP or token; run hc_mijia_login first", IsError: true}
	}

	// Build property list including 'value' fields.
	props, buildErr := buildSetPropList(propsSlice, deviceID)
	if buildErr != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("invalid props: %v", buildErr), IsError: true}
	}

	cli, err := miio.NewClient(device.IP, device.Token)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("miio client error: %v", err), IsError: true}
	}

	result, err := cli.SetProperties(props)
	if err != nil {
		return &tools.ToolResult{ForLLM: fmt.Sprintf("miio set_properties failed: %v", err), IsError: true}
	}

	// Reflect the change into local device state as best-effort.
	stateUpdate := map[string]interface{}{}
	for _, p := range props {
		key := fmt.Sprintf("%v_%v", p["siid"], p["piid"])
		stateUpdate[key] = p["value"]
	}
	_ = t.store.UpdateState(deviceID, stateUpdate)

	return tools.NewToolResult(string(result))
}

// ─────────────────────────────────────────────────────────────────────────────
// helpers
// ─────────────────────────────────────────────────────────────────────────────

// buildPropList converts a raw []any props list into miio get_properties format.
func buildPropList(raw []any, did string) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("prop[%d] is not an object", i)
		}
		siid, err := toInt(m["siid"])
		if err != nil {
			return nil, fmt.Errorf("prop[%d].siid: %w", i, err)
		}
		piid, err := toInt(m["piid"])
		if err != nil {
			return nil, fmt.Errorf("prop[%d].piid: %w", i, err)
		}
		result = append(result, map[string]any{
			"did":  did,
			"siid": siid,
			"piid": piid,
		})
	}
	return result, nil
}

// buildSetPropList converts a raw []any props list into miio set_properties format.
func buildSetPropList(raw []any, did string) ([]map[string]any, error) {
	result := make([]map[string]any, 0, len(raw))
	for i, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("prop[%d] is not an object", i)
		}
		siid, err := toInt(m["siid"])
		if err != nil {
			return nil, fmt.Errorf("prop[%d].siid: %w", i, err)
		}
		piid, err := toInt(m["piid"])
		if err != nil {
			return nil, fmt.Errorf("prop[%d].piid: %w", i, err)
		}
		value, hasValue := m["value"]
		if !hasValue {
			return nil, fmt.Errorf("prop[%d].value is required", i)
		}
		result = append(result, map[string]any{
			"did":   did,
			"siid":  siid,
			"piid":  piid,
			"value": value,
		})
	}
	return result, nil
}

// toInt converts a JSON number (float64) or integer to int.
func toInt(v any) (int, error) {
	switch n := v.(type) {
	case float64:
		return int(n), nil
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case json.Number:
		i, err := n.Int64()
		return int(i), err
	case nil:
		return 0, fmt.Errorf("value is nil")
	default:
		return 0, fmt.Errorf("expected number, got %T", v)
	}
}

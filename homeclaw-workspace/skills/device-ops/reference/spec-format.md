# Brand-Specific Specification Formats

This reference document describes the format differences between smart home brands (Xiaomi MIoT and Tuya) for device specifications.

---

## Xiaomi MIoT Spec

### API Call

```
hc_cli
- commandJson: {"brand":"xiaomi","method":"getSpec","params":{"deviceId":"<from_id>"}}
```

### Response Format

```json
[
  {"desc": "Light Control-Switch Status", "method": "SetProp", "param": {"did": "", "siid": 2, "piid": 1, "value": "$value$"}, "param_desc": "bool"},
  {"desc": "Light Control-Brightness", "method": "SetProp", "param": {"did": "", "siid": 2, "piid": 2, "value": "$value$"}, "param_desc": "int 0-100"},
  {"desc": "Light Control-Get Status", "method": "GetProp", "param": {"did": "", "siid": 2, "piid": 1}, "param_desc": ""}
]
```

### Key Fields

| Field | Type | Description |
|-------|------|-------------|
| `desc` | string | Human-readable description of the property/action |
| `method` | string | One of `SetProp`, `GetProp`, or `Action` |
| `param.siid` | integer | Service ID |
| `param.piid` | integer | Property ID (for SetProp/GetProp) |
| `param.aiid` | integer | Action ID (for Action) |
| `param.value` | string | Value template, contains `$value$` placeholder for settable properties |
| `param_desc` | string | Value type description (e.g., "bool", "int 0-100") |

### Method Types

**SetProp**: Set a device property
- Requires: `siid`, `piid`, `value`
- Example: Turn on light (`value: true`)

**GetProp**: Read a device property
- Requires: `siid`, `piid`
- Example: Get light status

**Action**: Trigger a device action
- Requires: `siid`, `aiid`, `in` (input parameters array)
- Example: Start vacuum cleaning

---

## Tuya Thing Model

### API Call

```
hc_cli
- commandJson: {"brand":"tuya","method":"getSpec","params":{"deviceId":"<from_id>"}}
```

### Response Format

```json
{
  "services": [
    {
      "code": "switch",
      "name": "Switch",
      "properties": [
        {"code": "switch", "type": "Boolean"},
        {"code": "fan_speed", "type": "Integer", "range": "1-4"}
      ]
    }
  ]
}
```

### Key Fields

| Field | Type | Description |
|-------|------|-------------|
| `services[].code` | string | Service code identifier |
| `services[].name` | string | Human-readable service name |
| `services[].properties[].code` | string | Property code (used in commands) |
| `services[].properties[].type` | string | Data type (Boolean, Integer, Enum, etc.) |
| `services[].properties[].range` | string | Value range for numeric types (e.g., "1-4") |

### Property Types

- **Boolean**: true/false values (e.g., switch on/off)
- **Integer**: Numeric values with optional range (e.g., brightness 0-100)
- **Enum**: Predefined value options (e.g., mode: ["cold", "heat", "auto"])
- **String**: Text values (less common)

---

## Comparison

| Aspect | Xiaomi MIoT | Tuya |
|--------|-------------|------|
| **Structure** | Flat array of specs | Nested services with properties |
| **Identification** | siid/piid/aiid integers | Property code strings |
| **Method Types** | SetProp, GetProp, Action | Implicit (set or get based on usage) |
| **Value Template** | `$value$` in param | `$value$` in command |
| **Complexity** | More explicit, hierarchical | Simpler, flat structure |

---

## Usage Tips

1. **Xiaomi**: Always extract `siid` and `piid`/`aiid` from the spec before building commands
2. **Tuya**: Use property `code` directly in commands
3. **Both**: Use `$value$` placeholder for operations that need dynamic values
4. **Both**: Check `param_desc` (Xiaomi) or `type`/`range` (Tuya) for value constraints

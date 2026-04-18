# Tuya Parsing Rules

This document contains the parsing rules for Tuya device specifications.

## Spec Structure

Tuya specs are returned as a nested structure with services and properties:

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

## Parsing Rules

### 1. Navigate Service Structure

Iterate through `services` array, then through each service's `properties` array.

### 2. Identify Property Types

Check the `type` field to determine value handling:
- `Boolean` → true/false values (switches, states)
- `Integer` → Numeric values with optional range (brightness, speed)
- `Enum` → Predefined options (modes, states)
- `String` → Text values (rare)

### 3. Extract Property Codes

Use the `code` field directly in commands:
- Property code becomes the key in the command JSON
- Example: `{"device_id": "xxx", "switch": true}`

### 4. Parse Property Codes for Operation Mapping

Map property codes to standard operations:

#### Switch/Power Operations
- Property codes: "switch", "power", "on_off"
- Map to: `turn_on`/`turn_off` (set), `get_state` (get)
- Value: `true` for turn_on, `false` for turn_off

#### Brightness Operations
- Property codes: "bright_value", "brightness", "bright"
- Map to: `set_brightness` (set), `get_brightness` (get)
- Check `range` field for valid values (e.g., "0-100")

#### Color Temperature Operations
- Property codes: "temp_value", "colour_temp", "color_temp"
- Map to: `set_color_temp` (set), `get_color_temp` (get)

#### Color/RGB Operations
- Property codes: "colour_data", "color_data", "rgb"
- Map to: `set_rgb_color` (set), `get_rgb_color` (get)

#### Fan Speed Operations
- Property codes: "fan_speed", "fan_speed_enum", "level"
- Map to: `set_percentage` (set), `get_percentage` (get)
- Check `range` field for valid values

#### Temperature Operations
- Property codes: "temp_set", "temperature", "temp"
- Map to: `set_temperature` (set), `get_temperature` (get)

#### Mode Operations
- Property codes: "mode", "work_mode", "operation_mode"
- Map to: `set_hvac_mode`, `set_preset_mode` (context dependent)
- Check `range` or enum values for valid options

### 5. Build Command Structure

For each property, generate an operation object with three fields: `method`, `ops`, and `param`:

#### For Set Property:
```json
{
  "method": "setProps",
  "ops": "<operation_name>",
  "param": {
    "device_id": "<device_from_id>",
    "<property_code>": <concrete_value_or_$value$>
  }
}
```

#### For Get Property:
```json
{
  "method": "getProps",
  "ops": "<operation_name>",
  "param": {
    "device_id": "<device_from_id>"
  }
}
```

**Note**: Tuya reads all properties at once, so get operations only need device_id.

### 6. Value Handling

- **Boolean properties**: Use concrete values `true`/`false`
- **Integer properties**: Use `"$value$"` placeholder (check range if provided)
- **Enum properties**: Use `"$value$"` placeholder or first enum value for examples
- **String properties**: Use `"$value$"` placeholder with quotes

### 7. Range Parsing

If `range` field exists (e.g., "1-4", "0-100"):
- Parse min and max values
- Use for validation when generating operations
- Include in operation metadata if needed

## Example Parsing

Input spec:
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

Parsed operations for device "fan789":
```json
[
  {
    "method": "setProps",
    "ops": "turn_on",
    "param": {"device_id": "fan789", "switch": true}
  },
  {
    "method": "setProps",
    "ops": "turn_off",
    "param": {"device_id": "fan789", "switch": false}
  },
  {
    "method": "getProps",
    "ops": "get_state",
    "param": {"device_id": "fan789"}
  },
  {
    "method": "setProps",
    "ops": "set_percentage",
    "param": {"device_id": "fan789", "fan_speed": "$value$"}
  }
]
```

## Service Code Patterns

Common service codes and their typical properties:

- `switch`: switch, power states
- `light`: brightness, color, color_temp
- `fan`: fan_speed, mode, oscillate
- `climate`: temperature, mode, humidity
- `cover`: position, control (open/close/stop)
- `vacuum`: power, mode, status

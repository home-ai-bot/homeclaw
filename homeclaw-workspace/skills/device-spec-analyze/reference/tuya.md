# Tuya Parsing Rules

This document contains the parsing rules for Tuya device specifications.

## Spec Structure

Tuya specs are Thing Models with a nested structure containing services and properties:

```json
{
  "modelId": "e1n49084",
  "services": [
    {
      "code": "",
      "name": "",
      "description": "",
      "properties": [
        {
          "abilityId": 1,
          "code": "switch_1",
          "name": "开关1",
          "description": "",
          "accessMode": "rw",
          "typeSpec": {
            "type": "bool"
          }
        },
        {
          "abilityId": 9,
          "code": "countdown_1",
          "name": "开关1倒计时",
          "description": "",
          "accessMode": "rw",
          "typeSpec": {
            "type": "value",
            "max": 86400,
            "step": 1,
            "unit": "s"
          }
        }
      ]
    }
  ]
}
```

## Parsing Approach

Tuya device analysis uses **LLM-based property-by-property analysis** (similar to Xiaomi):

1. Parse the Thing Model JSON structure
2. Extract all properties from all services
3. For **each property**, send its information to the LLM individually
4. LLM returns matching operations with values and methods
5. Build final operations array locally from LLM responses

## Property Analysis

For each property, the LLM receives:
- **code**: Property code (e.g., "switch_1", "countdown_1")
- **name**: Property name (e.g., "开关1", "开关1倒计时")
- **description**: Property description
- **type**: Property type from typeSpec (bool, value, enum, string)
- **access_mode**: Access mode (ro=read-only, wr=write-only, rw=read-write)
- **Supported Operations Reference** (ops.md)

## LLM Output Format

For each property, LLM returns a JSON array:
```json
[
  {"ops": "turn_on", "value": true, "method": "setProps"},
  {"ops": "turn_off", "value": false, "method": "setProps"},
  {"ops": "get_state", "value": null, "method": "getProps"}
]
```

## Operation Generation Rules

### Based on Access Mode:
- **ro** (read-only): Only generate get operations (method: "getProps")
- **wr** (write-only): Only generate set operations (method: "setProps")
- **rw** (read-write): Generate both get and set operations

### Based on Property Type:
- **bool**: Use concrete values `true`/`false` for set operations
- **value/integer**: Use `"$value$"` placeholder for set operations
- **enum**: Use `"$value$"` placeholder for set operations
- **string**: Use `"$value$"` placeholder for set operations

### Command Structure:

#### For Set Property (setProps):
```json
{
  "method": "setProps",
  "ops": "<operation_name>",
  "param": {
    "device_id": "<device_from_id>",
    "<property_code>": <value>
  }
}
```

#### For Get Property (getProps):
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

## Example Parsing

Input Thing Model for device "socket123":
```json
{
  "modelId": "e1n49084",
  "services": [
    {
      "code": "",
      "name": "",
      "properties": [
        {"code": "switch_1", "name": "开关1", "accessMode": "rw", "typeSpec": {"type": "bool"}},
        {"code": "child_lock", "name": "童锁开关", "accessMode": "rw", "typeSpec": {"type": "bool"}}
      ]
    }
  ]
}
```

Processing flow:
1. Parse model, extract 2 properties
2. For property "switch_1" (bool, rw):
   - LLM returns: `[{"ops": "turn_on", "value": true, "method": "setProps"}, {"ops": "turn_off", "value": false, "method": "setProps"}, {"ops": "get_state", "value": null, "method": "getProps"}]`
3. For property "child_lock" (bool, rw):
   - LLM returns: `[{"ops": "turn_on", "value": true, "method": "setProps"}, {"ops": "turn_off", "value": false, "method": "setProps"}]` (if child_lock not in ops reference, returns empty)

Final operations array:
```json
[
  {
    "method": "setProps",
    "ops": "turn_on",
    "param": {"device_id": "socket123", "switch_1": true}
  },
  {
    "method": "setProps",
    "ops": "turn_off",
    "param": {"device_id": "socket123", "switch_1": false}
  },
  {
    "method": "getProps",
    "ops": "get_state",
    "param": {"device_id": "socket123"}
  }
]
```

**Note**: If a property doesn't match any operation in the reference, the LLM will return an empty array, and no operations will be generated for that property.

## Service Code Patterns

Common service codes and their typical properties:

- `switch`: switch, power states
- `light`: brightness, color, color_temp
- `fan`: fan_speed, mode, oscillate
- `climate`: temperature, mode, humidity
- `cover`: position, control (open/close/stop)
- `vacuum`: power, mode, status

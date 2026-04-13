# Brand-Specific Operation Saving

This reference document describes how to save device operations for different smart home brands (Xiaomi and Tuya).

---

## Xiaomi Operations

### SetProp Operation

Use for operations that set a device property (e.g., turn_on, set_brightness, set_temperature).

**Format:**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"saveDeviceOps","params":{"from_id":"<from_id>","from":"xiaomi","ops":"<operation_name>","command":"{\"did\":\"<from_id>\",\"siid\":<siid>,\"piid\":<piid>,\"value\":<value_template>}"}}
```

**Example - turn_on (concrete value):**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"saveDeviceOps","params":{"from_id":"12345","from":"xiaomi","ops":"turn_on","command":"{\"did\":\"12345\",\"siid\":2,\"piid\":1,\"value\":true}"}}
```

**Example - set_brightness (with placeholder):**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"saveDeviceOps","params":{"from_id":"12345","from":"xiaomi","ops":"set_brightness","command":"{\"did\":\"12345\",\"siid\":2,\"piid\":2,\"value\":\"$value$\"}"}}
```

**When to use:**
- Operation sets a property value
- Spec method is `SetProp`
- Use concrete values for fixed operations (turn_on: true, turn_off: false)
- Use `$value$` placeholder for dynamic operations (brightness, temperature, etc.)

---

### GetProp Operation

Use for operations that read a device property (e.g., get_state, get_brightness).

**Format:**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"saveDeviceOps","params":{"from_id":"<from_id>","from":"xiaomi","ops":"<operation_name>","command":"{\"did\":\"<from_id>\",\"siid\":<siid>,\"piid\":<piid>}"}}
```

**Example - get_state:**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"saveDeviceOps","params":{"from_id":"12345","from":"xiaomi","ops":"get_state","command":"{\"did\":\"12345\",\"siid\":2,\"piid\":1}"}}
```

**When to use:**
- Operation reads a property value
- Spec method is `GetProp`
- No value needed, just siid and piid

---

### Action Operation

Use for operations that trigger a device action (e.g., start, pause, stop).

**Format:**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"saveDeviceOps","params":{"from_id":"<from_id>","from":"xiaomi","ops":"<operation_name>","command":"{\"did\":\"<from_id>\",\"siid\":<siid>,\"aiid\":<aiid>,\"in\":[]}"}}
```

**Example - start vacuum:**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"saveDeviceOps","params":{"from_id":"vacuum123","from":"xiaomi","ops":"start","command":"{\"did\":\"vacuum123\",\"siid\":3,\"aiid\":1,\"in\":[]}"}}
```

**When to use:**
- Operation triggers an action (not setting/getting a property)
- Spec method is `Action`
- Uses `aiid` instead of `piid`
- `in` is the input parameters array (usually empty `[]`)

---

## Tuya Operations

### Set Property Operation

Use for operations that set a device property (e.g., turn_on, set_percentage).

**Format:**
```
hc_cli
- commandJson: {"brand":"tuya","method":"saveDeviceOps","params":{"from_id":"<from_id>","from":"tuya","ops":"<operation_name>","command":"{\"device_id\":\"<from_id>\",\"<property_code>\":<value_template>}"}}
```

**Example - turn_on (concrete value):**
```
hc_cli
- commandJson: {"brand":"tuya","method":"saveDeviceOps","params":{"from_id":"fan789","from":"tuya","ops":"turn_on","command":"{\"device_id\":\"fan789\",\"switch\":true}"}}
```

**Example - set_percentage (with placeholder):**
```
hc_cli
- commandJson: {"brand":"tuya","method":"saveDeviceOps","params":{"from_id":"fan789","from":"tuya","ops":"set_percentage","command":"{\"device_id\":\"fan789\",\"fan_speed\":\"$value$\"}"}}
```

**When to use:**
- Operation sets a property value
- Property code comes from Tuya spec `services[].properties[].code`
- Use concrete values for fixed operations
- Use `$value$` placeholder for dynamic operations

---

### Get Property Operation

Use for operations that read device properties (e.g., get_state).

**Format:**
```
hc_cli
- commandJson: {"brand":"tuya","method":"saveDeviceOps","params":{"from_id":"<from_id>","from":"tuya","ops":"<operation_name>","command":"{\"device_id\":\"<from_id>\"}"}}
```

**Example - get_state:**
```
hc_cli
- commandJson: {"brand":"tuya","method":"saveDeviceOps","params":{"from_id":"fan789","from":"tuya","ops":"get_state","command":"{\"device_id\":\"fan789\"}"}}
```

**When to use:**
- Operation reads property values
- Tuya reads all properties at once (no need to specify which property)
- Only need `device_id` in command

---

## Value Template Guidelines

### Concrete Values

Use for operations with fixed, known values:

| Operation | Value | Example |
|-----------|-------|---------|
| turn_on | `true` | `{\"value\":true}` |
| turn_off | `false` | `{\"value\":false}` |
| lock | `true` or `1` | Depends on device |
| unlock | `false` or `0` | Depends on device |

### Placeholder Values

Use `$value$` for operations that need dynamic values at runtime:

| Operation | Placeholder | Example |
|-----------|-------------|---------|
| set_brightness | `\"$value$\"` | `{\"value\":\"$value$\"}` |
| set_temperature | `\"$value$\"` | `{\"value\":\"$value$\"}` |
| set_percentage | `\"$value$\"` | `{\"fan_speed\":\"$value$\"}` |
| set_color_temp | `\"$value$\"` | `{\"value\":\"$value$\"}` |
| set_volume | `\"$value$\"` | `{\"volume\":\"$value$\"}` |

**Note:** Wrap `$value$` in quotes for numeric types that expect strings.

---

## Common Patterns

### Xiaomi Pattern

1. Get spec → find matching operation
2. Extract `siid` and `piid`/`aiid` from spec
3. Build command with device ID and IDs
4. Use concrete value or `$value$` placeholder

### Tuya Pattern

1. Get spec → find property code
2. Use property code directly in command
3. Build command with device ID and property
4. Use concrete value or `$value$` placeholder

---

## Error Prevention

1. **Verify spec first**: Always check getSpec before saving operations
2. **Match method type**: SetProp → setProps, GetProp → getProps, Action → execute
3. **Check value type**: Boolean vs Integer vs String
4. **Use correct ID fields**: Xiaomi uses siid/piid/aiid, Tuya uses property codes
5. **Test with concrete values first**: Verify operation works before using placeholders

---
name: device-spec-analyze
description: Analyze or generate device operations and save operations for smart home devices. Use when the user says: "analyze device operations", "generate operations", "get supported operations", "configure device operations", "what operations does this device support", or when devices need to be set up for control. Triggers on requests to analyze, generate, or retrieve device operation capabilities for smart home devices.
---

## Workflows

- **Workflow 1: Single Device** — Analyze and generate  device operations  for one device
- **Workflow 2: Batch All Devices** — Configure all unconfigured devices (reuses Workflow 1)

---

## Workflow 1: Single Device

### Step 1 — Find Device

```
hc_common
- commandJson: {"method":"listDevices"}
```

Returns device list with `from_id`, `from`, `name`, `type`, `urn`, `space_name`, `ops`.

from is brand
Find the right device by name or room. If multiple devices match, Must Confirm!

**Check the `ops` field:**
- If `ops` not exists or is empty or `[]` → Continue to Step 2 (needs operations generation)
- If `ops` contains `"NoAction"` → **IMMEDIATELY STOP**. Tell user: "This device has been marked as non-operable and does not support operations." Do NOT proceed to any other steps. **TERMINATE EXECUTION NOW.**
- If `ops` has valid operations → **IMMEDIATELY STOP**. Tell user: "This device already has operations configured: [list operations]. No need to regenerate." Do NOT proceed to any other steps. **TERMINATE EXECUTION NOW.**

### Step 2 — Get Device Spec

```
hc_cli - commandJson: {"brand":"<brand>","method":"getSpec","params":{"deviceId":"<from_id>"}}
```

### Step 3 — Load Brand Parsing Rules

- **Xiaomi**: Load `reference/xiaomi.md`
- **Tuya**: Load `reference/tuya.md`

Apply parsing rules to:
1. Parse spec response (brand-specific format)
2. Map spec entries to standard operations
3. Generate `ops_array`

### Step 4 — Batch Save Operations or Mark as NoAction

**If operations were generated:**

Pass the entire JSON array from Step 3 as `ops_array` parameter, along with `from` and `from_id`:

```
hc_cli - commandJson: {"brand":"<brand>","method":"saveDeviceOps","params":{"from":"<brand>","from_id":"<from_id>","ops_array":"<step3.jsonArrayString>"}}
```

Parameters:
- `from`: Brand name (e.g., "xiaomi", "tuya")
- `from_id`: Device ID
- `ops_array`: The complete JSON array string from Step 3, containing all operations with `method`, `ops`, and `param` fields

**Example**: If Step 3 generated the following array:
```json
[
  {"method": "SetProp", "ops": "turn_on", "param": {"did": "12345", "siid": 2, "piid": 1, "value": true}},
  {"method": "SetProp", "ops": "turn_off", "param": {"did": "12345", "siid": 2, "piid": 1, "value": false}},
  {"method": "GetProp", "ops": "get_state", "param": {"did": "12345", "siid": 2, "piid": 1}}
]
```

Then call:
```
hc_cli - commandJson: {"brand":"xiaomi","method":"saveDeviceOps","params":{"from":"xiaomi","from_id":"12345","ops_array":"[{\"method\":\"SetProp\",\"ops\":\"turn_on\",\"param\":{\"did\":\"12345\",\"siid\":2,\"piid\":1,\"value\":true}},{\"method\":\"SetProp\",\"ops\":\"turn_off\",\"param\":{\"did\":\"12345\",\"siid\":2,\"piid\":1,\"value\":false}},{\"method\":\"GetProp\",\"ops\":\"get_state\",\"param\":{\"did\":\"12345\",\"siid\":2,\"piid\":1}}]"}}
```

**If NO operations could be generated:**
```
hc_cli - commandJson: {"brand":"<brand>","method":"markNoAction","params":{"from_id":"<id>","from":"<brand>"}}
```
Mark the device as non-operable (e.g., IR devices, gateways, sensors without control capability).


---

## Workflow 2: Batch All Devices

### Step 1 — List Unconfigured Devices

```
hc_cli - commandJson: {"brand":"any","method":"listDevicesWithoutOps"}
```

If count is 0, skip to end.

### Step 2 — Process Each Device

For **EACH** device in the list:
Use Workflow 1: Single Device with <brand> and <from_id>

### Step 3 — Verify

```
hc_cli - commandJson: {"brand":"any","method":"listDevicesWithoutOps"}
```

Should return count: 0

---

## Examples

### Xiaomi Light
```
1. getSpec → [{"desc":"Switch Status","method":"SetProp","param":{"siid":2,"piid":1,"value":"$value$"},"param_desc":"bool"}]
2. Load reference/xiaomi.md
3. Generate ops_array: [{"method":"SetProp","ops":"turn_on","param":{"did":"123","siid":2,"piid":1,"value":true}}, {"method":"SetProp","ops":"turn_off","param":{"did":"123","siid":2,"piid":1,"value":false}}, {"method":"GetProp","ops":"get_state","param":{"did":"123","siid":2,"piid":1}}]
4. saveDeviceOps(from="xiaomi", from_id="123", ops_array) → successfully saved 3 device operations
```

### Tuya Fan
```
1. getSpec → {"services":[{"properties":[{"code":"switch","type":"Boolean"},{"code":"fan_speed","type":"Integer","range":"1-4"}]}]}
2. Load reference/tuya.md
3. Generate ops_array with turn_on, turn_off, set_percentage operations
4. saveDeviceOps(from="tuya", from_id="fan789", ops_array) → successfully saved 3 device operations
```

### Gateway (No Operations)
```
1. getSpec → [] or empty response
2. Load reference/xiaomi.md - no matching operations found
3. markNoAction: device marked as non-operable
```

## Rules

1. Use `$value$` for dynamic values (brightness, temperature)
2. Use concrete values for fixed operations (turn_on: true, turn_off: false)
3. Save ALL supported operations for each device
4. Load brand-specific parsing rules before analyzing specs
5. Skip unsupported operations gracefully

## Error Handling

- **No devices**: Already configured, inform user
- **getSpec fails**: Skip device, continue
- **No operations generated**: Call markNoAction to mark device as non-operable
- **saveDeviceOps fails**: Retry once, then skip
- **Unknown type**: Save common ops (turn_on, turn_off, get_state)

## References

- `reference/xiaomi.md` — Xiaomi MIoT parsing rules
- `reference/tuya.md` — Tuya Thing Model parsing rules
- `reference/ops.md` — support operations
## Prerequisites

- Devices synced first (use `device-sync` skill)
- Brand credentials configured (xiaomi/tuya)

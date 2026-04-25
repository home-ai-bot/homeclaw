---
name: device-spec-analyze
description: Analyze device spec and generate operations. Triggers: "analyze device operations", "generate operations", "configure device operations", "what operations does this device support".
---

## Workflow 1: Single Device

### Step 1 — Find Device
```
hc_common - commandJson: {"method":"listDevices"}
```
Returns: from_id, from(brand), name, urn, ops
- ops empty/[] → Step 2
- ops has valid data → STOP: already configured

### Step 2 — Get Spec
```
hc_cli - commandJson: {"brand":"<brand>","method":"getSpec","params":{"deviceId":"<from_id>"}}
```

### Step 3 — Generate Operations
Apply brand rules from `reference/<brand>.md`. ONLY output ops from `reference/ops.md`.

Output format (per operation):
```json
{
  "ops": "turn_on",
  "param_type": "bool",
  "param_value": null,
  "method": "SetProp",
  "method_param": {"did":"{{.deviceId}}","siid":2,"piid":1,"value":"{{.value}}"}
}
```

Fields:
- `ops`: operation name from ops.md
- `param_type`: bool/int/enum/string/in
- `param_value`: null for get ops, true/false for bool, "min-max" for int range, {"1":"desc"} for enum
- `method`: SetProp/GetProp/Action/setProps/getProps/execute
- `method_param`: Go template with {{.deviceId}} and {{.value}} placeholders

### Step 4 — Save
```
hc_cli - commandJson: {"brand":"<brand>","method":"saveDeviceOps","params":{"from":"<brand>","urn":"<device_urn>","ops_array":"<step3.jsonArrayString>"}}
```

Note: Operations are stored per device type (URN), not per device instance. Devices with the same URN share the same operations.

## Workflow 2: Batch All Devices
1. `hc_cli - commandJson: {"brand":"any","method":"listDevicesWithoutOps"}` → if 0, done
2. For each device: Workflow 1
3. Verify: `listDevicesWithoutOps` → count 0

Note: listDevicesWithoutOps returns full device objects and deduplicates by URN. Only devices with completely empty ops are returned.

## Rules
- ONLY ops from ops.md list, 不在则忽略
- 仅生成rw/w操作，跳过read-only属性
- bool: turn_on=true, turn_off=false; dynamic: "$value$"
- method_param MUST use {{.deviceId}} template for device ID
- method_param MUST use {{.value}} template for runtime values (int/enum/string ops)

## Error Handling
- No devices → inform user
- getSpec fails → skip device, continue
- No ops generated → inform user, device cannot be configured
- saveDeviceOps fails → retry once

## Prerequisites
- Devices synced (device-sync skill)
- Brand credentials configured


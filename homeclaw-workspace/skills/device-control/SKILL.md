---
name: device-control
description: Control smart home devices from any brand (Xiaomi, Tuya, etc.). Use when the user wants to control, operate, or query status of smart devices (lights, fans, AC, switches, plugs, vacuum, etc.). Triggers on commands like "turn on the living room light", "set AC temperature to 26", "start vacuum cleaning", "is the bedroom light on", or any request to operate or query smart home devices. For camera control and visual analysis, use the camera-control skill.
---


## Workflows

- **Workflow 1: Control Device** — change device state (turn on/off, set value)
- **Workflow 2: Query Device Status** — read current property values

---

## Workflow 1: Control Device

### Step 1 — Find device & check operations

```
hc_common
- commandJson: {"method":"listDevices"}
```

Returns device list with `from_id`, `from`, `name`, `type`, `urn`, `space_name`, `ops`.

Find the right device by name or room. If multiple devices match, Must Confirm!

**Check the `ops` field:**
- If `ops` contains `["NoAction"]` → **IMMEDIATELY STOP**. Tell user: "This device does not support operations." Do NOT proceed to any other steps. Do NOT attempt to execute commands. **TERMINATE EXECUTION NOW.**
- If `ops` not exists or is empty or `[]` → **IMMEDIATELY STOP**. Tell user: "Please run the device analysis command first to generate operations for this device." Do NOT proceed to any other steps. Do NOT attempt to execute commands. **TERMINATE EXECUTION NOW.**
- If `ops` has valid operations → Continue to Step 2

### Step 2 — Execute command

Use the `hc_cli` exe method with the appropriate operation:

```
hc_cli
- commandJson: {"brand":"<from>","method":"exe","params":{"from_id":"<from_id>","ops":"<operation_name>"}}
```

The `ops` parameter should be one of the operation names from the device's `ops` field (e.g., "turn_on", "turn_off", "set_temperature", etc.)

---

## Workflow 2: Query Device Status

When user asks "Is the living room light on?", "What's the AC set to?", "Check fan status", etc.

### Step 1 — Find device & check operations

Same as Workflow 1 Step 1.

### Step 2 — Read current state

Use the `hc_cli` exe method with the get_state operation:

```
hc_cli
- commandJson: {"brand":"<brand>","method":"exe","params":{"from_id":"<from_id>","ops":"get_state"}}
```

Translate the returned values to natural language and report to user.

---

## Examples

### Example 1: Turn On a Xiaomi Light

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="12345", from="xiaomi", name="Living Room Light", ops=["turn_on", "turn_off", "get_state"]

2. ops has valid operations, proceed to execute

3. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"exe\",\"params\":{\"from_id\":\"12345\",\"ops\":\"turn_on\"}}"}
   → Light turned on
```

### Example 2: Check Xiaomi AC Status

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="ac001", from="xiaomi", name="Bedroom AC", ops=["turn_on", "turn_off", "get_state", "set_temperature"]

2. ops has valid operations, proceed to query

3. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"exe\",\"params\":{\"from_id\":\"ac001\",\"ops\":\"get_state\"}}"}
   → {"value": true}
   → Report: "The bedroom AC is currently on."
```

### Example 3: Device Operations Not Configured

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="vacuum123", from="xiaomi", name="Robot Vacuum", ops=[]

2. ops is empty, **IMMEDIATELY STOP EXECUTION**. Inform user: "Please run the device analysis command first to generate operations for this device." Do NOT proceed further. **TERMINATE NOW.**
```

### Example 4: Check if Tuya AC is On

```
1. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"listDevices\"}"}
   → from_id="ac456", from="tuya", name="Bedroom AC", ops=["turn_on", "turn_off", "get_state", "set_temperature", "set_mode"]

2. ops has valid operations, proceed to query

3. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"exe\",\"params\":{\"from_id\":\"ac456\",\"ops\":\"get_state\"}}"}
   → {"switch": true, "temp_set": 260, "mode": "cold"}
   → Report: "The bedroom AC is on, set to 26°C in cooling mode."
```

### Example 5: Device Does Not Support Operations

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="gateway01", from="xiaomi", name="Smart Gateway", ops=["NoAction"]

2. ops contains ["NoAction"], **IMMEDIATELY STOP EXECUTION**. Inform user: "This device does not support operations." Do NOT proceed further. **TERMINATE NOW.**
```

### Example 6: Set Xiaomi Fan Speed

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="67890", from="xiaomi", name="Bedroom Fan", ops=["turn_on", "turn_off", "set_speed", "get_state"]

2. ops has valid operations, proceed to execute

3. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"exe\",\"params\":{\"from_id\":\"67890\",\"ops\":\"set_speed\"}}"}
   → Fan speed set to level 3
```

---

## Error Handling

- **Device not found**: Ask user for more specific device name or room; if multiple match, list candidates and ask user to confirm
- **Device has ["NoAction"]**: **IMMEDIATELY TERMINATE**. Inform user "This device does not support operations" and STOP ALL EXECUTION. Do not attempt any further actions.
- **Operations not configured**: **IMMEDIATELY TERMINATE**. If ops is empty, inform user "Please run the device analysis command first to generate operations for this device" and STOP ALL EXECUTION. Do not attempt any further actions.
- **Device offline**: Inform user the device is offline, do not proceed
- **Brand not registered**: Credentials not configured, inform user to run device-sync first
- **Auth error / token invalid**: For Xiaomi, ask user to re-login via web UI; for Tuya, reconfigure API key

---

## Prerequisites

- Devices must be synced first (use `device-sync` skill)
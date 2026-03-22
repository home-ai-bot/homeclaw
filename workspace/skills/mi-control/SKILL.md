---
name: mi-control
description: Control Xiaomi/Mi Home smart devices. Use when the user wants to control, operate, or interact with Xiaomi smart devices (lights, fans, air purifiers, switches, etc.). Handles device discovery, spec lookup, authentication, and command execution. Triggers on commands like "turn on the living room light", "set fan speed to 3", "start vacuum cleaning", or any request to operate Mi Home devices.
---

# Mi Control - Control Xiaomi Smart Devices

Control Xiaomi/Mi Home smart devices through the cloud API using two methods:
- **`mi_set_prop`** - Set individual property states (on/off, brightness, speed)
- **`mi_action`** - Trigger multi-step predefined actions (start cleaning, factory reset)

## Decision Guide: mi_set_prop vs mi_action

### Use `mi_set_prop` when:
- Setting a **single state/value** directly
- The spec shows a **property** with `access: ["read", "write"]`
- Examples:
  - Turn on/off a light → set `switch` property to `true/false`
  - Set brightness to 80% → set `brightness` property to `80`
  - Set fan speed level → set `fan-level` property to `2`
  - Set target temperature → set `target-temperature` property to `24`

### Use `mi_action` when:
- Triggering a **predefined task/operation**
- The spec shows an **action** definition (has `aiid`)
- The operation involves **multiple internal steps**
- Examples:
  - Start robot vacuum cleaning → triggers battery check, brush lowering, motor start
  - Stop vacuum and return to dock → triggers navigation, cleaning stop
  - Start camera recording → triggers stream initialization
  - Factory reset device → triggers multi-step reset sequence

## Workflow

1. **Find Device** → `hc_list_devices`
2. **Get Device Spec** → `mi_get_spec` (find properties or actions)
3. **Check Auth** → `mi_get_account` (verify token status)
4. **Execute Command** → `mi_set_prop` OR `mi_action`

## Step 1: Find Device

```
hc_list_devices
```

Returns device list with `id`, `name`, `did`, `room_name`, `urn`.

## Step 2: Get Device Spec

```
mi_get_spec
- urn: the device URN from step 1
```

Returns MIoT spec JSON with services containing:

**Properties** (for `mi_set_prop`):
```json
{
  "type": "urn:miot-spec-v2:device:vacuum:0000A006:roborock-m1s:2",
  "description": "Robot Cleaner",
  "services": [
    {
      "iid": 4,
      "type": "urn:miot-spec-v2:service:battery:00007805:roborock-m1s:1",
      "description": "Battery",
      "properties": [
        {
          "iid": 1,
          "type": "urn:miot-spec-v2:property:battery-level:00000014:roborock-m1s:1",
          "description": "Battery Level",
          "format": "uint8",
          "access": [
            "read",
            "notify"
          ],
          "unit": "percentage",
          "value-range": [0, 100, 1]
        }
      ],
      "actions": [
        {
          "iid": 1,
          "type": "urn:miot-spec-v2:action:start-charge:00002802:roborock-m1s:1",
          "description": "Start Charge",
          "in": [],
          "out": []
        }
      ]
    }
  ]
}
```

services sub iid is siid
         sub properties sub iid is piid,format convert to `valueType`
         sub actions sub iid is siid

please determine only one actions or properties , get iid,iid as `siid`, `aiid` or  `piid`
## Step 3: Check Authentication

Call `mi_get_account`.

- If `account` is non-null → token valid, proceed to Step 4.
- If `account` is null → proceed to OAuth flow below.

### OAuth Flow (if needed)

**3a. Obtain OAuth authorization:**

Call `mi_get_oauth_url`. Present the returned `url` to the user:

> Please open this URL in your browser and complete Xiaomi login:
> `<url>`
> After login you will be redirected to a callback URL. Please paste the full callback URL or just the `code_value` parameter.

Wait for the user to provide the code.

**3b. Exchange code for token:**

Call `mi_get_access_token` with the `code` the user provided.

On success the token is saved. Continue to Step 4.

## Step 4: Execute Command

### Option A: `mi_set_prop` (Set Property)

Use for directly setting individual states.

```
mi_set_prop
- did: device ID from step 1
- siid: service ID from spec
- piid: property ID from spec
- value: the value to set (bool, int, string, etc.)
```

### Option B: `mi_action` (Trigger Action)

Use for triggering predefined multi-step operations.

```
mi_action
- did: device ID from step 1
- siid: service ID from spec
- aiid: action ID from spec
- in: optional array of input parameters [{"value": ...}]
```

## Examples

### Example 1: Turn On a Light (use mi_set_prop)

Light on/off is a simple property change.

```
1. hc_list_devices {"room_name": "living room"}
   → did="12345", urn="urn:miot-spec-v2:device:light:..."

2. mi_get_spec {"urn": "..."}
   → Service siid=2 has property: piid=1 "Switch Status" (bool, writable)

3. mi_get_account {}
   → Valid account

4. mi_set_prop {"did": "12345", "siid": 2, "piid": 1, "value": true}
   → Light turned on
```

### Example 2: Set Fan Speed (use mi_set_prop)

Fan speed is a property value.

```
1. hc_list_devices → did="67890"

2. mi_get_spec → Service siid=2 has property: piid=2 "Fan Level" (int 1-4)

3. mi_set_prop {"did": "67890", "siid": 2, "piid": 2, "value": 3}
   → Fan speed set to level 3
```

### Example 3: Start Robot Vacuum Cleaning (use mi_action)

Starting cleaning is a multi-step action.

```
1. hc_list_devices → did="vacuum123"

2. mi_get_spec → Service siid=3 has action: aiid=1 "Start Sweep"

3. mi_action {"did": "vacuum123", "siid": 3, "aiid": 1, "in": []}
   → Vacuum starts cleaning (internally: check battery, lower brush, start motor)
```

### Example 4: Stop Vacuum and Return to Dock (use mi_action)

```
mi_action {"did": "vacuum123", "siid": 3, "aiid": 2, "in": []}
→ Vacuum stops and returns to charging dock
```

## Quick Reference

| User Request | Tool | Key Parameter |
|-------------|------|---------------|
| Turn on/off light | `mi_set_prop` | piid (switch property) |
| Set brightness | `mi_set_prop` | piid (brightness property) |
| Set fan speed | `mi_set_prop` | piid (fan-level property) |
| Set temperature | `mi_set_prop` | piid (target-temp property) |
| Start vacuum cleaning | `mi_action` | aiid (start-sweep action) |
| Stop vacuum / return dock | `mi_action` | aiid (stop-sweep action) |
| Start camera stream | `mi_action` | aiid (start-stream action) |
| Factory reset | `mi_action` | aiid (reset action) |

## Error Handling

- **Device not found**: Ask user for more specific device name/room
- **Spec parse error**: Device may not support cloud control
- **Auth failed**: Retry OAuth flow
- **Command failed**: Check device online status, verify parameters match spec
- **Property not writable**: Check spec `access` array includes "write"
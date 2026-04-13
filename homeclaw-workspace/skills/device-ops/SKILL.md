---
name: device-ops
description: Analyze and save device operations for smart home devices. Use when you need to configure device capabilities, set up operation mappings, or prepare devices for automated control. This skill discovers devices without operations, analyzes their capabilities via getSpec, and saves the operation configurations.
---

## Purpose

This skill analyzes smart home devices and saves their operation configurations so they can be used by other skills (like device-control) for automated device control. It's typically run during initial setup or when new devices are added.

## Workflows

- **Workflow 1: Configure Operations for All Devices Without Ops** — batch process all unconfigured devices
- **Workflow 2: Configure Operations for Specific Device** — configure a single device

---

## Workflow 1: Configure Operations for All Devices Without Ops

### Step 1 — List devices without operations

```
hc_cli
- commandJson: {"brand":"any","method":"listDevicesWithoutOps"}
```

Returns devices that don't have any operations configured:
```json
{
  "devices": [
    {"from_id": "12345", "from": "xiaomi", "name": "Living Room Light", "type": "light", "space_name": "Living Room"},
    {"from_id": "67890", "from": "tuya", "name": "Bedroom Fan", "type": "fan"}
  ],
  "count": 2
}
```

If count is 0 or message says "All devices have operations configured", skip to end.

### Step 2 — For each device, get spec and analyze operations

For **EACH** device in the list:

#### Step 2.1 — Get device specification

Call `getSpec` with the device's brand and from_id. See **Reference: Brand-Specific Spec Format** below for response format details.

#### Step 2.2 — Map operations based on device type

Use the device `type` field and reference the supported operations below. See **Reference: Brand-Specific Operation Saving** below for command format details.

### Supported Device Types and Operations

#### light
**Operations:** `turn_on`, `turn_off`, `get_state`, `get_brightness`, `set_brightness`, `get_color_temp`, `set_color_temp`, `get_rgb_color`, `set_rgb_color`, `get_effect`, `set_effect`

**Analysis rules:**
- Find SetProp with "Switch" or "Power" in desc → map to `turn_on`/`turn_off` (value: true/false)
- Find GetProp with "Switch" or "Power" → map to `get_state`
- Find SetProp with "Brightness" → map to `set_brightness`
- Find GetProp with "Brightness" → map to `get_brightness`
- Find SetProp with "Color Temperature" → map to `set_color_temp`
- Find GetProp with "Color Temperature" → map to `get_color_temp`
- Find SetProp with "Color" or "RGB" → map to `set_rgb_color`
- Find GetProp with "Color" or "RGB" → map to `get_rgb_color`
- Find SetProp with "Effect" → map to `set_effect`
- Find GetProp with "Effect" → map to `get_effect`

#### switch
**Operations:** `turn_on`, `turn_off`, `get_state`

**Analysis rules:**
- Find SetProp with "Switch" or "Relay" → map to `turn_on`/`turn_off`
- Find GetProp with "Switch" or "Relay" → map to `get_state`

#### camera
**Operations:** `enable_motion_detection`, `disable_motion_detection`, `pan_left`, `pan_right`, `tilt_up`, `tilt_down`, `zoom_in`, `zoom_out`, `set_position`, `get_position`

**Analysis rules:**
- Find Action/SetProp with "Motion Detection" → map to `enable_motion_detection`/`disable_motion_detection`
- Find Action with "Pan Left" or similar → map to `pan_left`
- Find Action with "Pan Right" or similar → map to `pan_right`
- Find Action with "Tilt Up" or similar → map to `tilt_up`
- Find Action with "Tilt Down" or similar → map to `tilt_down`
- Find Action with "Zoom In" → map to `zoom_in`
- Find Action with "Zoom Out" → map to `zoom_out`
- Find SetProp with "Position" → map to `set_position`
- Find GetProp with "Position" → map to `get_position`

#### vacuum
**Operations:** `start`, `pause`, `stop`, `return_to_base`, `set_fan_speed`, `get_state`

**Analysis rules:**
- Find Action with "Start Sweep" or "Start Clean" → map to `start`
- Find Action with "Pause" → map to `pause`
- Find Action with "Stop" → map to `stop`
- Find Action with "Return" or "Dock" → map to `return_to_base`
- Find SetProp with "Fan Speed" or "Suction" → map to `set_fan_speed`
- Find GetProp with "Status" or "State" → map to `get_state`

#### fan
**Operations:** `turn_on`, `turn_off`, `get_state`, `get_percentage`, `set_percentage`, `get_preset_mode`, `set_preset_mode`, `get_oscillate`, `oscillate`, `get_direction`, `set_direction`

**Analysis rules:**
- Find SetProp with "Switch" or "Power" → map to `turn_on`/`turn_off`
- Find GetProp with "Switch" or "Power" → map to `get_state`
- Find SetProp with "Fan Level" or "Speed" → map to `set_percentage`
- Find GetProp with "Fan Level" or "Speed" → map to `get_percentage`
- Find SetProp with "Mode" → map to `set_preset_mode`
- Find GetProp with "Mode" → map to `get_preset_mode`
- Find SetProp with "Oscillate" or "Shake" → map to `oscillate`
- Find GetProp with "Oscillate" or "Shake" → map to `get_oscillate`
- Find SetProp with "Direction" → map to `set_direction`
- Find GetProp with "Direction" → map to `get_direction`

#### climate
**Operations:** `turn_on`, `turn_off`, `get_state`, `set_hvac_mode`, `set_temperature`, `set_humidity`, `set_fan_mode`, `set_swing_mode`, `set_preset_mode`

**Analysis rules:**
- Find SetProp with "Switch" or "Power" → map to `turn_on`/`turn_off`
- Find GetProp with "Switch" or "Power" → map to `get_state`
- Find SetProp with "Mode" or "HVAC Mode" → map to `set_hvac_mode`
- Find SetProp with "Temperature" or "Temp Set" → map to `set_temperature`
- Find SetProp with "Humidity" → map to `set_humidity`
- Find SetProp with "Fan Mode" → map to `set_fan_mode`
- Find SetProp with "Swing" → map to `set_swing_mode`
- Find SetProp with "Preset" → map to `set_preset_mode`

#### cover
**Operations:** `open`, `close`, `stop`, `set_position`, `get_state`

**Analysis rules:**
- Find Action/SetProp with "Open" → map to `open`
- Find Action/SetProp with "Close" → map to `close`
- Find Action with "Stop" → map to `stop`
- Find SetProp with "Position" → map to `set_position`
- Find GetProp with "Position" or "Status" → map to `get_state`

#### humidifier
**Operations:** `turn_on`, `turn_off`, `get_state`, `set_mode`, `set_humidity`

**Analysis rules:**
- Find SetProp with "Switch" or "Power" → map to `turn_on`/`turn_off`
- Find GetProp with "Switch" or "Power" → map to `get_state`
- Find SetProp with "Mode" → map to `set_mode`
- Find SetProp with "Humidity" → map to `set_humidity`

#### water_heater
**Operations:** `turn_on`, `turn_off`, `get_state`, `set_temperature`, `set_operation_mode`

**Analysis rules:**
- Find SetProp with "Switch" or "Power" → map to `turn_on`/`turn_off`
- Find GetProp with "Switch" or "Power" → map to `get_state`
- Find SetProp with "Temperature" → map to `set_temperature`
- Find SetProp with "Mode" or "Operation" → map to `set_operation_mode`

#### tv / tvbox
**Operations:** `turn_on`, `turn_off`, `get_state`, `play`, `pause`, `stop`, `set_volume`, `mute`, `select_source`

**Analysis rules:**
- Find SetProp with "Power" → map to `turn_on`/`turn_off`
- Find GetProp with "Power" → map to `get_state`
- Find Action with "Play" → map to `play`
- Find Action with "Pause" → map to `pause`
- Find Action with "Stop" → map to `stop`
- Find SetProp with "Volume" → map to `set_volume`
- Find SetProp/Action with "Mute" → map to `mute`
- Find SetProp with "Source" or "Input" → map to `select_source`

#### speaker
**Operations:** `turn_on`, `turn_off`, `get_state`, `play`, `pause`, `stop`, `next_track`, `previous_track`, `set_volume`, `mute`, `play_text`, `execute_text_directive`, `wake_up`, `ir_turn_on`, `ir_turn_off`, `ir_set_volume`, `ir_send_command`

**Analysis rules:**
- Similar to tv/tvbox, plus:
- Find Action with "Next" → map to `next_track`
- Find Action with "Previous" → map to `previous_track`
- Find Action with "Play Text" or "TTS" → map to `play_text`
- Find Action with "Wake" → map to `wake_up`

#### lock
**Operations:** `lock`, `unlock`, `get_state`

**Analysis rules:**
- Find Action/SetProp with "Lock" → map to `lock`
- Find Action/SetProp with "Unlock" → map to `unlock`
- Find GetProp with "Status" or "State" → map to `get_state`

#### doorbell
**Operations:** `get_state`

**Analysis rules:**
- Find GetProp with "Status" or "State" → map to `get_state`

#### sensor_* (all sensor types)
**Operations:** `get_state`

**Analysis rules:**
- Find GetProp for any property → map to `get_state`

### Step 2.3 — Save each operation

For EACH operation you identified, save it using the appropriate brand format. See **Reference: Brand-Specific Operation Saving** below for detailed command formats and examples.

### Step 3 — Verify completion

After processing all devices, optionally verify:
```
hc_cli
- commandJson: {"brand":"any","method":"listDevicesWithoutOps"}
```

Should return count: 0 or "All devices have operations configured"

---

## Workflow 2: Configure Operations for Specific Device

When you need to configure operations for a specific device only.

### Step 1 — Get device spec

Same as Workflow 1 Step 2.1

### Step 2 — Analyze and save operations

Same as Workflow 1 Steps 2.2 and 2.3, but only for the target device.

---

## Examples

### Example 1: Configure Xiaomi Light Operations

```
1. hc_cli {"commandJson":"{\"brand\":\"any\",\"method\":\"listDevicesWithoutOps\"}"}
   → devices: [{"from_id":"12345","from":"xiaomi","name":"Living Room Light","type":"light"}]

2. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"getSpec\",\"params\":{\"deviceId\":\"12345\"}}"}
   → [
       {"desc":"Light Control-Switch Status","method":"SetProp","param":{"did":"","siid":2,"piid":1,"value":"$value$"},"param_desc":"bool"},
       {"desc":"Light Control-Brightness","method":"SetProp","param":{"did":"","siid":2,"piid":2,"value":"$value$"},"param_desc":"int 0-100"}
     ]

3. Save turn_on operation:
   hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"12345\",\"from\":\"xiaomi\",\"ops\":\"turn_on\",\"command\":\"{\\\"did\\\":\\\"12345\\\",\\\"siid\\\":2,\\\"piid\\\":1,\\\"value\\\":true}\"}}"}

4. Save turn_off operation:
   hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"12345\",\"from\":\"xiaomi\",\"ops\":\"turn_off\",\"command\":\"{\\\"did\\\":\\\"12345\\\",\\\"siid\\\":2,\\\"piid\\\":1,\\\"value\\\":false}\"}}"}

5. Save get_state operation:
   hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"12345\",\"from\":\"xiaomi\",\"ops\":\"get_state\",\"command\":\"{\\\"did\\\":\\\"12345\\\",\\\"siid\\\":2,\\\"piid\\\":1}\"}}"}

6. Save set_brightness operation:
   hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"12345\",\"from\":\"xiaomi\",\"ops\":\"set_brightness\",\"command\":\"{\\\"did\\\":\\\"12345\\\",\\\"siid\\\":2,\\\"piid\\\":2,\\\"value\\\":\\\"$value$\\\"}\"}}"}

7. Save get_brightness operation:
   hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"12345\",\"from\":\"xiaomi\",\"ops\":\"get_brightness\",\"command\":\"{\\\"did\\\":\\\"12345\\\",\\\"siid\\\":2,\\\"piid\\\":2}\"}}"}
```

### Example 2: Configure Tuya Fan Operations

```
1. hc_cli {"commandJson":"{\"brand\":\"any\",\"method\":\"listDevicesWithoutOps\"}"}
   → devices: [{"from_id":"fan789","from":"tuya","name":"Bedroom Fan","type":"fan"}]

2. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"getSpec\",\"params\":{\"deviceId\":\"fan789\"}}"}
   → {"services":[{"code":"switch","name":"Switch","properties":[{"code":"switch","type":"Boolean"},{"code":"fan_speed","type":"Integer","range":"1-4"}]}]}

3. Save turn_on operation:
   hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"fan789\",\"from\":\"tuya\",\"ops\":\"turn_on\",\"command\":\"{\\\"device_id\\\":\\\"fan789\\\",\\\"switch\\\":true}\"}}"}

4. Save turn_off operation:
   hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"fan789\",\"from\":\"tuya\",\"ops\":\"turn_off\",\"command\":\"{\\\"device_id\\\":\\\"fan789\\\",\\\"switch\\\":false}\"}}"}

5. Save get_state operation:
   hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"fan789\",\"from\":\"tuya\",\"ops\":\"get_state\",\"command\":\"{\\\"device_id\\\":\\\"fan789\\\"}\"}}"}

6. Save set_percentage operation:
   hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"saveDeviceOps\",\"params\":{\"from_id\":\"fan789\",\"from\":\"tuya\",\"ops\":\"set_percentage\",\"command\":\"{\\\"device_id\\\":\\\"fan789\\\",\\\"fan_speed\\\":\\\"$value$\\\"}\"}}"}
```

---

## Important Rules

1. **Use `$value$` placeholder** for operations that need dynamic values (e.g., set_brightness, set_temperature)
2. **Use concrete values** for operations with fixed values (e.g., turn_on uses `true`, turn_off uses `false`)
3. **Save ALL applicable operations** for each device type - don't skip operations that the device supports
4. **Match operations to device type** - use the type field from the device, not the URN or name
5. **Verify spec before saving** - always check getSpec results to ensure the operation exists
6. **Handle missing operations gracefully** - if a device doesn't support an operation from the list, skip it
7. **Process all devices** - when using listDevicesWithoutOps, process every device in the list

---

## Error Handling

- **No devices without ops**: Device operations are already configured, inform user
- **getSpec fails**: Device may not support cloud control, skip this device and continue
- **saveDeviceOps fails**: Retry once, then skip and continue with other operations
- **Unknown device type**: Analyze spec manually and save common operations (turn_on, turn_off, get_state)
- **Spec returns empty**: Device may not be fully configured, inform user

---

## Reference Documents

When you need brand-specific details, load these reference files:

- **Spec Formats**: `reference/spec-format.md` — Xiaomi MIoT and Tuya specification response formats
- **Operation Saving**: `reference/operation-saving.md` — How to save operations for each brand with examples

Load these files when:
- Step 2.1: Need to understand spec response structure
- Step 2.3: Need command format templates and examples

---

## Prerequisites

- Devices must be synced first (use `device-sync` skill)
- Brand credentials must be configured (xiaomi/tuya)
- This skill is typically run once during initial setup or when new devices are added

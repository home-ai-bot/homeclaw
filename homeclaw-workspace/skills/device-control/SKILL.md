---
name: device-control
description: Control smart home devices from any brand (Xiaomi, Tuya, etc.). Use when the user wants to control, operate, query status, or interact with smart devices (lights, fans, AC, switches, plugs, vacuum, etc.). Triggers on commands like "turn on the living room light", "set AC temperature to 26", "start vacuum cleaning", "is the bedroom light on", or any request to operate or query smart home devices. 
---

# Device Control

Control smart home devices from any brand through the unified `hc_cli` tool.

**Available Methods:**
- **`listDevices`** — List all synced devices
- **`listCameras`** — List camera devices with RTSP URLs
- **`getSpec`** — Get device capability specification (available commands/properties)
- **`getProps`** — Read device property values
- **`setProps`** — Write device property values
- **`execute`** — Trigger device actions

**Related Tools:**
- **`hc_video`** — Unified video tool: `capImage` (capture frame), `capAnalyze` (capture and analyze)

## Workflows

- **Workflow 1: Control Device** — change device state (turn on/off, set value)
- **Workflow 2: Query Device Status** — read current property values
- **Workflow 3: Camera Capture & Visual Analysis** — capture camera frame and analyze

---

## Workflow 1: Control Device

When user says "turn on the living room light", "set AC to 26°C", "start vacuum", etc.

### Step 1 — Find device

```
hc_cli
- commandJson: {"brand":"<brand>","method":"listDevices"}
```

Returns device list with `from_id`, `from`, `name`, `type`, `urn`, `space_name`.

Find the right device by name or room. If multiple devices match, Must Confirm!.
Note the `from_id` (device ID) and `from` (brand: `xiaomi` or `tuya`).

### Step 2 — Get device spec

Fetch available commands to determine what to send.

**Xiaomi:**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"getSpec","params":{"deviceId":"<from_id>"}}
```

Returns MIoT spec commands list:
```json
[
  {"desc": "Light Control-Switch Status", "method": "SetProp", "param": {"did": "", "siid": 2, "piid": 1, "value": "$value$"}, "param_desc": "bool"},
  {"desc": "Vacuum Control-Start Sweep",  "method": "Action",  "param": {"did": "", "siid": 3, "aiid": 1, "in": []},          "param_desc": ""}
]
```

- Pick **one** matching command
- If param contains `"value": "$value$"`, replace `$value$` with the correct value based on `param_desc` and user intent
- Fill in `"did"` with the device's `from_id`

**Tuya:**
```
hc_cli
- commandJson: {"brand":"tuya","method":"getSpec","params":{"deviceId":"<from_id>"}}
```

Returns Tuya Thing Model. Use `services` to identify property codes and value ranges.

### Step 3 — Execute command

**Xiaomi — Set property (SetProp):**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"setProps","params":{"did":"<from_id>","siid":<siid>,"piid":<piid>,"value":<value>}}
```

**Xiaomi — Trigger action (Action):**
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"execute","params":{"did":"<from_id>","siid":<siid>,"aiid":<aiid>,"in":[]}}
```

**Tuya — Set property:**
```
hc_cli
- commandJson: {"brand":"tuya","method":"setProps","params":{"device_id":"<from_id>","<property_code>":<value>}}
```

---

## Workflow 2: Query Device Status

When user asks "Is the living room light on?", "What's the AC set to?", "Check fan status", etc.

### Step 1 — Find device

Same as Workflow 1 Step 1.

### Step 2 — Read current state

**Xiaomi** (read specific property — get siid/piid from getSpec first if needed):
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"getProps","params":{"did":"<from_id>","siid":<siid>,"piid":<piid>}}
```

Or batch read multiple properties:
```
hc_cli
- commandJson: {"brand":"xiaomi","method":"getProps","params":{"props":[{"did":"<from_id>","siid":2,"piid":1},{"did":"<from_id>","siid":2,"piid":2}]}}
```

**Tuya** (reads all properties at once):
```
hc_cli
- commandJson: {"brand":"tuya","method":"getProps","params":{"device_id":"<from_id>"}}
```

Translate the returned values to natural language and report to user.

---

## Workflow 3: Camera Capture & Visual Analysis

When user asks "what does the camera see?", "is anyone at the door?", "check the baby monitor", etc.


### Step 1 — Find camera

```
hc_cli
- commandJson: {"brand":"<brand>","method":"listCameras"}
```

Returns camera list with `from_id`, `from`, `name`, `type`, `space_name`, `rtsp_url`.

- If user provides `rtsp_url` directly, skip this step
- Only proceed if camera is online

### Step 2 — Capture & analyze frame

```
hc_video
- commandJson: {"method":"capAnalyze","params":{"rtsp_url":"<rtsp_url from step 1>","prompt":"Is there anyone at the door?"}}
```

Or capture frame only:
```
hc_video
- commandJson: {"method":"capImage","params":{"rtsp_url":"<rtsp_url from step 1>"}}
```

Optional parameters:
- `rtsp_transport`: "tcp" (default) or "udp"
- `return_image`: true/false (if true, also returns the image via MediaResult)

```
hc_video
- commandJson: {"method":"capAnalyze","params":{"rtsp_url":"<rtsp_url>","prompt":"Describe the scene","return_image":true}}
```

Returns (capAnalyze):
```json
{"analysis": "Description of what's visible...","file_path": "/tmp/homeclaw_frame_123.jpg"}
```

Returns (capImage):
```json
{"file_path": "/tmp/homeclaw_frame_123.jpg"}
```

Report the analysis result to the user in natural language.

---

## Examples

### Example 1: Turn On a Xiaomi Light

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="12345", from="xiaomi", name="Living Room Light", urn="urn:miot-spec-v2:device:light:..."

2. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"getSpec\",\"params\":{\"deviceId\":\"12345\"}}"}
   → [{"desc":"Light Control-Switch Status","method":"SetProp","param":{"did":"","siid":2,"piid":1,"value":"$value$"},"param_desc":"bool"}]

3. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"setProps\",\"params\":{\"did\":\"12345\",\"siid\":2,\"piid\":1,\"value\":true}}"}
   → Light turned on
```

### Example 2: Check Xiaomi AC Status

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="ac001", from="xiaomi", name="Bedroom AC"

2. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"getSpec\",\"params\":{\"deviceId\":\"ac001\"}}"}
   → [{"desc":"Air Conditioner-Switch","method":"GetProp","param":{"did":"","siid":2,"piid":1},"param_desc":"bool"}, ...]

3. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"getProps\",\"params\":{\"did\":\"ac001\",\"siid\":2,\"piid\":1}}"}
   → {"value": true}
   → Report: "The bedroom AC is currently on."
```

### Example 3: Start Xiaomi Robot Vacuum

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="vacuum123", from="xiaomi", name="Robot Vacuum"

2. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"getSpec\",\"params\":{\"deviceId\":\"vacuum123\"}}"}
   → [{"desc":"Vacuum Control-Start Sweep","method":"Action","param":{"did":"","siid":3,"aiid":1,"in":[]},"param_desc":""}]

3. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"execute\",\"params\":{\"did\":\"vacuum123\",\"siid\":3,\"aiid\":1,\"in\":[]}}"}
   → Vacuum starts cleaning
```

### Example 4: Check if Tuya AC is On

```
1. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"listDevices\"}"}
   → from_id="ac456", from="tuya", name="Bedroom AC"

2. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"getProps\",\"params\":{\"device_id\":\"ac456\"}}"}
   → {"switch": true, "temp_set": 260, "mode": "cold"}
   → Report: "The bedroom AC is on, set to 26°C in cooling mode."
```

### Example 5: Turn On a Tuya Light

```
1. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"listDevices\"}"}
   → from_id="abc123", from="tuya", name="Bedroom Light"

2. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"getSpec\",\"params\":{\"deviceId\":\"abc123\"}}"}
   → services include property "switch_led" (bool)

3. hc_cli {"commandJson":"{\"brand\":\"tuya\",\"method\":\"setProps\",\"params\":{\"device_id\":\"abc123\",\"switch_led\":true}}"}
   → {"success": true}
```

### Example 6: What does the living room camera see?

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listCameras\"}"}
   → {"cameras": [{"from_id": "cam001", "from": "xiaomi", "name": "Living Room Camera", "type": "...", "space_name": "Living Room", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001"}]}

2. hc_video {"commandJson":"{\"method\":\"capAnalyze\",\"params\":{\"rtsp_url\":\"rtsp://127.0.0.1:8554/xiaomi_cam001\",\"prompt\":\"Describe what you see in the living room\"}}"}
   → {"analysis": "The living room is empty. A sofa and TV are visible. No people present."}
```

### Example 6b: Capture camera frame only

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listCameras\"}"}
   → {"cameras": [{"from_id": "cam001", "from": "xiaomi", "name": "Living Room Camera", "type": "...", "space_name": "Living Room", "rtsp_url": "rtsp://127.0.0.1:8554/xiaomi_cam001"}]}

2. hc_video {"commandJson":"{\"method\":\"capImage\",\"params\":{\"rtsp_url\":\"rtsp://127.0.0.1:8554/xiaomi_cam001\"}}"}
   → {"file_path": "/tmp/homeclaw_frame_123.jpg"}
```

### Example 7: Set Xiaomi Fan Speed

```
1. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"listDevices\"}"}
   → from_id="67890", from="xiaomi", name="Bedroom Fan"

2. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"getSpec\",\"params\":{\"deviceId\":\"67890\"}}"}
   → [{"desc":"Fan Control-Fan Level","method":"SetProp","param":{"did":"","siid":2,"piid":2,"value":"$value$"},"param_desc":"int 1-4"}]

3. hc_cli {"commandJson":"{\"brand\":\"xiaomi\",\"method\":\"setProps\",\"params\":{\"did\":\"67890\",\"siid\":2,\"piid\":2,\"value\":3}}"}
   → Fan speed set to level 3
```

---

## Error Handling

- **Device not found**: Ask user for more specific device name or room; if multiple match, list candidates and ask user to confirm
- **Device offline**: Inform user the device is offline, do not proceed
- **Brand not registered**: Credentials not configured, inform user to run device-sync first
- **Auth error / token invalid**: For Xiaomi, ask user to re-login via web UI; for Tuya, reconfigure API key
- **Spec not available**: Device may not support cloud control
- **Property not supported**: Use getProps to inspect available properties first
- **Camera RTSP failed**: Camera may be offline or go2rtc not running; check prerequisites
- **FFmpeg not available**: FFmpeg must be installed for camera frame capture
- **Invalid method**: Use valid methods: `listDevices`, `listCameras`, `getSpec`, `getProps`, `setProps`, `execute`

## Prerequisites for Camera Capture

- Devices must be synced first (use `device-sync` skill)
- go2rtc must be running to serve RTSP streams
- FFmpeg must be installed
- Vision-capable LLM must be configured (for `capAnalyze` method)
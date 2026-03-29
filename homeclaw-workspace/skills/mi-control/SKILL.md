---
name: mi-control
description: Control Xiaomi/Mi Home smart devices. Use when the user wants to control, operate, or interact with Xiaomi smart devices (lights, fans, air purifiers, switches, etc.). Triggers on commands like "turn on the living room light", "set fan speed to 3", "start vacuum cleaning", or any request to operate Mi Home devices.
---

# Mi Control - Control Xiaomi Smart Devices

Control Xiaomi/Mi Home smart devices through the cloud API using the method:
- **`mi_execute_action`** 


## Workflow

1. **Find Device** → `hc_list_devices`
2. **Get Device Spec** → `mi_get_spec_commands` (find one command)
3. **Execute Command** → `mi_execute_action`

## Step 1: Find Device

```
hc_list_devices
```

Returns device list with `from_id`, `from`,  `name`,  `urn`

filter from=xiaomi ，find the right device

## Step 2: Get Device Spec

```
mi_get_spec_commands
- urn: the device URN from step 1
```


Returns MIoT json：

```
[
  {
    "desc": "Camera Control-Switch Status",
    "method": "SetProp",
    "param": {
      "did": "",
      "siid": 2,
      "piid": 1,
      "value": "$value$"
    },
    "param_desc": "bool"
  },
  {
    "desc": "P2P Stream-Start P2P Stream",
    "method": "Action",
    "param": {
      "did": "",
      "siid": 3,
      "aiid": 1,
      "in": []
    },
    "param_desc": ""
  },
  {
    "desc": "P2P Stream-Stop Camera Stream",
    "method": "Action",
    "param": {
      "did": "",
      "siid": 3,
      "aiid": 2,
      "in": []
    },
    "param_desc": ""
  }
]

```

please determine only one actions，if param contains  "value": "$value$"，change $value$ to right value by param_desc and user request


## Step 3: Execute Command

Use for triggering predefined multi-step operations.

```
mi_execute_action
-   {
    "desc": "Camera Control-Switch Status",
    "method": "SetProp",
    "param": {
      "did": "",
      "siid": 2,
      "piid": 1,
      "value": "true"
    },
    "param_desc": "bool"
  }
```

## Examples

### Example 1: Turn On a Light

```
1. hc_list_devices
   → from_id="12345", from="xiaomi", name="Living Room Light", urn="urn:miot-spec-v2:device:light:..."

2. mi_get_spec_commands {"urn": "urn:miot-spec-v2:device:light:..."}
   → [{"desc": "Light Control-Switch Status", "method": "SetProp", "param": {"did": "", "siid": 2, "piid": 1, "value": "$value$"}, "param_desc": "bool"}]

3. mi_execute_action {"desc": "Light Control-Switch Status", "method": "SetProp", "param": {"did": "12345", "siid": 2, "piid": 1, "value": true}, "param_desc": "bool"}
   → Light turned on
```

### Example 2: Set Fan Speed

```
1. hc_list_devices
   → from_id="67890", from="xiaomi", name="Bedroom Fan", urn="urn:miot-spec-v2:device:fan:..."

2. mi_get_spec_commands {"urn": "urn:miot-spec-v2:device:fan:..."}
   → [{"desc": "Fan Control-Fan Level", "method": "SetProp", "param": {"did": "", "siid": 2, "piid": 2, "value": "$value$"}, "param_desc": "int 1-4"}]

3. mi_execute_action {"desc": "Fan Control-Fan Level", "method": "SetProp", "param": {"did": "67890", "siid": 2, "piid": 2, "value": 3}, "param_desc": "int 1-4"}
   → Fan speed set to level 3
```

### Example 3: Start Robot Vacuum Cleaning

```
1. hc_list_devices
   → from_id="vacuum123", from="xiaomi", name="Robot Vacuum", urn="urn:miot-spec-v2:device:vacuum:..."

2. mi_get_spec_commands {"urn": "urn:miot-spec-v2:device:vacuum:..."}
   → [{"desc": "Vacuum Control-Start Sweep", "method": "Action", "param": {"did": "", "siid": 3, "aiid": 1, "in": []}, "param_desc": ""}]

3. mi_execute_action {"desc": "Vacuum Control-Start Sweep", "method": "Action", "param": {"did": "vacuum123", "siid": 3, "aiid": 1, "in": []}, "param_desc": ""}
   → Vacuum starts cleaning
```

### Example 4: Stop Vacuum and Return to Dock

```
1. hc_list_devices
   → from_id="vacuum123", from="xiaomi", name="Robot Vacuum", urn="urn:miot-spec-v2:device:vacuum:..."

2. mi_get_spec_commands {"urn": "urn:miot-spec-v2:device:vacuum:..."}
   → [{"desc": "Vacuum Control-Stop and Charge", "method": "Action", "param": {"did": "", "siid": 3, "aiid": 2, "in": []}, "param_desc": ""}]

3. mi_execute_action {"desc": "Vacuum Control-Stop and Charge", "method": "Action", "param": {"did": "vacuum123", "siid": 3, "aiid": 2, "in": []}, "param_desc": ""}
   → Vacuum stops and returns to charging dock
```


## Error Handling

- **Device not found**: Ask user for more specific device name/room
- **Spec parse error**: Device may not support cloud control
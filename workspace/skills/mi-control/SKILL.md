---
name: mi-control
description: Control Xiaomi/Mi Home smart devices. Use when the user wants to control, operate, or interact with Xiaomi smart devices (lights, fans, air purifiers, switches, etc.).
---

# Mi Control

## Workflow

### Step 1 — Find device and action

Call `hc_list_devices` to get all devices.

Each device has:
- `from_id`: device ID (did)
- `name`: device name
- `room_name`: room location
- `actions`: pre-generated action mappings

Example device response:
```json
{
  "from_id": "12345",
  "name": "Living Room Light",
  "room_name": "Living Room",
  "actions": {
    "turn_on": "{\"method\":\"SetProp\",\"param\":{\"did\":\"12345\",\"siid\":2,\"piid\":1,\"value\":true}}",
    "turn_off": "{\"method\":\"SetProp\",\"param\":{\"did\":\"12345\",\"siid\":2,\"piid\":1,\"value\":false}}",
    "get_state": "{\"method\":\"GetProp\",\"param\":{\"did\":\"12345\",\"siid\":2,\"piid\":1}}"
  }
}
```

### Step 2 — Execute action

Match user intent to an action key (e.g., "turn on" → `turn_on`).

Call `mi_execute_action` with the action value directly:

```
mi_execute_action
- actionJson: the JSON string from device.actions[action_key]
```

## Examples

### Turn on living room light

```
1. hc_list_devices
   → Find device with name containing "light" in "living room"
   → actions.turn_on = {"method":"SetProp","param":{"did":"12345","siid":2,"piid":1,"value":true}}

2. mi_execute_action {"actionJson": "{\"method\":\"SetProp\",\"param\":{\"did\":\"12345\",\"siid\":2,\"piid\":1,\"value\":true}}"}
   → Light turned on
```

### Start robot vacuum

```
1. hc_list_devices
   → Find vacuum device
   → actions.start = {"method":"Action","param":{"did":"67890","siid":3,"aiid":1,"in":[]}}

2. mi_execute_action {"actionJson": "{\"method\":\"Action\",\"param\":{\"did\":\"67890\",\"siid\":3,\"aiid\":1,\"in\":[]}}"}
   → Vacuum starts cleaning
```

## Common Action Keys

| User Intent | Action Key |
|-------------|------------|
| Turn on | `turn_on` |
| Turn off | `turn_off` |
| Get status | `get_state` |
| Start | `start` |
| Stop | `stop` |
| Pause | `pause` |

## Error Handling

| Situation | Action |
|-----------|--------|
| Device not found | Ask user for device name/room |
| Action not in device.actions | Device may not support this operation |
| Execute failed | Report error to user |
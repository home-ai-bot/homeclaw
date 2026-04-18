# Xiaomi MIoT Parsing Rules

This document contains the parsing rules for Xiaomi MIoT device specifications.

## Spec Structure

Xiaomi MIoT specs are returned as a flat array of operation definitions:

```json
[
  {
    "desc": "Light Control-Switch Status",
    "method": "SetProp",
    "param": {
      "did": "",
      "siid": 2,
      "piid": 1,
      "value": "$value$",
      "param_desc": "bool"
    },

  }
]
```

## Parsing Rules

For each JSON object in the input spec array, parse it according to these rules:

1. **method**: Extract directly from `specjson.method`
2. **ops**: Use the `specjson.method` and `specjson.desc` to choose one matching operation from `./ops.md`
3. **param**: Extract from `specjson.param` with the following modifications:
   - Change `did` value to `<from_id>` (the target device ID)
   - Change `value` to the appropriate value based on `param_desc`
   - If `value` is not a single fixed value, keep it as `"$value$"`

**Return Format**: An array of JSON objects, where each object contains three fields: `method`, `ops`, and `param`

```json
[
  {
    "method": "<method>",
    "ops": "<operation_name>",
    "param": { ... }
  }
]
```

## Example Parsing

Input spec:
```json
[
  {"desc": "Light Control-Switch Status", "method": "SetProp", "param": {"did": "", "siid": 2, "piid": 1, "value": "$value$","param_desc": "bool"}},
  {"desc": "Light Control-Brightness", "method": "SetProp", "param": {"did": "", "siid": 2, "piid": 2, "value": "$value$", "param_desc": "int 0-100"}},
  {"desc": "Light Control-Get Status", "method": "GetProp", "param": {"did": "", "siid": 2, "piid": 1,"param_desc": ""} }
]
```

Parsed operations for device "12345":
```json
[
  {
    "method": "SetProp",
    "ops": "turn_on",
    "param": {"did": "12345", "siid": 2, "piid": 1, "value": true, "param_desc": "bool"}
  },
  {
    "method": "SetProp",
    "ops": "turn_off",
    "param": {"did": "12345", "siid": 2, "piid": 1, "value": false, "param_desc": "bool"}
  },
  {
    "method": "SetProp",
    "ops": "set_brightness",
    "param": {"did": "12345", "siid": 2, "piid": 2, "value": "$value$", "param_desc": "int 0-100"}
  },
  {
    "method": "GetProp",
    "ops": "get_brightness",
    "param": {"did": "12345", "siid": 2, "piid": 2}
  },
  {
    "method": "GetProp",
    "ops": "get_state",
    "param": {"did": "12345", "siid": 2, "piid": 1}
  }
]
```

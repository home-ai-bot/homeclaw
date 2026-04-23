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

1. **method**: Extract directly from `method`
2. **ops**: Use the `method` and `desc` to choose one matching operation from ## Supported Operations Reference:
3. **param**: Extract from `param` with the following modifications:
   - Change `did` value to `<from_id>` (the target device ID)
   - Change `value` to the appropriate value based on `param_desc`
   - If `value` is not a single fixed value, keep it as `"$value$"`

**Return Format**: An array of JSON objects, where each object contains three fields: `method`, `ops`, and `param`

## output example
[
 {
    "method": "SetProp",
    "ops": "turn_on",
    "param": {"did": "12345", "siid": 2, "piid": 1, "value": true, "param_desc": "bool"}
  },
  {
    "method": "SetProp",
    "ops": "set_brightness",
    "param": {"did": "12345", "siid": 2, "piid": 2, "value": "$value$", "param_desc": "int 0-100"}
  }
]

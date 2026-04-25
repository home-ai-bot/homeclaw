# Xiaomi MIoT

## Spec: services[].properties[] + services[].actions[]
根据总体spec判断设备类型，只保留核心操作，忽略次要功能
Skip: service.type含"device-information"|"battery"|read-only属性
Per property: siid←service.iid, piid←property.iid
Per action: siid←service.iid, aiid←action.iid

## Output JSON (no comments)
```json
[{"ops":"turn","param_type":"bool","param_value":null,"method":"SetProp","method_param":{"did":"{{.deviceId}}","siid":2,"piid":1,"value":"{{.value}}"}},{"ops":"start","param_type":"in","param_value":"[2]","method":"execute","method_param":{"did":"{{.deviceId}}","siid":2,"aiid":1,"in":["{{.value}}"]}}]
```

## Field Source
| Field | Source | Rule |
|-------|--------|------|
| ops | service.description + property/action的type URN + description推断 bool类型turn、lock等 | 必须在ops.md中 |
| param_type | property→format; action→"in" | bool/int/enum/string/in |
| param_value | by param_type | bool→null, int→"min-max"(value-range), enum→{"1":"desc"}(value-list的value:desc), string→null, in→action.in数组(如[2]) |
| method | property含write→SetProp; action→execute | |
| method_param | golang template, 仅{{.deviceId}}和{{.value}}为变量, siid/piid/aiid填spec实际值 | → templates below |

## method_param Template (golang, 仅2变量: {{.deviceId}}, {{.value}})
SetProp: `{"did":"{{.deviceId}}","siid":<siid>,"piid":<piid>,"value":"{{.value}}"}`
execute: `{"did":"{{.deviceId}}","siid":<siid>,"aiid":<aiid>,"in":["{{.value}}"]}`
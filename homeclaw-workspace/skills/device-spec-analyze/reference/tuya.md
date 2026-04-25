# Tuya Thing Model


## Spec: services[].properties[]
根据总体spec判断设备类型，只保留核心操作，忽略次要功能
Skip: accessMode=ro的属性
Per property: code, name, description, accessMode(rw|wr), typeSpec{type,min,max,step,range,maxlen}

## Output JSON (no comments)
```json
[{"ops":"turn","param_type":"bool","param_value":null,"method":"setProps","method_param":{"device_id":"{{.deviceId}}","switch_1":"{{.value}}"}}]
```

## Field Source
| Field | Source | Rule |
|-------|--------|------|
| ops | property.code + property.name + property.description推断,不是常见操作先 忽略 | 必须在ops.md中|
| param_type | typeSpec.type | bool→bool, value→int, enum→enum, string→string |
| param_value | by param_type | bool→null, value→"0-max"(无min时0补), enum→range数组, string→null |
| method | rw/wr→setProps | |
| method_param | golang template, 仅{{.deviceId}}和{{.value}}为变量, code填spec实际值 | → templates below |

## method_param Template (golang, 仅2变量: {{.deviceId}}, {{.value}})
setProps: `{"device_id":"{{.deviceId}}","<code>":"{{.value}}"}`

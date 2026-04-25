import {
  IconLoader2,
  IconWand,
} from "@tabler/icons-react"
import { useStore } from "jotai"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Badge } from "@/components/ui/badge"
import { Switch } from "@/components/ui/switch"
import { Slider } from "@/components/ui/slider"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  deviceOpsAtom,
} from "@/homeclaw/store/device-ops"
import { callTool } from "@/homeclaw/api/device-command-executor"
import { SmartHomeLayout } from "@/homeclaw/components/smart-home-layout"

// ── Types ──────────────────────────────────────────────────────────────────

/**
 * DeviceOp matches the backend structure from pkg/homeclaw/data/types.go
 */
export interface DeviceOp {
  urn: string
  from: string
  ops: string
  param_type: "bool" | "int" | "enum" | "string" | "in"
  param_value: any  // null, true/false, "min-max", {"1":"desc"}, or array
  method: string
  method_param: string  // Go template JSON string
}

interface DeviceWithOps {
  from_id: string
  from: string
  name: string
  type: string
  urn: string
  space_name: string
  ops: DeviceOp[]
}

interface RoomGroup {
  room_name: string
  devices: DeviceWithOps[]
}

interface ListOpsResponse {
  success: boolean
  rooms: RoomGroup[]
  count: number
  message?: string
}

// ── Helpers ────────────────────────────────────────────────────────────────

/**
 * Fetch all devices with operations via WebSocket listOps method.
 */
async function fetchDevicesWithOpsViaWS(): Promise<RoomGroup[]> {
  try {
    const result = await callTool(
      {
        toolName: "hc_cli",
        method: "listOps",
        brand: "",
        params: {},
      },
      {
        timeout: 30000,
      }
    )

    if (!result.success || !result.message) {
      throw new Error(result.error || "Failed to fetch devices")
    }

    // Parse the JSON response from the tool
    const response: ListOpsResponse = JSON.parse(result.message)
    return response.rooms || []
  } catch (error) {
    console.error("Failed to fetch devices with ops:", error)
    throw error
  }
}

/**
 * Parse the param_value string "min-max" into [min, max] numbers.
 */
function parseRange(paramValue: any): [number, number] {
  if (typeof paramValue === "string") {
    const parts = paramValue.split("-").map(Number)
    if (parts.length === 2 && !isNaN(parts[0]) && !isNaN(parts[1])) {
      return [parts[0], parts[1]]
    }
  }
  return [0, 100]
}

/**
 * Parse enum param_value JSON object into { value, label } pairs.
 */
function parseEnumOptions(paramValue: any): Array<{ value: string; label: string }> {
  if (typeof paramValue === "object" && paramValue !== null) {
    return Object.entries(paramValue).map(([value, label]) => ({
      value,
      label: String(label),
    }))
  }
  return []
}

// ── Control Components ─────────────────────────────────────────────────────

interface ControlProps {
  op: DeviceOp
  fromId: string
  from: string
  deviceName: string
}

/**
 * Bool control: renders a toggle switch.
 * Combines turn_on/turn_off ops into a single switch.
 */
function BoolControl({ op, fromId, from, deviceName }: ControlProps) {
  const [isOn, setIsOn] = useState(op.param_value === true)
  const [loading, setLoading] = useState(false)

  const handleToggle = async (checked: boolean) => {
    setLoading(true)
    setIsOn(checked)
    try {
      await callTool(
        {
          toolName: "hc_cli",
          method: "exe",
          brand: from,
          params: {
            from_id: fromId,
            from,
            ops: op.ops,
            value: checked,
          },
        },
        {
          timeout: 60000,
          successMessage: `${deviceName} ${checked ? "已开启" : "已关闭"}`,
        }
      )
    } catch (error) {
      console.error("Failed to execute bool op:", error)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="flex items-center justify-between gap-3">
      <Label className="text-sm">{op.ops}</Label>
      <Switch
        checked={isOn}
        onCheckedChange={handleToggle}
        disabled={loading}
      />
    </div>
  )
}

/**
 * Int control: renders a slider with min-max range.
 */
function IntControl({ op, fromId, from, deviceName }: ControlProps) {
  const [min, max] = parseRange(op.param_value)
  const [value, setValue] = useState<number>(min)
  const [loading, setLoading] = useState(false)

  const handleCommit = async (vals: number[]) => {
    const newVal = vals[0]
    setValue(newVal)
    setLoading(true)
    try {
      await callTool(
        {
          toolName: "hc_cli",
          method: "exe",
          brand: from,
          params: {
            from_id: fromId,
            from,
            ops: op.ops,
            value: newVal,
          },
        },
        {
          timeout: 60000,
          successMessage: `${deviceName} ${op.ops} → ${newVal}`,
        }
      )
    } catch (error) {
      console.error("Failed to execute int op:", error)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <Label className="text-sm">{op.ops}</Label>
        <span className="text-xs text-muted-foreground">{value}</span>
      </div>
      <Slider
        min={min}
        max={max}
        step={1}
        value={[value]}
        onValueChange={handleCommit}
        disabled={loading}
      />
      <div className="flex justify-between text-xs text-muted-foreground">
        <span>{min}</span>
        <span>{max}</span>
      </div>
    </div>
  )
}

/**
 * Enum control: renders a dropdown select.
 */
function EnumControl({ op, fromId, from, deviceName }: ControlProps) {
  const options = parseEnumOptions(op.param_value)
  const [value, setValue] = useState<string>(options[0]?.value ?? "")
  const [loading, setLoading] = useState(false)

  const handleChange = async (newVal: string) => {
    setValue(newVal)
    setLoading(true)
    try {
      await callTool(
        {
          toolName: "hc_cli",
          method: "exe",
          brand: from,
          params: {
            from_id: fromId,
            from,
            ops: op.ops,
            value: newVal,
          },
        },
        {
          timeout: 60000,
          successMessage: `${deviceName} ${op.ops} → ${options.find(o => o.value === newVal)?.label ?? newVal}`,
        }
      )
    } catch (error) {
      console.error("Failed to execute enum op:", error)
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="space-y-2">
      <Label className="text-sm">{op.ops}</Label>
      <Select
        value={value}
        onValueChange={handleChange}
        disabled={loading}
      >
        <SelectTrigger>
          <SelectValue placeholder="Select..." />
        </SelectTrigger>
        <SelectContent>
          {options.map((opt) => (
            <SelectItem key={opt.value} value={opt.value}>
              {opt.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  )
}

/**
 * String control: renders a text input with a submit button.
 */
function StringControl({ op, fromId, from, deviceName }: ControlProps) {
  const [value, setValue] = useState("")
  const [loading, setLoading] = useState(false)

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault()
    if (!value.trim()) return
    setLoading(true)
    try {
      await callTool(
        {
          toolName: "hc_cli",
          method: "exe",
          brand: from,
          params: {
            from_id: fromId,
            from,
            ops: op.ops,
            value: value.trim(),
          },
        },
        {
          timeout: 60000,
          successMessage: `${deviceName} ${op.ops} → ${value.trim()}`,
        }
      )
      setValue("")
    } catch (error) {
      console.error("Failed to execute string op:", error)
    } finally {
      setLoading(false)
    }
  }

  return (
    <form onSubmit={handleSubmit} className="space-y-2">
      <Label className="text-sm">{op.ops}</Label>
      <div className="flex gap-2">
        <Input
          value={value}
          onChange={(e) => setValue(e.target.value)}
          placeholder="Enter value..."
          disabled={loading}
        />
        <Button
          type="submit"
          size="sm"
          disabled={loading || !value.trim()}
        >
          {loading ? <IconLoader2 className="size-4 animate-spin" /> : "Send"}
        </Button>
      </div>
    </form>
  )
}

/**
 * Renders the appropriate control based on param_type.
 */
function OpControl(props: ControlProps) {
  const { op } = props
  switch (op.param_type) {
    case "bool":
      return <BoolControl {...props} />
    case "int":
      return <IntControl {...props} />
    case "enum":
      return <EnumControl {...props} />
    case "string":
      return <StringControl {...props} />
    default:
      return (
        <div className="text-xs text-muted-foreground">
          Unsupported param_type: {op.param_type}
        </div>
      )
  }
}

// ── Main page ──────────────────────────────────────────────────────────────

export function DeviceControlPage() {
  const store = useStore()
  const { t } = useTranslation("homeclaw")

  const [state, setState] = useState(store.get(deviceOpsAtom))
  const [rooms, setRooms] = useState<RoomGroup[]>([])
  const [processingDevices, setProcessingDevices] = useState<Set<string>>(new Set())

  // ── Subscribe to store ───────────────────────────────────────────────────

  useEffect(() => {
    const unsub = store.sub(deviceOpsAtom, () => setState(store.get(deviceOpsAtom)))
    return unsub
  }, [store])

  // ── Load devices with ops on mount ────────────────────────────────────────

  useEffect(() => {
    const loadDevices = async () => {
      store.set(deviceOpsAtom, (prev) => ({ ...prev, isLoading: true, error: null }))
      try {
        const fetchedRooms = await fetchDevicesWithOpsViaWS()
        setRooms(fetchedRooms)
        store.set(deviceOpsAtom, (prev) => ({ ...prev, isLoading: false, error: null }))
      } catch (error) {
        const errorMsg = error instanceof Error ? error.message : "Unknown error"
        store.set(deviceOpsAtom, (prev) => ({ ...prev, isLoading: false, error: errorMsg }))
      }
    }
    void loadDevices()
  }, [store])



  // ── Action handlers ──────────────────────────────────────────────────────

  const handleGenerateOps = async (fromId: string, from: string, deviceName: string) => {
    const key = `${fromId}-${from}`
    setProcessingDevices((prev) => new Set(prev).add(key))

    try {
      await callTool(
        {
          toolName: "hc_llm",
          method: "analyzeDeviceOpsAsync",
          brand: from,
          params: {
            brand: from,
            from_id: fromId,
          },
        },
        {
          timeout: 10000,
          successMessage: `${deviceName} 操作分析已启动，请耐心等待分析完成`,
        }
      )
    } catch (error) {
      console.error("Failed to generate ops:", error)
    } finally {
      setProcessingDevices((prev) => {
        const next = new Set(prev)
        next.delete(key)
        return next
      })
    }
  }

  // ── Main render ──────────────────────────────────────────────────────────

  return (
    <SmartHomeLayout
      title={t("device_control")}
      isLoading={state.isLoading}
    >
      <div className="pt-2 space-y-6">
        {rooms.length === 0 && !state.isLoading ? (
          <div className="text-muted-foreground py-12 text-center">
            <p className="text-base">{t("device_control_page.noDevices")}</p>
          </div>
        ) : (
          rooms.map((room) => (
            <div key={room.room_name}>
              <h2 className="text-lg font-semibold mb-3">{room.room_name}</h2>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {room.devices.map((device) => {
                  const deviceKey = `${device.from_id}-${device.from}`

                  return (
                    <Card key={deviceKey}>
                      <CardHeader className="pb-2">
                        <div className="flex items-center justify-between">
                          <CardTitle className="text-base">{device.name}</CardTitle>
                          <Badge variant="outline">{device.type}</Badge>
                        </div>
                        <CardDescription>
                          {device.from} · {device.from_id}
                        </CardDescription>
                      </CardHeader>
                      <CardContent className="space-y-3">
                        {device.ops.map((op, idx) => (
                          <OpControl
                            key={`${op.urn}-${op.ops}-${idx}`}
                            op={op}
                            fromId={device.from_id}
                            from={device.from}
                            deviceName={device.name}
                          />
                        ))}
                        <div className="flex gap-2 pt-2">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={processingDevices.has(deviceKey)}
                            onClick={() =>
                              handleGenerateOps(device.from_id, device.from, device.name)
                            }
                            className="flex items-center gap-1"
                          >
                            {processingDevices.has(deviceKey) ? (
                              <IconLoader2 className="size-3 animate-spin" />
                            ) : (
                              <IconWand className="size-3" />
                            )}
                            <span className="text-xs">{t("device_control_page.generateOps")}</span>
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  )
                })}
              </div>
            </div>
          ))
        )}
      </div>
    </SmartHomeLayout>
  )
}

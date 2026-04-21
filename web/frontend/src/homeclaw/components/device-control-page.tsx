import {
  IconLoader2,
  IconPower,
  IconSettings,
  IconInfoCircle,
  IconBan,
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
import {
  deviceOpsAtom,
  fetchDevicesWithOps,
  type OperationLog,
} from "@/homeclaw/store/device-ops"
import { callTool } from "@/homeclaw/api/device-command-executor"
import { SmartHomeLayout } from "@/homeclaw/components/smart-home-layout"

// ── Helpers ────────────────────────────────────────────────────────────────

const getOpIcon = (opsName: string) => {
  const name = opsName.toLowerCase()
  if (name.includes("turn_on") || name.includes("start") || name.includes("open"))
    return IconPower
  if (name.includes("set") || name.includes("configure")) return IconSettings
  return IconInfoCircle
}

function newLogId() {
  return `log-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
}

// ── Main page ──────────────────────────────────────────────────────────────

export function DeviceControlPage() {
  const store = useStore()
  const { t } = useTranslation("homeclaw")

  const [state, setState] = useState(store.get(deviceOpsAtom))
  const [executingOps, setExecutingOps] = useState<Set<string>>(new Set())
  const [processingDevices, setProcessingDevices] = useState<Set<string>>(new Set())

  // Local log management for device operations
  const appendLog = (entry: OperationLog) => {
    store.set(deviceOpsAtom, (prev) => ({
      ...prev,
      logs: [...prev.logs, entry],
    }))
  }

  const updateLog = (id: string, patch: Partial<OperationLog>) => {
    store.set(deviceOpsAtom, (prev) => ({
      ...prev,
      logs: prev.logs.map((l) => (l.id === id ? { ...l, ...patch } : l)),
    }))
  }

  // ── Subscribe to store ───────────────────────────────────────────────────

  useEffect(() => {
    const unsub = store.sub(deviceOpsAtom, () => setState(store.get(deviceOpsAtom)))
    return unsub
  }, [store])

  // ── Load devices on mount ────────────────────────────────────────────────
  // Always reset to loading state first to discard any stale cached data,
  // so navigating back to this page always triggers a fresh fetch.

  useEffect(() => {
    store.set(deviceOpsAtom, (prev) => ({ ...prev, rooms: [], isLoading: true, error: null }))
    const init = async () => {
      const status = await fetchDevicesWithOps()
      store.set(deviceOpsAtom, (prev) => ({ ...prev, ...status }))
    }
    void init()
  }, [store])

  // ── Action handlers ──────────────────────────────────────────────────────

  const handleExecuteOp = async (
    fromId: string,
    from: string,
    opsName: string,
    deviceName: string,
  ) => {
    const key = `${fromId}-${from}-${opsName}`
    setExecutingOps((prev) => new Set(prev).add(key))

    const logId = newLogId()
    appendLog({
      id: logId,
      timestamp: Date.now(),
      deviceName,
      opsName,
      status: "pending",
      message: "正在执行...",
    })

    try {
      // Use callTool with successMessage for tooltip notification
      const result = await callTool(
        {
          toolName: "hc_cli",
          method: "exe",
          brand: from,
          params: {
            from_id: fromId,
            from: from,
            ops: opsName,
          },
        },
        {
          timeout: 60000,
          successMessage: `${deviceName} ${opsName} 成功`,
        }
      )

      updateLog(logId, {
        status: result.success ? "success" : "failed",
        message: result.success
          ? result.message || "执行成功"
          : result.error || "未知错误",
      })
    } catch (error) {
      updateLog(logId, {
        status: "failed",
        message: error instanceof Error ? error.message : "未知错误",
      })
    } finally {
      setExecutingOps((prev) => {
        const next = new Set(prev)
        next.delete(key)
        return next
      })
    }
  }

  const handleRefresh = async () => {
    store.set(deviceOpsAtom, (prev) => ({ ...prev, isLoading: true, error: null }))
    const status = await fetchDevicesWithOps()
    store.set(deviceOpsAtom, (prev) => ({ ...prev, ...status }))
  }

  const handleMarkAsNoAction = async (fromId: string, from: string, deviceName: string) => {
    const key = `${fromId}-${from}`
    setProcessingDevices((prev) => new Set(prev).add(key))

    const logId = newLogId()
    appendLog({
      id: logId,
      timestamp: Date.now(),
      deviceName,
      opsName: "标记不可操作",
      status: "pending",
      message: "正在处理...",
    })

    try {
      // Use callTool with hc_cli.markNoAction via WebSocket
      const result = await callTool(
        {
          toolName: "hc_cli",
          method: "markNoAction",
          brand: from,
          params: {
            from_id: fromId,
            from: from,
          },
        },
        {
          timeout: 60000,
          successMessage: `${deviceName} 已标记为不可操作`,
        }
      )

      updateLog(logId, {
        status: result.success ? "success" : "failed",
        message: result.success
          ? result.message || "已标记为不可操作"
          : result.error || "未知错误",
      })

      if (result.success) {
        await handleRefresh()
      }
    } catch (error) {
      updateLog(logId, {
        status: "failed",
        message: error instanceof Error ? error.message : "未知错误",
      })
    } finally {
      setProcessingDevices((prev) => {
        const next = new Set(prev)
        next.delete(key)
        return next
      })
    }
  }

  const handleGenerateOps = async (fromId: string, from: string, deviceName: string) => {
    const key = `${fromId}-${from}`
    setProcessingDevices((prev) => new Set(prev).add(key))

    const logId = newLogId()
    appendLog({
      id: logId,
      timestamp: Date.now(),
      deviceName,
      opsName: "生成操作",
      status: "pending",
      message: "正在生成设备操作...",
    })

    try {
      // Call hc_llm analyzeDeviceOpsAsync to generate operations for a single device
      // Async method starts analysis in background and returns immediately
      const result = await callTool(
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
          timeout: 10000, // 10 seconds is enough since it returns immediately
          successMessage: `${deviceName} 操作分析已启动，请耐心等待分析完成`,
        }
      )

      updateLog(logId, {
        status: result.success ? "success" : "failed",
        message: result.success
          ? result.message || "操作分析已启动"
          : result.error || "未知错误",
      })

      // Note: Device list will need to be manually refreshed after analysis completes
      // since the analysis runs in background
    } catch (error) {
      updateLog(logId, {
        status: "failed",
        message: error instanceof Error ? error.message : "未知错误",
      })
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
        {state.rooms.length === 0 ? (
          <div className="text-muted-foreground py-12 text-center">
            <p className="text-base">{t("device_control_page.noDevices")}</p>
          </div>
        ) : (
          state.rooms.map((room) => (
            <div key={room.room_name}>
              <h2 className="text-lg font-semibold mb-3">{room.room_name}</h2>
              <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4">
                {room.devices.map((device) => {
                  const isNoAction =
                    device.ops && device.ops.includes("NoAction")
                  const deviceKey = `${device.from_id}-${device.from}`

                  return (
                    <Card
                      key={deviceKey}
                      className={isNoAction ? "bg-muted/50" : ""}
                    >
                      <CardHeader className="pb-2">
                        <div className="flex items-center justify-between">
                          <CardTitle className="text-base">{device.name}</CardTitle>
                          <Badge variant="outline">{device.type}</Badge>
                        </div>
                        <CardDescription>
                          {device.from} · {device.from_id}
                        </CardDescription>
                      </CardHeader>
                      <CardContent>
                        <div className="flex flex-wrap gap-2">
                          {isNoAction ? (
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
                              <span className="text-xs">生成操作</span>
                            </Button>
                          ) : device.ops && device.ops.length > 0 ? (
                            device.ops.map((op) => {
                              const Icon = getOpIcon(op)
                              const opKey = `${deviceKey}-${op}`
                              const isExecuting = executingOps.has(opKey)

                              return (
                                <Button
                                  key={op}
                                  variant="outline"
                                  size="sm"
                                  disabled={isExecuting}
                                  onClick={() =>
                                    handleExecuteOp(
                                      device.from_id,
                                      device.from,
                                      op,
                                      device.name,
                                    )
                                  }
                                  className="flex items-center gap-1"
                                >
                                  {isExecuting ? (
                                    <IconLoader2 className="size-3 animate-spin" />
                                  ) : (
                                    <Icon className="size-3" />
                                  )}
                                  <span className="text-xs">{op}</span>
                                </Button>
                              )
                            })
                          ) : (
                            <>
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={processingDevices.has(deviceKey)}
                                onClick={() =>
                                  handleMarkAsNoAction(
                                    device.from_id,
                                    device.from,
                                    device.name,
                                  )
                                }
                                className="flex items-center gap-1"
                              >
                                {processingDevices.has(deviceKey) ? (
                                  <IconLoader2 className="size-3 animate-spin" />
                                ) : (
                                  <IconBan className="size-3" />
                                )}
                                <span className="text-xs">不可操作</span>
                              </Button>
                              <Button
                                variant="outline"
                                size="sm"
                                disabled={processingDevices.has(deviceKey)}
                                onClick={() =>
                                  handleGenerateOps(
                                    device.from_id,
                                    device.from,
                                    device.name,
                                  )
                                }
                                className="flex items-center gap-1"
                              >
                                {processingDevices.has(deviceKey) ? (
                                  <IconLoader2 className="size-3 animate-spin" />
                                ) : (
                                  <IconWand className="size-3" />
                                )}
                                <span className="text-xs">生成操作</span>
                              </Button>
                            </>
                          )}
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

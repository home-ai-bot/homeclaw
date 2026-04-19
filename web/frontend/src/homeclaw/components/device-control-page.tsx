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
import { markDeviceAsNoAction } from "@/homeclaw/api/device-ops"
import { useDeviceControl } from "@/homeclaw/context/device-control-context"
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

  // Use shared smart home WebSocket hook
  const {
    sendWebSocketMessage,
  } = useDeviceControl()

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

  const clearLogs = () => {
    store.set(deviceOpsAtom, (prev) => ({ ...prev, logs: [] }))
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
      await markDeviceAsNoAction({ from_id: fromId, from })
      updateLog(logId, { status: "success", message: "已标记为不可操作" })
      await handleRefresh()
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
      message: "已发送请求，等待 Agent 生成...",
    })

    try {
      const messageId = `generate-ops-${Date.now()}`
      const content = `使用device-spec-analyze skill 生成{brand:${from},from_id:${fromId},device_name:${deviceName}}的操作`

      // Fire-and-forget via shared smart home WS.
      // The agent response will arrive as a message.create and be shown in the log.
      sendWebSocketMessage({
        type: "message.send",
        id: messageId,
        session_id: "device-control",
        payload: { content, media: [] },
      })

      updateLog(logId, { status: "success", message: "请求已发送，结果将在日志中显示" })
    } catch (error) {
      updateLog(logId, {
        status: "failed",
        message: error instanceof Error ? error.message : "发送失败",
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

      {/* Device operation log panel (local) */}
      {state.logs.length > 0 && (
        <div className="border-t bg-muted/30 px-4 sm:px-6 pt-2 pb-3 mt-6">
          <div className="flex items-center justify-between mb-1">
            <span className="text-xs font-medium text-muted-foreground">操作日志</span>
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-xs text-muted-foreground"
              onClick={clearLogs}
            >
              清空
            </Button>
          </div>
          <div className="h-36 overflow-y-auto rounded border bg-background px-2">
            {state.logs.map((log) => (
              <div key={log.id} className="flex items-start gap-2 py-1.5 border-b last:border-0 text-xs">
                <span className="font-medium">{log.deviceName}</span>
                <span className="text-muted-foreground mx-1">·</span>
                <span className="text-muted-foreground">{log.opsName}</span>
                {log.message && (
                  <p className="text-muted-foreground mt-0.5 break-all">{log.message}</p>
                )}
              </div>
            ))}
          </div>
        </div>
      )}
    </SmartHomeLayout>
  )
}

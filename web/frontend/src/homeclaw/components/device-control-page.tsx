import {
  IconLoader2,
  IconPower,
  IconSettings,
  IconInfoCircle,
  IconBan,
  IconWand,
  IconWifi,
  IconWifiOff,
  IconAlertCircle,
  IconCircleCheck,
  IconCircleX,
  IconClock,
  IconTrash,
} from "@tabler/icons-react"
import { useStore } from "jotai"
import { useEffect, useRef, useState } from "react"
import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/page-header"
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
import { executeDeviceOperation } from "@/homeclaw/api/device-command-executor"
import { markDeviceAsNoAction } from "@/homeclaw/api/device-ops"
import {
  deviceControlWS,
  type DeviceControlWSStatus,
} from "@/homeclaw/api/device-control-websocket"

// ── Helpers ────────────────────────────────────────────────────────────────

const getOpIcon = (opsName: string) => {
  const name = opsName.toLowerCase()
  if (name.includes("turn_on") || name.includes("start") || name.includes("open"))
    return IconPower
  if (name.includes("set") || name.includes("configure")) return IconSettings
  return IconInfoCircle
}

function formatTime(ts: number): string {
  return new Date(ts).toLocaleTimeString()
}

function newLogId() {
  return `log-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`
}

// ── WS status badge ────────────────────────────────────────────────────────

function WsStatusBadge({ status }: { status: DeviceControlWSStatus }) {
  if (status === "connected") {
    return (
      <Badge variant="outline" className="gap-1 text-green-600 border-green-600">
        <IconWifi className="size-3" />
        已连接
      </Badge>
    )
  }
  if (status === "connecting") {
    return (
      <Badge variant="outline" className="gap-1 text-yellow-600 border-yellow-600">
        <IconLoader2 className="size-3 animate-spin" />
        连接中
      </Badge>
    )
  }
  if (status === "error") {
    return (
      <Badge variant="destructive" className="gap-1">
        <IconAlertCircle className="size-3" />
        连接错误
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="gap-1 text-muted-foreground">
      <IconWifiOff className="size-3" />
      未连接
    </Badge>
  )
}

// ── Log entry row ──────────────────────────────────────────────────────────

function LogEntry({ log }: { log: OperationLog }) {
  const StatusIcon =
    log.status === "success"
      ? IconCircleCheck
      : log.status === "failed"
        ? IconCircleX
        : IconClock

  const statusColor =
    log.status === "success"
      ? "text-green-600"
      : log.status === "failed"
        ? "text-destructive"
        : "text-muted-foreground"

  return (
    <div className="flex items-start gap-2 py-1.5 border-b last:border-0 text-xs">
      <StatusIcon className={`size-3.5 mt-0.5 shrink-0 ${statusColor}`} />
      <div className="min-w-0 flex-1">
        <span className="font-medium">{log.deviceName}</span>
        <span className="text-muted-foreground mx-1">·</span>
        <span className="text-muted-foreground">{log.opsName}</span>
        {log.message && (
          <p className="text-muted-foreground mt-0.5 break-all">{log.message}</p>
        )}
      </div>
      <span className="text-muted-foreground shrink-0">{formatTime(log.timestamp)}</span>
    </div>
  )
}

// ── Main page ──────────────────────────────────────────────────────────────

export function DeviceControlPage() {
  const store = useStore()
  const { t } = useTranslation("homeclaw")

  const [state, setState] = useState(store.get(deviceOpsAtom))
  const [executingOps, setExecutingOps] = useState<Set<string>>(new Set())
  const [processingDevices, setProcessingDevices] = useState<Set<string>>(new Set())
  const logEndRef = useRef<HTMLDivElement>(null)

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

  // ── Dedicated WebSocket lifecycle ────────────────────────────────────────
  // Connect when this page mounts; disconnect when it unmounts.
  // This is completely separate from the chat WebSocket (controller.ts).

  useEffect(() => {
    // Sync WS status into store
    const onStatus = (s: DeviceControlWSStatus) => {
      store.set(deviceOpsAtom, (prev) => ({ ...prev, wsStatus: s }))
    }
    deviceControlWS.onStatus(onStatus)

    // Handle incoming messages: append async agent responses as log entries
    const onMessage = (data: unknown) => {
      const d = data as Record<string, unknown>
      const payload = d.payload as Record<string, unknown> | undefined
      if (
        d.type === "message.create" &&
        typeof payload?.content === "string" &&
        payload.content
      ) {
        appendLog({
          id: newLogId(),
          timestamp: Date.now(),
          deviceName: "",
          opsName: "Agent 回复",
          status: "success",
          message: payload.content as string,
        })
      }
    }
    deviceControlWS.onMessage(onMessage)

    void deviceControlWS.connect()

    return () => {
      deviceControlWS.offStatus(onStatus)
      deviceControlWS.offMessage(onMessage)
      deviceControlWS.disconnect()
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // ── Auto-scroll log to bottom ────────────────────────────────────────────

  useEffect(() => {
    logEndRef.current?.scrollIntoView({ behavior: "smooth" })
  }, [state.logs])

  // ── Log helpers ──────────────────────────────────────────────────────────

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
      const result = await executeDeviceOperation(fromId, from, opsName, 60000)
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

      // Fire-and-forget via dedicated device-control WS.
      // The agent response will arrive as a message.create and be shown in the log.
      deviceControlWS.sendMessage({
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

  // ── Loading / error / empty states ──────────────────────────────────────

  if (state.isLoading) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader title={t("device_control")} />
        <div className="flex items-center justify-center flex-1">
          <div className="text-muted-foreground flex items-center gap-2 text-sm">
            <IconLoader2 className="size-4 animate-spin" />
            加载中...
          </div>
        </div>
      </div>
    )
  }

  if (state.error) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader title={t("device_control")} />
        <div className="flex items-center justify-center flex-1">
          <Card className="max-w-md">
            <CardHeader>
              <CardTitle className="text-destructive">加载失败</CardTitle>
              <CardDescription>{state.error}</CardDescription>
            </CardHeader>
            <CardContent>
              <Button onClick={handleRefresh}>重试</Button>
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  if (state.rooms.length === 0) {
    return (
      <div className="flex h-full flex-col">
        <PageHeader title={t("device_control")} />
        <div className="flex items-center justify-center flex-1">
          <Card className="max-w-md">
            <CardHeader>
              <CardTitle>暂无设备</CardTitle>
              <CardDescription className="space-y-2">
                <p>请先在对应的智能家居平台中同步设备</p>
                <p className="text-xs text-muted-foreground">
                  支持的平台：小米、HomeKit、涂鸦等
                </p>
              </CardDescription>
            </CardHeader>
            <CardContent>
              <Button onClick={handleRefresh} variant="outline">
                刷新设备列表
              </Button>
            </CardContent>
          </Card>
        </div>
      </div>
    )
  }

  // ── Main render ──────────────────────────────────────────────────────────

  return (
    <div className="flex h-full flex-col">
      {/* Header row with WS status badge */}
      <div className="flex items-center gap-3 px-4 sm:px-6 pt-4 pb-2">
        <h1 className="text-xl font-semibold flex-1">{t("device_control")}</h1>
        <WsStatusBadge status={state.wsStatus} />
        <Button variant="ghost" size="sm" onClick={handleRefresh}>
          刷新
        </Button>
      </div>

      {/* Device list (scrollable) */}
      <div className="min-h-0 flex-1 overflow-y-auto px-4 sm:px-6">
        <div className="py-2 space-y-6">
          {state.rooms.map((room) => (
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
          ))}
        </div>
      </div>

      {/* Async operation log panel */}
      <div className="border-t bg-muted/30 px-4 sm:px-6 pt-2 pb-3 shrink-0">
        <div className="flex items-center justify-between mb-1">
          <span className="text-xs font-medium text-muted-foreground">操作日志</span>
          {state.logs.length > 0 && (
            <Button
              variant="ghost"
              size="sm"
              className="h-6 px-2 text-xs text-muted-foreground"
              onClick={clearLogs}
            >
              <IconTrash className="size-3 mr-1" />
              清空
            </Button>
          )}
        </div>
        <div className="h-36 overflow-y-auto rounded border bg-background px-2">
          {state.logs.length === 0 ? (
            <div className="flex h-full items-center justify-center text-xs text-muted-foreground">
              暂无日志，执行操作后结果会显示在这里
            </div>
          ) : (
            <>
              {state.logs.map((log) => (
                <LogEntry key={log.id} log={log} />
              ))}
              <div ref={logEndRef} />
            </>
          )}
        </div>
      </div>
    </div>
  )
}

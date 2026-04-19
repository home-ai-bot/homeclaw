import {
  IconLoader2,
  IconMessage2,
  IconWifi,
  IconWifiOff,
  IconAlertCircle,
  IconTrash,
  IconChevronRight,
  IconSend,
  IconDownload,
  IconAlertTriangle,
  IconTool,
} from "@tabler/icons-react"
import { useEffect, useRef, useState, useCallback } from "react"
import { createPortal } from "react-dom"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import {
  deviceControlWS,
  type DeviceControlWSStatus,
} from "@/homeclaw/api/device-control-websocket"
import { callTool, type ToolCallParams, type ToolCallOptions } from "@/homeclaw/api/device-command-executor"

export interface CommunicationLog {
  id: string
  timestamp: number
  type: "send" | "receive" | "error" | "tool"
  title: string
  content?: string
  metadata?: {
    toolName?: string
    method?: string
    brand?: string
  }
}

/**
 * Global WebSocket manager for smart home device control.
 * This component should be mounted once at the app level (e.g., in AppHeader)
 * to maintain a persistent connection across page navigation.
 */
export function useGlobalDeviceControlWS() {
  const [wsStatus, setWsStatus] = useState<DeviceControlWSStatus>("disconnected")
  const [logs, setLogs] = useState<CommunicationLog[]>([])
  const [showLogPanel, setShowLogPanel] = useState(false)
  const logContainerRef = useRef<HTMLDivElement>(null)

  // Auto-scroll logs to bottom
  useEffect(() => {
    if (logContainerRef.current) {
      logContainerRef.current.scrollTop = logContainerRef.current.scrollHeight
    }
  }, [logs])

  // Manage WebSocket lifecycle - connect once and persist
  useEffect(() => {
    const onStatus = (status: DeviceControlWSStatus) => {
      setWsStatus(status)
    }

    const onMessage = (data: unknown) => {
      const d = data as Record<string, unknown>
      const payload = d.payload as Record<string, unknown> | undefined

      // Only log final responses, not intermediate messages
      if (d.type === "message.create" && typeof payload?.content === "string") {
        // Check if this is a tool response
        if (payload.tool_call) {
          const toolCall = payload.tool_call as Record<string, unknown>
          const isError = payload.is_error === true
          
          addLog({
            type: isError ? "error" : "receive",
            title: isError 
              ? `工具错误: ${toolCall.tool_name || "unknown"}` 
              : `工具响应: ${toolCall.tool_name || "unknown"}`,
            content: payload.content,
            metadata: {
              toolName: toolCall.tool_name as string,
              method: toolCall.method as string,
            },
          })
        } else {
          // Regular message response
          addLog({
            type: "receive",
            title: "Agent 回复",
            content: payload.content,
          })
        }
      }
      // Ignore other message types to reduce noise
    }

    deviceControlWS.onStatus(onStatus)
    deviceControlWS.onMessage(onMessage)
    void deviceControlWS.connect()

    return () => {
      deviceControlWS.offStatus(onStatus)
      deviceControlWS.offMessage(onMessage)
      // Don't disconnect on unmount - keep connection alive
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const addLog = useCallback((log: Omit<CommunicationLog, "id" | "timestamp">) => {
    const newLog: CommunicationLog = {
      ...log,
      id: `log-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      timestamp: Date.now(),
    }
    setLogs((prev) => [...prev, newLog])
  }, [])

  const sendWebSocketMessage = useCallback((message: unknown) => {
    try {
      deviceControlWS.sendMessage(message)
      const d = message as Record<string, unknown>
      addLog({
        type: "send",
        title: String(d.type || "发送消息"),
        content: JSON.stringify(message, null, 2),
      })
    } catch (error) {
      addLog({
        type: "error",
        title: "发送失败",
        content: error instanceof Error ? error.message : "未知错误",
      })
      throw error
    }
  }, [addLog])

  const executeToolCall = useCallback(async (
    params: ToolCallParams,
    options: ToolCallOptions = {},
  ) => {
    const { toolName, method, brand } = params
    const logTitle = options.logTitle || `${toolName}.${method}`

    // Log the request
    addLog({
      type: "tool",
      title: `工具调用: ${logTitle}`,
      content: JSON.stringify({ brand, method, params: params.params }, null, 2),
      metadata: { toolName, method, brand },
    })

    const result = await callTool(params, options)

    if (!result.success) {
      addLog({
        type: "error",
        title: `工具调用失败: ${logTitle}`,
        content: result.error,
        metadata: { toolName, method, brand },
      })
    }

    return result
  }, [addLog])

  const clearLogs = useCallback(() => {
    setLogs([])
  }, [])

  const toggleLogPanel = useCallback(() => {
    setShowLogPanel((prev) => !prev)
  }, [])

  return {
    wsStatus,
    logs,
    showLogPanel,
    logContainerRef,
    sendWebSocketMessage,
    executeToolCall,
    clearLogs,
    toggleLogPanel,
    setShowLogPanel,
    addLog,
  }
}

// ── WS Status Badge ────────────────────────────────────────────────────────

function WsStatusBadge({ status }: { status: DeviceControlWSStatus }) {
  const { t } = useTranslation("homeclaw")

  if (status === "connected") {
    return (
      <Badge variant="outline" className="gap-1 text-green-600 border-green-600">
        <IconWifi className="size-3" />
        {t("smart_home.ws.connected")}
      </Badge>
    )
  }
  if (status === "connecting") {
    return (
      <Badge variant="outline" className="gap-1 text-yellow-600 border-yellow-600">
        <IconLoader2 className="size-3 animate-spin" />
        {t("smart_home.ws.connecting")}
      </Badge>
    )
  }
  if (status === "error") {
    return (
      <Badge variant="destructive" className="gap-1">
        <IconAlertCircle className="size-3" />
        {t("smart_home.ws.error")}
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="gap-1 text-muted-foreground">
      <IconWifiOff className="size-3" />
      {t("smart_home.ws.disconnected")}
    </Badge>
  )
}

// ── Log Entry ──────────────────────────────────────────────────────────────

function LogEntry({ log }: { log: CommunicationLog }) {
  const formatTime = (timestamp: number) => {
    return new Date(timestamp).toLocaleTimeString()
  }

  const TypeIcon =
    log.type === "send"
      ? IconSend
      : log.type === "receive"
        ? IconDownload
        : log.type === "tool"
          ? IconTool
          : IconAlertTriangle

  const typeColor =
    log.type === "send"
      ? "text-blue-600"
      : log.type === "receive"
        ? "text-green-600"
        : log.type === "tool"
          ? "text-purple-600"
          : log.title.includes("失败") || log.title.includes("错误")
            ? "text-destructive"
            : "text-yellow-600"

  return (
    <div className="flex items-start gap-2 py-1.5 border-b last:border-0 text-xs">
      <TypeIcon className={`size-3.5 mt-0.5 shrink-0 ${typeColor}`} />
      <div className="min-w-0 flex-1">
        <div className="font-medium">{log.title}</div>
        {log.content && (
          <pre className="text-muted-foreground mt-1 text-[11px] whitespace-pre-wrap break-words font-mono bg-muted/30 rounded px-2 py-1.5 max-h-48 overflow-y-auto">
            {log.content}
          </pre>
        )}
      </div>
      <span className="text-muted-foreground shrink-0 text-[10px]">{formatTime(log.timestamp)}</span>
    </div>
  )
}

// ── Main Control Panel Component ───────────────────────────────────────────

interface DeviceControlPanelProps {
  wsStatus: DeviceControlWSStatus
  logs: CommunicationLog[]
  showLogPanel: boolean
  logContainerRef: React.RefObject<HTMLDivElement | null>
  onToggleLogPanel: () => void
  onClearLogs: () => void
}

export function DeviceControlPanel({
  wsStatus,
  logs,
  showLogPanel,
  logContainerRef,
  onToggleLogPanel,
  onClearLogs,
}: DeviceControlPanelProps) {
  return (
    <>
      {/* WS Status Badge */}
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <WsStatusBadge status={wsStatus} />
          </TooltipTrigger>
          <TooltipContent>
            <p>WebSocket 连接状态</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>

      {/* Log Panel Toggle Button */}
      <TooltipProvider>
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant={showLogPanel ? "default" : "ghost"}
              size="sm"
              onClick={onToggleLogPanel}
              className="relative h-8 w-8 p-0"
            >
              <IconMessage2 className="size-4" />
              {logs.length > 0 && (
                <span className="absolute -top-1 -right-1 size-4 bg-primary rounded-full text-white text-[10px] flex items-center justify-center">
                  {logs.length > 9 ? "9+" : logs.length}
                </span>
              )}
            </Button>
          </TooltipTrigger>
          <TooltipContent>
            <p>通信记录 ({logs.length})</p>
          </TooltipContent>
        </Tooltip>
      </TooltipProvider>

      {/* Right side log panel - use portal to escape header's backdrop-filter stacking context */}
      {showLogPanel && createPortal(
        <div className="fixed right-0 top-14 bottom-0 w-80 border-l bg-background flex flex-col shadow-lg z-[60]">
          {/* Log panel header */}
          <div className="flex items-center justify-between px-4 py-3 border-b">
            <div className="flex items-center gap-2">
              <IconMessage2 className="size-4" />
              <span className="font-medium text-sm">通信记录</span>
              {logs.length > 0 && (
                <Badge variant="secondary" className="text-xs">
                  {logs.length}
                </Badge>
              )}
            </div>
            <div className="flex items-center gap-1">
              {logs.length > 0 && (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 w-6 p-0"
                        onClick={onClearLogs}
                      >
                        <IconTrash className="size-3.5" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>清空记录</p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 w-6 p-0"
                      onClick={onToggleLogPanel}
                    >
                      <IconChevronRight className="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>关闭</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          </div>

          {/* Log entries */}
          <div
            ref={logContainerRef}
            className="flex-1 overflow-y-auto px-3 py-2"
          >
            {logs.length === 0 ? (
              <div className="flex h-full items-center justify-center text-xs text-muted-foreground text-center px-4">
                暂无通信记录
              </div>
            ) : (
              <>
                {logs.map((log) => (
                  <LogEntry key={log.id} log={log} />
                ))}
              </>
            )}
          </div>
        </div>,
        document.body,
      )}
    </>
  )
}

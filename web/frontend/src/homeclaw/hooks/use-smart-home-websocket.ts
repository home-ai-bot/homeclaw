import { useEffect, useRef, useState, useCallback } from "react"
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
  /** For tool calls: show the method and brand */
  metadata?: {
    toolName?: string
    method?: string
    brand?: string
  }
}

/**
 * Hook to manage smart home WebSocket connection and communication logs.
 * All smart home pages should use this hook for consistent WS handling.
 */
export function useSmartHomeWebSocket() {
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

  // Manage WebSocket lifecycle
  useEffect(() => {
    const onStatus = (status: DeviceControlWSStatus) => {
      setWsStatus(status)
    }

    const onMessage = (data: unknown) => {
      const d = data as Record<string, unknown>
      const payload = d.payload as Record<string, unknown> | undefined

      // Log incoming messages
      if (d.type === "message.create" && typeof payload?.content === "string") {
        addLog({
          type: "receive",
          title: "Agent 回复",
          content: payload.content,
        })
      } else if (d.type === "message.create" && payload?.tool_call) {
        // Tool call response
        const toolCall = payload.tool_call as Record<string, unknown>
        addLog({
          type: "receive",
          title: `工具响应: ${toolCall.tool_name || "unknown"}`,
          content: typeof payload.content === "string" ? payload.content : JSON.stringify(payload, null, 2),
          metadata: {
            toolName: toolCall.tool_name as string,
            method: toolCall.method as string,
          },
        })
      } else {
        addLog({
          type: "receive",
          title: String(d.type || "未知消息"),
          content: JSON.stringify(data, null, 2),
        })
      }
    }

    deviceControlWS.onStatus(onStatus)
    deviceControlWS.onMessage(onMessage)
    void deviceControlWS.connect()

    return () => {
      deviceControlWS.offStatus(onStatus)
      deviceControlWS.offMessage(onMessage)
      // Don't disconnect here - let the last unmounting page handle it
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // Disconnect when component truly unmounts (use ref to track)
  const mountCountRef = useRef(0)
  useEffect(() => {
    mountCountRef.current++
    return () => {
      mountCountRef.current--
      if (mountCountRef.current === 0) {
        deviceControlWS.disconnect()
      }
    }
  }, [])

  const addLog = (log: Omit<CommunicationLog, "id" | "timestamp">) => {
    const newLog: CommunicationLog = {
      ...log,
      id: `log-${Date.now()}-${Math.random().toString(36).slice(2, 7)}`,
      timestamp: Date.now(),
    }
    setLogs((prev) => [...prev, newLog])
  }

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
  }, [])

  /**
   * Execute a tool call with automatic logging.
   * This is the preferred method for calling tools from smart home pages.
   */
  const executeToolCall = useCallback(async (
    params: ToolCallParams,
    options: ToolCallOptions = {},
  ) => {
    const { toolName, method, brand } = params
    const logTitle = options.logTitle || `${toolName}.${method}`

    // Execute the tool call
    const result = await callTool(params, options)

    // Only log failures to communication panel
    // Success cases are usually handled by the page's own log/UI
    if (!result.success) {
      addLog({
        type: "error",
        title: `工具调用失败: ${logTitle}`,
        content: result.error,
        metadata: { toolName, method, brand },
      })
    }

    return result
  }, [])

  const clearLogs = () => {
    setLogs([])
  }

  const toggleLogPanel = () => {
    setShowLogPanel((prev) => !prev)
  }

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

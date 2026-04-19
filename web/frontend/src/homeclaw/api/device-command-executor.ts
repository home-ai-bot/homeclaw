// Universal tool command executor via dedicated device-control WebSocket.
// Uses DeviceControlWebSocket (session_id=device-control) which is separate
// from the shared chat picoWS to prevent session conflicts.
//
// Supports:
// 1. Async execution with response waiting (for quick operations)
// 2. Fire-and-forget mode (for long-running operations)
// 3. Automatic logging to communication panel
// 4. Toast notification on success

import { toast } from "sonner"
import { deviceControlWS } from "@/homeclaw/api/device-control-websocket"

export interface ToolCallParams {
  /** Tool name (e.g., "hc_cli", "xiaomi_control", "tuya_control") */
  toolName: string
  /** Method to call (e.g., "exe", "login", "query") */
  method: string
  /** Brand identifier (e.g., "xiaomi", "tuya", "homekit") */
  brand: string
  /** Method-specific parameters */
  params?: Record<string, unknown>
}

export interface ToolCallOptions {
  /** Timeout in milliseconds (default: 60000) */
  timeout?: number
  /** Whether to wait for response (default: true). Set false for fire-and-forget. */
  waitForResponse?: boolean
  /** Success message to show in tooltip/notification */
  successMessage?: string
  /** Custom log title (default: toolName.method) */
  logTitle?: string
}

export interface ToolCallResult {
  success: boolean
  message?: string
  error?: string
  /** Whether this was a fire-and-forget call */
  fireAndForget?: boolean
}

/**
 * Execute a tool command via WebSocket with flexible options.
 *
 * Message format sent to the agent:
 *   tool:toolName {"brand":"<brand>","method":"<method>","params":{...}}
 *
 * The agent's HandleToolCall intercepts the "tool:" prefix, skips the LLM,
 * and dispatches directly to the appropriate tool handler.
 *
 * @example
 * // Async execution with response
 * const result = await callTool({
 *   toolName: "hc_cli",
 *   method: "exe",
 *   brand: "xiaomi",
 *   params: { from_id: "123", from: "xiaomi", ops: "turn_on" }
 * }, { successMessage: "设备已开启" })
 *
 * @example
 * // Fire-and-forget (long-running operation)
 * await callTool({
 *   toolName: "hc_cli",
 *   method: "generate_ops",
 *   brand: "tuya",
 *   params: { from_id: "456", from: "tuya" }
 * }, { waitForResponse: false, successMessage: "已发送请求" })
 */
export async function callTool(
  params: ToolCallParams,
  options: ToolCallOptions = {},
): Promise<ToolCallResult> {
  const {
    timeout = 60000,
    waitForResponse = true,
    successMessage,
  } = options

  const { toolName, method, brand, params: methodParams } = params

  try {
    const messageId = `tool-${toolName}-${method}-${Date.now()}`

    // Build command JSON
    const commandJson = JSON.stringify({
      brand,
      method,
      params: methodParams || {},
    })

    const message = {
      type: "message.send",
      id: messageId,
      session_id: "device-control",
      payload: {
        content: `tool:${toolName} ${commandJson}`,
        media: [],
      },
    }

    // Fire-and-forget mode
    if (!waitForResponse) {
      deviceControlWS.sendMessage(message)

      // Show success notification if provided
      if (successMessage) {
        showTooltip(successMessage)
      }

      return {
        success: true,
        message: successMessage || "命令已发送（异步执行）",
        fireAndForget: true,
      }
    }

    // Async execution with response waiting
    const response = await deviceControlWS.sendAndWait(
      message,
      (data) => {
        const d = data as Record<string, unknown>
        const payload = d.payload as Record<string, unknown> | undefined
        return (
          d.id === messageId ||
          (d.type === "message.create" && typeof payload?.content === "string")
        )
      },
      timeout,
    )

    const d = response as Record<string, unknown>
    const payload = d.payload as Record<string, unknown> | undefined

    if (d.type === "error") {
      const errorMsg = (payload?.message as string) || "Unknown error from gateway"
      return {
        success: false,
        error: errorMsg,
      }
    }

    const successMsg = (payload?.content as string) || successMessage || "命令执行成功"

    // Show success notification
    if (successMessage) {
      showTooltip(successMessage)
    }

    return {
      success: true,
      message: successMsg,
    }
  } catch (error) {
    console.error(`[ToolCall] ${toolName}.${method} failed:`, error)
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

/**
 * Execute a device operation (backward compatible wrapper).
 * @deprecated Use callTool() instead for more flexibility
 */
export async function executeDeviceOperation(
  fromId: string,
  from: string,
  opsName: string,
  timeout = 60000,
): Promise<ToolCallResult> {
  return callTool(
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
    { timeout },
  )
}

/**
 * Show a tooltip/notification message to the user.
 * Uses sonner toast library for consistent UI notifications.
 */
function showTooltip(message: string): void {
  toast.success(message, {
    duration: 3000,
    position: "top-right",
  })
}

/**
 * Hook to listen for tooltip messages.
 * Use this in your components to show toast notifications.
 *
 * @example
 * function MyComponent() {
 *   useTooltipListener()
 *   // ... rest of component
 * }
 */
export function useTooltipListener() {
  // This is a placeholder - in production, integrate with your toast library
  // Example with useEffect:
  //
  // useEffect(() => {
  //   const handler = (event: CustomEvent) => {
  //     toast.success(event.detail.message)
  //   }
  //   window.addEventListener("homeclaw:tooltip", handler as EventListener)
  //   return () => window.removeEventListener("homeclaw:tooltip", handler as EventListener)
  // }, [])
}

// Device command executor via dedicated device-control WebSocket.
// Uses DeviceControlWebSocket (session_id=device-control) which is separate
// from the shared chat picoWS to prevent session conflicts.

import { deviceControlWS } from "@/homeclaw/api/device-control-websocket"

export interface DeviceCommandResult {
  success: boolean
  message?: string
  error?: string
}

/**
 * Execute a named device operation by sending a tool:hc_cli exe command
 * directly to the gateway via the dedicated device-control WebSocket.
 *
 * Message format sent to the agent:
 *   tool:hc_cli {"brand":"<from>","method":"exe","params":{"from_id":"<fromId>","from":"<from>","ops":"<opsName>"}}
 *
 * The agent's HandleToolCall intercepts the "tool:" prefix, skips the LLM,
 * and dispatches directly to CLITool.execExe which resolves the stored
 * device operation and calls the brand client.
 */
export async function executeDeviceOperation(
  fromId: string,
  from: string,
  opsName: string,
  timeout = 60000,
): Promise<DeviceCommandResult> {
  try {
    const messageId = `device-op-${Date.now()}`

    // Build the commandJson directly — no backend roundtrip needed.
    // execExe in cli_tool.go resolves params from the DeviceOpStore internally.
    const commandJson = JSON.stringify({
      brand: from,
      method: "exe",
      params: {
        from_id: fromId,
        from: from,
        ops: opsName,
      },
    })

    const response = await deviceControlWS.sendAndWait(
      {
        type: "message.send",
        id: messageId,
        session_id: "device-control",
        payload: {
          content: `tool:hc_cli ${commandJson}`,
          media: [],
        },
      },
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
      return {
        success: false,
        error: (payload?.message as string) || "Unknown error from gateway",
      }
    }

    return {
      success: true,
      message: (payload?.content as string) || "Command executed successfully",
    }
  } catch (error) {
    console.error("[DeviceCmd] Device operation failed:", error)
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

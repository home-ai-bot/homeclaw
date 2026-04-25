// API client for Device Operations integration

export interface Device {
  from_id: string
  from: string
  name: string
  type: string
  token?: string
  ip: string
  urn?: string
  space_name?: string
  ops?: string[]
}

export interface DeviceOpsByRoom {
  room_name: string
  devices: Device[]
}

export interface ExecuteDeviceOpRequest {
  from_id: string
  from: string
  ops_name: string
}

export interface ExecuteDeviceOpResponse {
  success: boolean
  command: Record<string, any>
}

export interface ClearDeviceOpsResponse {
  success: boolean
  brand: string
  devices_cleared: number
  ops_deleted: number
  message: string
}

const BASE_URL = ""

async function request<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, options)
  if (!res.ok) {
    const text = await res.text()
    throw new Error(text || `API error: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

/**
 * Fetch all devices via WebSocket using hc_common.listDevices tool.
 * Returns a flat list of devices with their operations.
 */
export async function listDevices(): Promise<Device[]> {
  const { deviceControlWS } = await import("@/homeclaw/api/device-control-websocket")

  try {
    const message = {
      type: "message.send",
      id: `tool-hc_common-listDevices-${Date.now()}`,
      session_id: "device-control",
      payload: {
        content: `tool:hc_common {"method":"listDevices"}`,
        media: [],
      },
    }

    const response = await deviceControlWS.sendAndWait(
      message,
      (data: unknown) => {
        const d = data as Record<string, unknown>
        return d.type === "message.create"
      },
      5000,
    )

    const payload = (response as Record<string, unknown>)?.payload as Record<string, unknown> | undefined
    const content = payload?.content as string | undefined

    if (content) {
      try {
        const devices = JSON.parse(content)
        if (Array.isArray(devices)) {
          return devices as Device[]
        }
      } catch {
        console.error("Failed to parse devices response:", content)
      }
    }

    return []
  } catch (error) {
    console.error("Failed to load devices:", error)
    return []
  }
}

export async function executeDeviceOp(
  req: ExecuteDeviceOpRequest,
): Promise<ExecuteDeviceOpResponse> {
  return request<ExecuteDeviceOpResponse>("/api/device-ops/execute", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(req),
  })
}

export async function clearDeviceOps(
  brand: string,
): Promise<ClearDeviceOpsResponse> {
  // Use WebSocket to call hc_cli.clearOps method
  const { callTool } = await import("@/homeclaw/api/device-command-executor")

  const result = await callTool(
    {
      toolName: "hc_cli",
      method: "clearOps",
      brand: brand,
      params: {
        brand: brand,
      },
    },
    {
      timeout: 30000,
    }
  )

  if (!result.success || !result.message) {
    throw new Error(result.error || "Failed to clear device operations")
  }

  // Parse the JSON response from the tool
  const response: ClearDeviceOpsResponse = JSON.parse(result.message)
  return response
}

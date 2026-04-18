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

export interface DeviceOpsResponse {
  rooms: DeviceOpsByRoom[]
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

export async function getAllDevicesWithOps(): Promise<DeviceOpsResponse> {
  return request<DeviceOpsResponse>("/api/device-ops/devices")
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

export interface MarkNoActionRequest {
  from_id: string
  from: string
}

export interface MarkNoActionResponse {
  success: boolean
  message: string
}

export async function markDeviceAsNoAction(
  req: MarkNoActionRequest,
): Promise<MarkNoActionResponse> {
  return request<MarkNoActionResponse>("/api/device-ops/mark-no-action", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(req),
  })
}

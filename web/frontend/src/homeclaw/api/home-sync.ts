import { deviceControlWS } from "./device-control-websocket"

export interface HomeInfo {
  id: string
  name: string
  current?: boolean
}

export interface DeviceInfo {
  from_id: string
  name: string
  type: string
  space_name?: string
  online?: boolean
}

interface SyncHomesResponse {
  success: boolean
  homes?: HomeInfo[]
  error?: string
}

interface SyncDevicesResponse {
  success: boolean
  devices?: DeviceInfo[]
  error?: string
}

export async function syncHomesViaWS(brand: string): Promise<SyncHomesResponse> {
  try {
    const message = {
      type: "message.send",
      id: `tool-hc_cli-syncHomes-${Date.now()}`,
      session_id: "device-control",
      payload: {
        content: `tool:hc_cli {"brand":"${brand}","method":"syncHomes"}`,
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
        const result = JSON.parse(content)
        if (result.error) {
          return { success: false, error: result.error }
        }
        if (result.homes && Array.isArray(result.homes)) {
          return {
            success: true,
            homes: result.homes.map((h: Record<string, unknown>) => ({
              id: h.home_id as string,
              name: h.name as string,
            })),
          }
        }
      } catch {
        // Parse error, return raw content as success
        return { success: true }
      }
    }

    return { success: true }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Failed to sync homes",
    }
  }
}

export async function setCurrentHomeViaWS(
  brand: string,
  homeId: string,
): Promise<{ success: boolean; error?: string }> {
  try {
    const message = {
      type: "message.send",
      id: `tool-hc_cli-setCurrentHome-${Date.now()}`,
      session_id: "device-control",
      payload: {
        content: `tool:hc_cli {"brand":"${brand}","method":"setCurrentHome","params":{"from_id":"${homeId}","from":"${brand}"}}`,
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
        const result = JSON.parse(content)
        if (result.error) {
          return { success: false, error: result.error }
        }
      } catch {
        // Ignore parse errors
      }
    }

    return { success: true }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Failed to set current home",
    }
  }
}

export async function syncDevicesViaWS(
  brand: string,
  homeId: string,
): Promise<SyncDevicesResponse> {
  try {
    const message = {
      type: "message.send",
      id: `tool-hc_cli-syncDevices-${Date.now()}`,
      session_id: "device-control",
      payload: {
        content: `tool:hc_cli {"brand":"${brand}","method":"syncDevices","params":{"homeId":"${homeId}"}}`,
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
        const result = JSON.parse(content)
        if (result.error) {
          return { success: false, error: result.error }
        }
        if (result.devices && Array.isArray(result.devices)) {
          return {
            success: true,
            devices: result.devices.map((d: Record<string, unknown>) => ({
              from_id: d.from_id as string,
              name: d.name as string,
              type: d.type as string,
              space_name: d.space_name as string | undefined,
              online: d.online as boolean | undefined,
            })),
          }
        }
      } catch {
        // Parse error, return success
        return { success: true }
      }
    }

    return { success: true }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Failed to sync devices",
    }
  }
}

export interface HomeInfoWithBrand {
  from_id: string
  from: string
  name: string
  current: boolean
}

export async function loadHomesFromBackend(): Promise<HomeInfoWithBrand[]> {
  try {
    const message = {
      type: "message.send",
      id: `tool-hc_common-listHomes-${Date.now()}`,
      session_id: "device-control",
      payload: {
        content: `tool:hc_common {"method":"listHomes"}`,
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
        const homes = JSON.parse(content)
        if (Array.isArray(homes)) {
          return homes.map((h: Record<string, unknown>) => ({
            from_id: h.from_id as string,
            from: h.from as string,
            name: h.name as string,
            current: h.current as boolean,
          }))
        }
      } catch {
        console.error("Failed to parse homes response:", content)
      }
    }

    return []
  } catch (error) {
    console.error("Failed to load homes:", error)
    return []
  }
}

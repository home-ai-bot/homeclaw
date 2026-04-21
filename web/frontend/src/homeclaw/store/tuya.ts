import { atom } from "jotai"

import {
  type TuyaRegion,
  type TuyaUser,
  getTuyaRegions,
  getTuyaStatus,
  loginTuya,
  logoutTuya,
  deleteTuyaCredentials,
} from "@/homeclaw/api/tuya"
import { callTool } from "@/homeclaw/api/device-command-executor"
import {
  syncHomesViaWS,
  setCurrentHomeViaWS,
  syncDevicesViaWS,
  loadHomesFromBackend,
  type HomeInfo,
  type DeviceInfo,
} from "@/homeclaw/api/home-sync"
import { listDevices } from "@/homeclaw/api/device-ops"

export interface TuyaStoreState {
  isLoggedIn: boolean
  isLoading: boolean
  authType: "token" | "credentials" | null
  regions: TuyaRegion[]
  user: TuyaUser | null
  region: string | null
  error: string | null
  // Home/Family state
  homes: HomeInfo[]
  selectedHomeId: string | null
  isSyncingHomes: boolean
  // Device state
  devices: DeviceInfo[]
  isSyncingDevices: boolean
}

const DEFAULT_TUYA_STATE: TuyaStoreState = {
  isLoggedIn: false,
  isLoading: true,
  authType: null,
  regions: [],
  user: null,
  region: null,
  error: null,
  homes: [],
  selectedHomeId: null,
  isSyncingHomes: false,
  devices: [],
  isSyncingDevices: false,
}

export const tuyaAtom = atom<TuyaStoreState>(DEFAULT_TUYA_STATE)

export async function fetchTuyaRegions() {
  try {
    const response = await getTuyaRegions()
    return response.regions
  } catch (error) {
    console.error("Failed to fetch Tuya regions:", error)
    return []
  }
}

export async function fetchTuyaStatus(): Promise<Partial<TuyaStoreState>> {
  try {
    // Use WebSocket to get actual authentication status via hc_cli.getAuthStatus
    const result = await callTool(
      {
        toolName: "hc_cli",
        method: "getAuthStatus",
        brand: "tuya",
        params: {
          brand: "tuya_token",
        },
      },
      { timeout: 5000 },
    )

    if (result.success && result.message) {
      try {
        const statusData = JSON.parse(result.message)
        return {
          isLoggedIn: statusData.logged_in || false,
          authType: statusData.logged_in ? "token" : null,
          region: statusData.region || null,
          error: null,
          isLoading: false,
        }
      } catch (parseError) {
        console.error("Failed to parse auth status:", parseError)
      }
    }

    // Fallback to HTTP status check (for backward compatibility)
    const status = await getTuyaStatus()
    return {
      isLoggedIn: status.logged_in,
      authType: status.auth_type ?? null,
      region: status.region || null,
      error: status.error || null,
      isLoading: false,
    }
  } catch (error) {
    return {
      isLoggedIn: false,
      authType: null,
      error: error instanceof Error ? error.message : "Unknown error",
      isLoading: false,
    }
  }
}

export async function tuyaLogin(
  region: string,
  username: string,
  password: string,
): Promise<{ success: boolean; error?: string }> {
  try {
    const response = await loginTuya({ region, username, password })
    if (response.success) {
      return {
        success: true,
      }
    }
    return {
      success: false,
      error: response.error || "Login failed",
    }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function tuyaLogout(): Promise<{ success: boolean; error?: string }> {
  try {
    await logoutTuya()
    return { success: true }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function tuyaDeleteCredentials(): Promise<{ success: boolean; error?: string }> {
  try {
    await deleteTuyaCredentials()
    return { success: true }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function tuyaSaveToken(
  token: string,
): Promise<{ success: boolean; error?: string }> {
  try {
    const result = await callTool(
      {
        toolName: "hc_cli",
        method: "saveAuth",
        brand: "tuya",
        params: {
          brand: "tuya_token",
          token: token,
        },
      },
      { successMessage: "Token saved successfully" },
    )

    if (result.success) return { success: true }
    return { success: false, error: result.error || "Failed to save token" }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function tuyaDeleteToken(): Promise<{ success: boolean; error?: string }> {
  try {
    const result = await callTool(
      {
        toolName: "hc_cli",
        method: "deleteAuth",
        brand: "tuya",
        params: {
          brand: "tuya_token",
        },
      },
      { successMessage: "Token deleted successfully" },
    )

    if (result.success) return { success: true }
    return { success: false, error: result.error || "Failed to delete token" }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function syncTuyaHomes(): Promise<{ success: boolean; error?: string }> {
  const result = await syncHomesViaWS("tuya")
  return result
}

export async function selectTuyaHome(
  homeId: string,
): Promise<{ success: boolean; error?: string }> {
  return await setCurrentHomeViaWS("tuya", homeId)
}

export async function syncTuyaDevices(
  homeId: string,
): Promise<{ success: boolean; error?: string }> {
  const result = await syncDevicesViaWS("tuya", homeId)
  return result
}

export async function loadTuyaHomes(): Promise<HomeInfo[]> {
  const allHomes = await loadHomesFromBackend()
  return allHomes
    .filter((h) => h.from === "tuya")
    .map((h) => ({
      id: h.from_id,
      name: h.name,
      current: h.current,
    }))
}

export async function loadTuyaDevices(): Promise<DeviceInfo[]> {
  try {
    // Fetch flat list of devices
    const devices = await listDevices()

    // Filter Tuya devices and convert to DeviceInfo format
    const tuyaDevices: DeviceInfo[] = devices
      .filter((device) => device.from === "tuya")
      .map((device) => ({
        from_id: device.from_id,
        name: device.name,
        type: device.type,
        space_name: device.space_name,
        online: true,
      }))

    return tuyaDevices
  } catch (error) {
    console.error("Failed to load tuya devices:", error)
    return []
  }
}

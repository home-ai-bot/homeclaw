import { atom } from "jotai"

import {
  type TuyaRegion,
  type TuyaUser,
  getTuyaRegions,
  getTuyaStatus,
  loginTuya,
  logoutTuya,
  deleteTuyaCredentials,
  saveTuyaToken,
  deleteTuyaToken,
} from "@/homeclaw/api/tuya"
import {
  syncHomesViaWS,
  setCurrentHomeViaWS,
  syncDevicesViaWS,
  loadHomesFromBackend,
  type HomeInfo,
  type DeviceInfo,
} from "@/homeclaw/api/home-sync"
import { listDevices, type Device } from "@/homeclaw/api/device-ops"

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
    const response = await saveTuyaToken(token)
    if (response.success) return { success: true }
    return { success: false, error: response.error || "Failed to save token" }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function tuyaDeleteToken(): Promise<{ success: boolean; error?: string }> {
  try {
    await deleteTuyaToken()
    return { success: true }
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

    // Group by room and filter Tuya devices
    const roomMap = new Map<string, Device[]>()
    for (const device of devices) {
      if (device.from !== "tuya") continue

      const roomName = device.space_name || "Unassigned"
      if (!roomMap.has(roomName)) {
        roomMap.set(roomName, [])
      }
      roomMap.get(roomName)!.push(device)
    }

    // Convert to DeviceInfo format
    const tuyaDevices: DeviceInfo[] = []
    for (const [roomName, roomDevices] of roomMap.entries()) {
      for (const device of roomDevices) {
        tuyaDevices.push({
          from_id: device.from_id,
          name: device.name,
          type: device.type,
          space_name: roomName !== "Unassigned" ? roomName : undefined,
          online: true,
        })
      }
    }

    return tuyaDevices
  } catch (error) {
    console.error("Failed to load tuya devices:", error)
    return []
  }
}

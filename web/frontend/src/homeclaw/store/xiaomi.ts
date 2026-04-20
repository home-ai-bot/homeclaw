import { atom } from "jotai"

import {
  type XiaomiLoginError,
  getXiaomiStatus,
  xiaomiLogin,
  xiaomiCaptcha,
  xiaomiVerify,
  xiaomiLogout,
} from "@/homeclaw/api/xiaomi"
import {
  syncHomesViaWS,
  setCurrentHomeViaWS,
  syncDevicesViaWS,
  loadHomesFromBackend,
  type HomeInfo,
  type DeviceInfo,
} from "@/homeclaw/api/home-sync"
import { listDevices, type Device } from "@/homeclaw/api/device-ops"

export type LoginStep = "login" | "captcha" | "verify" | "success"

export interface XiaomiStoreState {
  isLoggedIn: boolean
  isLoading: boolean
  userId: string | null
  error: string | null
  // Multi-step login state
  loginStep: LoginStep
  captchaImage: string | null // base64 image
  verifyTarget: string | null // phone or email to verify
  verifyType: "phone" | "email" | null
  // Home/Family state
  homes: HomeInfo[]
  selectedHomeId: string | null
  isSyncingHomes: boolean
  // Device state
  devices: DeviceInfo[]
  isSyncingDevices: boolean
}

const DEFAULT_XIAOMI_STATE: XiaomiStoreState = {
  isLoggedIn: false,
  isLoading: true,
  userId: null,
  error: null,
  loginStep: "login",
  captchaImage: null,
  verifyTarget: null,
  verifyType: null,
  homes: [],
  selectedHomeId: null,
  isSyncingHomes: false,
  devices: [],
  isSyncingDevices: false,
}

export const xiaomiAtom = atom<XiaomiStoreState>(DEFAULT_XIAOMI_STATE)

export async function fetchXiaomiStatus(): Promise<Partial<XiaomiStoreState>> {
  try {
    const status = await getXiaomiStatus()
    return {
      isLoggedIn: status.logged_in,
      userId: status.user_id || null,
      error: status.error || null,
      isLoading: false,
    }
  } catch (error) {
    return {
      isLoggedIn: false,
      error: error instanceof Error ? error.message : "Unknown error",
      isLoading: false,
    }
  }
}

function handleLoginError(
  error: XiaomiLoginError,
): { step: XiaomiStoreState["loginStep"]; captchaImage: string | null; verifyTarget: string | null; verifyType: XiaomiStoreState["verifyType"] } {
  if (error.captcha) {
    return {
      step: "captcha",
      captchaImage: error.captcha,
      verifyTarget: null,
      verifyType: null,
    }
  }
  if (error.verify_phone) {
    return {
      step: "verify",
      captchaImage: null,
      verifyTarget: error.verify_phone,
      verifyType: "phone",
    }
  }
  if (error.verify_email) {
    return {
      step: "verify",
      captchaImage: null,
      verifyTarget: error.verify_email,
      verifyType: "email",
    }
  }
  return {
    step: "login",
    captchaImage: null,
    verifyTarget: null,
    verifyType: null,
  }
}

export async function xiaomiAuthLogin(
  username: string,
  password: string,
): Promise<{ success: boolean; error?: string; step?: XiaomiStoreState["loginStep"]; captchaImage?: string | null; verifyTarget?: string | null; verifyType?: XiaomiStoreState["verifyType"] }> {
  try {
    const result = await xiaomiLogin(username, password)
    if (result.ok) {
      return { success: true }
    }
    const handled = handleLoginError(result.error)
    return { success: false, ...handled }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function xiaomiAuthCaptcha(
  captcha: string,
): Promise<{ success: boolean; error?: string; step?: XiaomiStoreState["loginStep"]; captchaImage?: string | null; verifyTarget?: string | null; verifyType?: XiaomiStoreState["verifyType"] }> {
  try {
    const result = await xiaomiCaptcha(captcha)
    if (result.ok) {
      return { success: true }
    }
    const handled = handleLoginError(result.error)
    return { success: false, ...handled }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function xiaomiAuthVerify(
  verify: string,
): Promise<{ success: boolean; error?: string; step?: XiaomiStoreState["loginStep"]; captchaImage?: string | null; verifyTarget?: string | null; verifyType?: XiaomiStoreState["verifyType"] }> {
  try {
    const result = await xiaomiVerify(verify)
    if (result.ok) {
      return { success: true }
    }
    const handled = handleLoginError(result.error)
    return { success: false, ...handled }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export async function xiaomiLogoutAction(): Promise<{ success: boolean; error?: string }> {
  try {
    await xiaomiLogout()
    return { success: true }
  } catch (error) {
    return {
      success: false,
      error: error instanceof Error ? error.message : "Unknown error",
    }
  }
}

export function resetLoginStep(): Partial<XiaomiStoreState> {
  return {
    loginStep: "login",
    captchaImage: null,
    verifyTarget: null,
    verifyType: null,
    error: null,
  }
}

export async function syncXiaomiHomes(): Promise<{ success: boolean; error?: string }> {
  const result = await syncHomesViaWS("xiaomi")
  return result
}

export async function selectXiaomiHome(
  homeId: string,
): Promise<{ success: boolean; error?: string }> {
  return await setCurrentHomeViaWS("xiaomi", homeId)
}

export async function syncXiaomiDevices(
  homeId: string,
): Promise<{ success: boolean; error?: string }> {
  const result = await syncDevicesViaWS("xiaomi", homeId)
  return result
}

export async function loadXiaomiHomes(): Promise<HomeInfo[]> {
  const allHomes = await loadHomesFromBackend()
  console.log('[loadXiaomiHomes] All homes from backend:', allHomes)
  const xiaomiHomes = allHomes
    .filter((h) => h.from === "xiaomi")
    .map((h) => ({
      id: h.from_id,
      name: h.name,
      current: h.current,
    }))
  console.log('[loadXiaomiHomes] Filtered xiaomi homes:', xiaomiHomes)
  return xiaomiHomes
}

export async function loadXiaomiDevices(): Promise<DeviceInfo[]> {
  try {
    // Fetch flat list of devices
    const devices = await listDevices()
    console.log('[loadXiaomiDevices] All devices from backend:', devices)

    // Group by room and filter Xiaomi devices
    const roomMap = new Map<string, Device[]>()
    for (const device of devices) {
      if (device.from !== "xiaomi") continue

      const roomName = device.space_name || "Unassigned"
      if (!roomMap.has(roomName)) {
        roomMap.set(roomName, [])
      }
      roomMap.get(roomName)!.push(device)
    }

    // Convert to DeviceInfo format
    const xiaomiDevices: DeviceInfo[] = []
    for (const [roomName, roomDevices] of roomMap.entries()) {
      for (const device of roomDevices) {
        xiaomiDevices.push({
          from_id: device.from_id,
          name: device.name,
          type: device.type,
          space_name: roomName !== "Unassigned" ? roomName : undefined,
          online: true,
        })
      }
    }

    console.log('[loadXiaomiDevices] Filtered xiaomi devices:', xiaomiDevices)
    return xiaomiDevices
  } catch (error) {
    console.error("Failed to load xiaomi devices:", error)
    return []
  }
}

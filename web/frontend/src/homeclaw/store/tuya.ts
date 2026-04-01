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

export interface TuyaStoreState {
  isLoggedIn: boolean
  isLoading: boolean
  regions: TuyaRegion[]
  user: TuyaUser | null
  region: string | null
  error: string | null
}

const DEFAULT_TUYA_STATE: TuyaStoreState = {
  isLoggedIn: false,
  isLoading: true,
  regions: [],
  user: null,
  region: null,
  error: null,
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
      region: status.region || null,
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

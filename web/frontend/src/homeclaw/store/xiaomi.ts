import { atom } from "jotai"

import {
  type XiaomiLoginError,
  getXiaomiStatus,
  xiaomiLogin,
  xiaomiCaptcha,
  xiaomiVerify,
  xiaomiLogout,
} from "@/homeclaw/api/xiaomi"

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

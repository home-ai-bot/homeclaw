// API client for Xiaomi smart home integration.

export interface XiaomiStatusResponse {
  logged_in: boolean
  user_id?: string
  error?: string
}

// LoginError from xiaomi.Cloud - returned with 401 status
export interface XiaomiLoginError {
  captcha?: string // base64 encoded image
  verify_phone?: string
  verify_email?: string
}

export interface XiaomiAuthResponse {
  success: boolean
  error?: string
}

const BASE_URL = ""

async function request<T>(
  path: string,
  options?: RequestInit,
): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, options)
  if (!res.ok) {
    // For 401, the response body contains LoginError (captcha/verify)
    if (res.status === 401) {
      const errorData = (await res.json()) as XiaomiLoginError
      const error = new Error("LoginStep")
      ;(error as Error & { data: XiaomiLoginError }).data = errorData
      throw error
    }
    const text = await res.text()
    throw new Error(text || `API error: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

// Special request that returns LoginError on 401 instead of throwing
async function authRequest(
  path: string,
  options?: RequestInit,
): Promise<{ ok: true; data: XiaomiAuthResponse } | { ok: false; error: XiaomiLoginError }> {
  const res = await fetch(`${BASE_URL}${path}`, options)
  if (res.ok) {
    const data = (await res.json()) as XiaomiAuthResponse
    return { ok: true, data }
  }
  if (res.status === 401) {
    const errorData = (await res.json()) as XiaomiLoginError
    return { ok: false, error: errorData }
  }
  const text = await res.text()
  throw new Error(text || `API error: ${res.status} ${res.statusText}`)
}

export async function getXiaomiStatus(): Promise<XiaomiStatusResponse> {
  return request<XiaomiStatusResponse>("/api/xiaomi/status")
}

export async function xiaomiLogin(
  username: string,
  password: string,
): Promise<{ ok: true; data: XiaomiAuthResponse } | { ok: false; error: XiaomiLoginError }> {
  const params = new URLSearchParams()
  params.append("username", username)
  params.append("password", password)
  return authRequest("/api/xiaomi/auth", {
    method: "POST",
    body: params,
  })
}

export async function xiaomiCaptcha(
  captcha: string,
): Promise<{ ok: true; data: XiaomiAuthResponse } | { ok: false; error: XiaomiLoginError }> {
  const params = new URLSearchParams()
  params.append("captcha", captcha)
  return authRequest("/api/xiaomi/auth", {
    method: "POST",
    body: params,
  })
}

export async function xiaomiVerify(
  verify: string,
): Promise<{ ok: true; data: XiaomiAuthResponse } | { ok: false; error: XiaomiLoginError }> {
  const params = new URLSearchParams()
  params.append("verify", verify)
  return authRequest("/api/xiaomi/auth", {
    method: "POST",
    body: params,
  })
}

export async function xiaomiLogout(): Promise<XiaomiAuthResponse> {
  return request<XiaomiAuthResponse>("/api/xiaomi/logout", {
    method: "POST",
  })
}

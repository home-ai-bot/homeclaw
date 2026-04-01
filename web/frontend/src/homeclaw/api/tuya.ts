// API client for Tuya smart home integration.

export interface TuyaRegion {
  name: string
  host: string
  description: string
  continent: string
}

export interface TuyaRegionsResponse {
  regions: TuyaRegion[]
}

export interface TuyaStatusResponse {
  logged_in: boolean
  region?: string
  username?: string
  error?: string
}

export interface TuyaLoginRequest {
  region: string
  username: string
  password: string
}

export interface TuyaUser {
  uid: string
  username: string
  nickname: string
  email: string
  timezone: string
}

export interface TuyaLoginResponse {
  success: boolean
  user?: TuyaUser
  region?: string
  error?: string
}

export interface TuyaLogoutResponse {
  success: boolean
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

export async function getTuyaRegions(): Promise<TuyaRegionsResponse> {
  return request<TuyaRegionsResponse>("/api/tuya/regions")
}

export async function getTuyaStatus(): Promise<TuyaStatusResponse> {
  return request<TuyaStatusResponse>("/api/tuya/status")
}

export async function loginTuya(
  credentials: TuyaLoginRequest,
): Promise<TuyaLoginResponse> {
  return request<TuyaLoginResponse>("/api/tuya/login", {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify(credentials),
  })
}

export async function logoutTuya(): Promise<TuyaLogoutResponse> {
  return request<TuyaLogoutResponse>("/api/tuya/logout", {
    method: "POST",
  })
}

export async function deleteTuyaCredentials(): Promise<TuyaLogoutResponse> {
  return request<TuyaLogoutResponse>("/api/tuya/credentials", {
    method: "DELETE",
  })
}

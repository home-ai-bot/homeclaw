// API client for go2rtc process management.

interface Go2RTCStatusResponse {
  go2rtc_status: "running" | "starting" | "restarting" | "stopped" | "error"
  go2rtc_start_allowed?: boolean
  go2rtc_start_reason?: string
  pid?: number
  config_path?: string
  binary_path?: string
  [key: string]: unknown
}

interface Go2RTCLogsResponse {
  logs?: string[]
  log_total?: number
  log_run_id?: number
}

interface Go2RTCActionResponse {
  status: string
  pid?: number
  log_total?: number
  log_run_id?: number
}

const BASE_URL = ""

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${BASE_URL}${path}`, options)
  if (!res.ok) {
    throw new Error(`API error: ${res.status} ${res.statusText}`)
  }
  return res.json() as Promise<T>
}

export async function getGo2RTCStatus(): Promise<Go2RTCStatusResponse> {
  return request<Go2RTCStatusResponse>("/api/go2rtc/status")
}

export async function getGo2RTCLogs(options?: {
  log_offset?: number
  log_run_id?: number
}): Promise<Go2RTCLogsResponse> {
  const params = new URLSearchParams()
  if (options?.log_offset !== undefined) {
    params.set("log_offset", options.log_offset.toString())
  }
  if (options?.log_run_id !== undefined) {
    params.set("log_run_id", options.log_run_id.toString())
  }
  const queryString = params.toString() ? `?${params.toString()}` : ""
  return request<Go2RTCLogsResponse>(`/api/go2rtc/logs${queryString}`)
}

export async function startGo2RTC(): Promise<Go2RTCActionResponse> {
  return request<Go2RTCActionResponse>("/api/go2rtc/start", {
    method: "POST",
  })
}

export async function stopGo2RTC(): Promise<Go2RTCActionResponse> {
  return request<Go2RTCActionResponse>("/api/go2rtc/stop", {
    method: "POST",
  })
}

export async function restartGo2RTC(): Promise<Go2RTCActionResponse> {
  return request<Go2RTCActionResponse>("/api/go2rtc/restart", {
    method: "POST",
  })
}

export async function clearGo2RTCLogs(): Promise<Go2RTCActionResponse> {
  return request<Go2RTCActionResponse>("/api/go2rtc/logs/clear", {
    method: "POST",
  })
}

export type {
  Go2RTCStatusResponse,
  Go2RTCLogsResponse,
  Go2RTCActionResponse,
}

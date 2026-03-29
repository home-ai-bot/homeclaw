import { atom, getDefaultStore } from "jotai"

import { type Go2RTCStatusResponse, getGo2RTCStatus } from "@/api/go2rtc"

export type Go2RTCState =
  | "running"
  | "starting"
  | "restarting"
  | "stopping"
  | "stopped"
  | "error"
  | "unknown"

export interface Go2RTCStoreState {
  status: Go2RTCState
  canStart: boolean
}

type Go2RTCStorePatch = Partial<Go2RTCStoreState>

const DEFAULT_GO2RTC_STATE: Go2RTCStoreState = {
  status: "unknown",
  canStart: true,
}

const GO2RTC_POLL_INTERVAL_MS = 3000
const GO2RTC_TRANSIENT_POLL_INTERVAL_MS = 1000
const GO2RTC_STOPPING_TIMEOUT_MS = 5000

interface RefreshGo2RTCStateOptions {
  force?: boolean
}

// Global atom for go2rtc state
export const go2rtcAtom = atom<Go2RTCStoreState>(DEFAULT_GO2RTC_STATE)

let go2rtcPollingSubscribers = 0
let go2rtcPollingTimer: ReturnType<typeof setTimeout> | null = null
let go2rtcPollingRequest: Promise<void> | null = null
let go2rtcStoppingTimer: ReturnType<typeof setTimeout> | null = null

function clearGo2RTCStoppingTimeout() {
  if (go2rtcStoppingTimer !== null) {
    clearTimeout(go2rtcStoppingTimer)
    go2rtcStoppingTimer = null
  }
}

function normalizeGo2RTCStoreState(
  prev: Go2RTCStoreState,
  patch: Go2RTCStorePatch,
) {
  const next = { ...prev, ...patch }

  if (next.status === prev.status && next.canStart === prev.canStart) {
    return prev
  }

  return next
}

export function updateGo2RTCStore(
  patch:
    | Go2RTCStorePatch
    | ((prev: Go2RTCStoreState) => Go2RTCStorePatch | Go2RTCStoreState),
) {
  const store = getDefaultStore()
  store.set(go2rtcAtom, (prev) => {
    const nextPatch = typeof patch === "function" ? patch(prev) : patch
    return normalizeGo2RTCStoreState(prev, nextPatch)
  })
  const nextState = store.get(go2rtcAtom)
  if (nextState?.status !== "stopping") {
    clearGo2RTCStoppingTimeout()
  }
}

export function beginGo2RTCStoppingTransition() {
  clearGo2RTCStoppingTimeout()
  updateGo2RTCStore({
    status: "stopping",
    canStart: false,
  })
  go2rtcStoppingTimer = setTimeout(() => {
    go2rtcStoppingTimer = null
    updateGo2RTCStore((prev) =>
      prev.status === "stopping" ? { status: "running" } : prev,
    )
    void refreshGo2RTCState({ force: true })
  }, GO2RTC_STOPPING_TIMEOUT_MS)
}

export function cancelGo2RTCStoppingTransition() {
  clearGo2RTCStoppingTimeout()
  updateGo2RTCStore((prev) =>
    prev.status === "stopping" ? { status: "running" } : prev,
  )
}

export function applyGo2RTCStatusToStore(
  data: Partial<
    Pick<Go2RTCStatusResponse, "go2rtc_status" | "go2rtc_start_allowed">
  >,
) {
  updateGo2RTCStore((prev) => ({
    status:
      prev.status === "stopping" && data.go2rtc_status === "running"
        ? "stopping"
        : (data.go2rtc_status ?? prev.status),
    canStart:
      prev.status === "stopping" && data.go2rtc_status === "running"
        ? false
        : (data.go2rtc_start_allowed ?? prev.canStart),
  }))
}

function nextGo2RTCPollInterval() {
  const status = getDefaultStore().get(go2rtcAtom).status
  if (
    status === "starting" ||
    status === "restarting" ||
    status === "stopping"
  ) {
    return GO2RTC_TRANSIENT_POLL_INTERVAL_MS
  }
  return GO2RTC_POLL_INTERVAL_MS
}

function scheduleGo2RTCPoll(delay = nextGo2RTCPollInterval()) {
  if (go2rtcPollingSubscribers === 0) {
    return
  }

  if (go2rtcPollingTimer !== null) {
    clearTimeout(go2rtcPollingTimer)
  }

  go2rtcPollingTimer = setTimeout(() => {
    go2rtcPollingTimer = null
    void refreshGo2RTCState()
  }, delay)
}

export async function refreshGo2RTCState(
  options: RefreshGo2RTCStateOptions = {},
) {
  if (go2rtcPollingRequest) {
    await go2rtcPollingRequest
    if (options.force) {
      return refreshGo2RTCState()
    }
    return
  }

  go2rtcPollingRequest = (async () => {
    try {
      const status = await getGo2RTCStatus()
      applyGo2RTCStatusToStore(status)
    } catch {
      // Preserve the last known state when a poll fails.
    } finally {
      go2rtcPollingRequest = null
      scheduleGo2RTCPoll()
    }
  })()

  try {
    await go2rtcPollingRequest
  } finally {
    if (go2rtcPollingSubscribers === 0 && go2rtcPollingTimer !== null) {
      clearTimeout(go2rtcPollingTimer)
      go2rtcPollingTimer = null
    }
  }
}

export function subscribeGo2RTCPolling() {
  go2rtcPollingSubscribers += 1
  if (go2rtcPollingSubscribers === 1) {
    void refreshGo2RTCState()
  }

  return () => {
    go2rtcPollingSubscribers = Math.max(0, go2rtcPollingSubscribers - 1)
    if (go2rtcPollingSubscribers === 0 && go2rtcPollingTimer !== null) {
      clearTimeout(go2rtcPollingTimer)
      go2rtcPollingTimer = null
    }
  }
}

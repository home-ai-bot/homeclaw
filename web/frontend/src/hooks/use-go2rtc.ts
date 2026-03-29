import { useAtomValue } from "jotai"
import { useCallback, useEffect, useState } from "react"

import { restartGo2RTC, startGo2RTC, stopGo2RTC } from "@/api/go2rtc"
import {
  beginGo2RTCStoppingTransition,
  cancelGo2RTCStoppingTransition,
  go2rtcAtom,
  refreshGo2RTCState,
  subscribeGo2RTCPolling,
  updateGo2RTCStore,
} from "@/store"

export function useGo2RTC() {
  const go2rtc = useAtomValue(go2rtcAtom)
  const { status: state, canStart } = go2rtc
  const [loading, setLoading] = useState(false)

  useEffect(() => {
    return subscribeGo2RTCPolling()
  }, [])

  const start = useCallback(async () => {
    if (!canStart) return

    setLoading(true)
    try {
      await startGo2RTC()
      updateGo2RTCStore({
        status: "starting",
      })
    } catch (err) {
      console.error("Failed to start go2rtc:", err)
    } finally {
      await refreshGo2RTCState({ force: true })
      setLoading(false)
    }
  }, [canStart])

  const stop = useCallback(async () => {
    setLoading(true)
    beginGo2RTCStoppingTransition()
    try {
      await stopGo2RTC()
    } catch (err) {
      console.error("Failed to stop go2rtc:", err)
      cancelGo2RTCStoppingTransition()
    } finally {
      await refreshGo2RTCState({ force: true })
      setLoading(false)
    }
  }, [])

  const restart = useCallback(async () => {
    if (state !== "running") return

    setLoading(true)
    try {
      await restartGo2RTC()
      updateGo2RTCStore({
        status: "restarting",
      })
    } catch (err) {
      console.error("Failed to restart go2rtc:", err)
    } finally {
      await refreshGo2RTCState({ force: true })
      setLoading(false)
    }
  }, [state])

  return { state, loading, canStart, start, stop, restart }
}

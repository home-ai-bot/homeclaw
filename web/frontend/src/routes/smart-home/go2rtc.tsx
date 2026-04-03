import { createFileRoute } from "@tanstack/react-router"

import { Go2RTCPage } from "@/homeclaw/components/go2rtc-page"

export const Route = createFileRoute("/smart-home/go2rtc")({
  component: Go2RTCPage,
})

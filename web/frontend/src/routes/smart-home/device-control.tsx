import { createFileRoute } from "@tanstack/react-router"

import { DeviceControlPage } from "@/homeclaw/components/device-control-page"

export const Route = createFileRoute("/smart-home/device-control")({
  component: DeviceControlPage,
})

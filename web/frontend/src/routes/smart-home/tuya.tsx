import { createFileRoute } from "@tanstack/react-router"

import { TuyaPage } from "@/homeclaw/components/tuya-page"

export const Route = createFileRoute("/smart-home/tuya")({
  component: TuyaPage,
})

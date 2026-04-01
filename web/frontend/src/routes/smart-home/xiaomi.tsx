import { createFileRoute } from "@tanstack/react-router"

import { XiaomiPage } from "@/homeclaw/components/xiaomi-page"

export const Route = createFileRoute("/smart-home/xiaomi")({
  component: XiaomiPage,
})

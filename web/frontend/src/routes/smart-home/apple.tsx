import { createFileRoute } from "@tanstack/react-router"

import { ApplePage } from "@/homeclaw/components/apple-page"

export const Route = createFileRoute("/smart-home/apple")({
  component: ApplePage,
})

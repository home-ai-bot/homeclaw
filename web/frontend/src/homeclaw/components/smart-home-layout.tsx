import { IconLoader2 } from "@tabler/icons-react"
import type { ReactNode } from "react"

interface SmartHomeLayoutProps {
  /** Page title */
  title: string
  /** Children content */
  children: ReactNode
  /** Whether currently loading */
  isLoading?: boolean
}

/**
 * Simple layout wrapper for smart home pages.
 * WebSocket control panel is now managed globally in AppHeader.
 */
export function SmartHomeLayout({
  title,
  children,
  isLoading = false,
}: SmartHomeLayoutProps) {
  return (
    <div className="flex h-full flex-col">
      {/* Page header */}
      <div className="flex items-center gap-3 px-4 sm:px-6 pt-4 pb-2">
        <h1 className="text-xl font-semibold flex-1">{title}</h1>
        {isLoading && <IconLoader2 className="size-5 animate-spin text-muted-foreground" />}
      </div>

      {/* Page content (scrollable) */}
      <div className="min-h-0 flex-1 overflow-y-auto px-4 sm:px-6">
        {children}
      </div>
    </div>
  )
}

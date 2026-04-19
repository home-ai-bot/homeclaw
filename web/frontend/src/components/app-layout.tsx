import type { ReactNode } from "react"
import { Toaster } from "sonner"

import { AppHeader } from "@/components/app-header"
import { AppSidebar } from "@/components/app-sidebar"
import { TourGuide } from "@/components/tour/tour-guide"
import { SidebarProvider } from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { DeviceControlProvider } from "@/homeclaw/context/device-control-context"
import { useGlobalDeviceControlWS } from "@/homeclaw/components/device-control-panel"

// Wrapper component to use the hook and provide context
function DeviceControlWrapper({ children }: { children: ReactNode }) {
  const wsManager = useGlobalDeviceControlWS()
  
  return (
    <DeviceControlProvider value={wsManager}>
      {children}
    </DeviceControlProvider>
  )
}

export function AppLayout({ children }: { children: ReactNode }) {
  return (
    <DeviceControlWrapper>
      <TooltipProvider>
        <SidebarProvider className="flex h-dvh flex-col overflow-hidden">
          <AppHeader />

          <div className="flex flex-1 overflow-hidden">
            <AppSidebar />
            <div className="flex w-full flex-col overflow-hidden">
              <main className="flex min-h-0 w-full max-w-full flex-1 flex-col overflow-hidden">
                {children}
              </main>
            </div>
          </div>
          <Toaster position="bottom-center" />
          <TourGuide />
        </SidebarProvider>
      </TooltipProvider>
    </DeviceControlWrapper>
  )
}

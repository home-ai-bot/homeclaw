import { IconLoader2, IconDevices, IconRefresh, IconCircle, IconCircleOff } from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

export interface DeviceInfo {
  from_id: string
  name: string
  type: string
  space_name?: string
  online?: boolean
}

interface DeviceListSectionProps {
  devices: DeviceInfo[]
  isSyncing: boolean
  onSync: () => void
  disabled?: boolean
}

export function DeviceListSection({
  devices,
  isSyncing,
  onSync,
  disabled,
}: DeviceListSectionProps) {
  const { t } = useTranslation("homeclaw")

  // Sort devices by room (space_name) then by name
  const sortedDevices = [...devices].sort((a, b) => {
    const roomA = a.space_name || ""
    const roomB = b.space_name || ""
    if (roomA !== roomB) return roomA.localeCompare(roomB)
    return a.name.localeCompare(b.name)
  })

  return (
    <Card className="mt-4">
      <CardHeader className="py-3 px-4">
        <div className="flex items-center justify-between">
          <CardTitle className="flex items-center gap-2 text-base">
            <IconDevices className="size-4" />
            {t("device_section.title")}
          </CardTitle>
          <Button
            onClick={onSync}
            disabled={isSyncing || disabled}
            variant="outline"
            size="sm"
            className="h-8"
          >
            {isSyncing ? (
              <>
                <IconLoader2 className="mr-1 size-3 animate-spin" />
                {t("device_section.syncingDevices")}
              </>
            ) : (
              <>
                <IconRefresh className="mr-1 size-3" />
                {t("device_section.syncDevices")}
              </>
            )}
          </Button>
        </div>
      </CardHeader>
      <CardContent className="px-4 pb-4">
        {disabled ? (
          <div className="text-muted-foreground py-4 text-center text-sm">
            {t("device_section.selectHomeFirst")}
          </div>
        ) : devices.length === 0 ? (
          <div className="text-muted-foreground py-4 text-center text-sm">
            {t("device_section.noDevices")}
          </div>
        ) : (
          <div className="space-y-1">
            {sortedDevices.map((device) => (
              <div
                key={device.from_id}
                className="flex items-center gap-2 rounded border px-3 py-1.5 text-sm"
              >
                {device.online !== false ? (
                  <IconCircle className="size-3 shrink-0 fill-green-500 text-green-500" />
                ) : (
                  <IconCircleOff className="size-3 shrink-0 text-muted-foreground" />
                )}
                {device.space_name && (
                  <span className="shrink-0 text-muted-foreground">{device.space_name}</span>
                )}
                <span className="font-medium truncate">{device.name}</span>
                <span className="shrink-0 text-muted-foreground text-xs">{device.type}</span>
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  )
}

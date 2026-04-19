import { IconLoader2, IconHome, IconRefresh, IconCheck } from "@tabler/icons-react"
import { useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"

export interface HomeInfo {
  id: string
  name: string
  current?: boolean
}

interface HomeSectionProps {
  homes: HomeInfo[]
  selectedHomeId: string | null
  isSyncing: boolean
  onSync: () => void
  onSelect: (homeId: string) => void
}

export function HomeSection({
  homes,
  selectedHomeId,
  isSyncing,
  onSync,
  onSelect,
}: HomeSectionProps) {
  const { t } = useTranslation("homeclaw")
  const [pendingHomeId, setPendingHomeId] = useState<string | null>(null)

  const handleHomeSelect = (homeId: string) => {
    if (homeId === selectedHomeId) {
      return
    }
    if (selectedHomeId !== null) {
      setPendingHomeId(homeId)
    } else {
      onSelect(homeId)
    }
  }

  const confirmHomeChange = () => {
    if (pendingHomeId) {
      onSelect(pendingHomeId)
      setPendingHomeId(null)
    }
  }

  const cancelHomeChange = () => {
    setPendingHomeId(null)
  }

  return (
    <Card className="mt-4">
      <CardHeader className="py-3 px-4">
        <div className="flex items-center justify-between">
          <CardTitle className="flex items-center gap-2 text-base">
            <IconHome className="size-4" />
            {t("home_section.title")}
          </CardTitle>
          <Button
            onClick={onSync}
            disabled={isSyncing}
            variant="outline"
            size="sm"
            className="h-8"
          >
            {isSyncing ? (
              <>
                <IconLoader2 className="mr-1 size-3 animate-spin" />
                {t("home_section.syncingHomes")}
              </>
            ) : (
              <>
                <IconRefresh className="mr-1 size-3" />
                {t("home_section.syncHomes")}
              </>
            )}
          </Button>
        </div>
        {homes.length > 0 && (
          <CardDescription className="px-0">
            {homes.length} {t("home_section.title").toLowerCase()} available
          </CardDescription>
        )}
      </CardHeader>
      <CardContent className="px-4 pb-4">
        {homes.length === 0 ? (
          <div className="text-muted-foreground py-4 text-center text-sm">
            {t("home_section.noHomes")}
          </div>
        ) : (
          <div className="grid grid-cols-3 gap-2 lg:grid-cols-4">
            {homes.map((home) => {
              const isSelected = home.id === selectedHomeId
              const isCurrent = home.current === true
              return (
                <button
                  key={home.id}
                  type="button"
                  onClick={() => handleHomeSelect(home.id)}
                  className={`flex items-center gap-2 rounded border p-2 text-sm transition-colors hover:bg-accent ${
                    isSelected
                      ? "border-primary bg-accent"
                      : "border-border"
                  }`}
                >
                  <span className="truncate font-medium">{home.name}</span>
                  {isCurrent && (
                    <IconCheck
                      className={`size-4 flex-shrink-0 ${
                        isSelected ? "text-primary" : "text-muted-foreground"
                      }`}
                    />
                  )}
                </button>
              )
            })}
          </div>
        )}

        {pendingHomeId && (
          <div className="mt-3 rounded-md border border-amber-200 bg-amber-50 p-2 dark:border-amber-800 dark:bg-amber-950">
            <p className="text-xs text-amber-800 dark:text-amber-200">
              {t("home_section.confirmHomeChange")}
            </p>
            <div className="mt-2 flex gap-2">
              <Button size="sm" className="h-7" onClick={confirmHomeChange}>
                {t("buttons.confirm")}
              </Button>
              <Button size="sm" variant="outline" className="h-7" onClick={cancelHomeChange}>
                {t("buttons.cancel")}
              </Button>
            </div>
          </div>
        )}
      </CardContent>
    </Card>
  )
}

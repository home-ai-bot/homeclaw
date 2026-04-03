import {
  IconLoader2,
  IconPlayerPlay,
  IconPlayerStop,
  IconRefresh,
  IconVideo,
} from "@tabler/icons-react"
import { useTranslation } from "react-i18next"

import { PageHeader } from "@/components/page-header"
import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { useGo2RTC } from "@/homeclaw/hooks/use-go2rtc"

const STATUS_COLOR: Record<string, string> = {
  running: "text-green-500",
  starting: "text-yellow-500",
  restarting: "text-yellow-500",
  stopping: "text-orange-500",
  stopped: "text-muted-foreground",
  error: "text-destructive",
  unknown: "text-muted-foreground",
}

export function Go2RTCPage() {
  const { t } = useTranslation("homeclaw")
  const { state, loading, canStart, start, stop, restart } = useGo2RTC()

  const isTransient =
    state === "starting" || state === "restarting" || state === "stopping"
  const isRunning = state === "running"
  const isBusy = loading || isTransient

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("go2rtc.title")} />

      <div className="min-h-0 flex-1 overflow-y-auto px-4 sm:px-6">
        <div className="pt-2">
          <p className="text-muted-foreground text-sm">
            {t("go2rtc.description")}
          </p>
        </div>

        <Card className="mt-4">
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <IconVideo className="size-5" />
              {t("go2rtc.service.title")}
            </CardTitle>
            <CardDescription>{t("go2rtc.service.desc")}</CardDescription>
          </CardHeader>

          <CardContent>
            <div className="flex items-center gap-2 text-sm">
              <span className="text-muted-foreground">
                {t("go2rtc.status.label")}:
              </span>
              {isTransient ? (
                <span className="flex items-center gap-1 text-yellow-500">
                  <IconLoader2 className="size-3.5 animate-spin" />
                  {t(`go2rtc.status.${state}`)}
                </span>
              ) : (
                <span className={STATUS_COLOR[state] ?? "text-muted-foreground"}>
                  {t(`go2rtc.status.${state}`, { defaultValue: state })}
                </span>
              )}
            </div>
          </CardContent>

          <CardFooter className="flex flex-wrap gap-2">
            {/* Start */}
            <Button
              variant="default"
              disabled={!canStart || isBusy}
              onClick={() => void start()}
            >
              {loading && state === "starting" ? (
                <IconLoader2 className="mr-2 size-4 animate-spin" />
              ) : (
                <IconPlayerPlay className="mr-2 size-4" />
              )}
              {t("go2rtc.action.start")}
            </Button>

            {/* Stop */}
            <Button
              variant="outline"
              disabled={!isRunning || isBusy}
              onClick={() => void stop()}
            >
              {loading && state === "stopping" ? (
                <IconLoader2 className="mr-2 size-4 animate-spin" />
              ) : (
                <IconPlayerStop className="mr-2 size-4" />
              )}
              {t("go2rtc.action.stop")}
            </Button>

            {/* Restart */}
            <Button
              variant="outline"
              disabled={!isRunning || isBusy}
              onClick={() => void restart()}
            >
              {loading && state === "restarting" ? (
                <IconLoader2 className="mr-2 size-4 animate-spin" />
              ) : (
                <IconRefresh className="mr-2 size-4" />
              )}
              {t("go2rtc.action.restart")}
            </Button>
          </CardFooter>
        </Card>
      </div>
    </div>
  )
}

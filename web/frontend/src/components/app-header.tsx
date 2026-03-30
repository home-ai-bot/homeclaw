import {
  IconBook,
  IconLanguage,
  IconLoader2,
  IconMenu2,
  IconMoon,
  IconPlayerPlay,
  IconPower,
  IconRefresh,
  IconSun,
  IconVideo,
  IconVideoOff,
} from "@tabler/icons-react"
import { Link } from "@tanstack/react-router"
import * as React from "react"
import { useTranslation } from "react-i18next"

import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/ui/alert-dialog.tsx"
import { Button } from "@/components/ui/button.tsx"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu.tsx"
import { Separator } from "@/components/ui/separator.tsx"
import { SidebarTrigger } from "@/components/ui/sidebar"
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import { useGateway } from "@/hooks/use-gateway.ts"
import { useGo2RTC } from "@/homeclaw/hooks/use-go2rtc"
import { useTheme } from "@/hooks/use-theme.ts"

export function AppHeader() {
  const { i18n, t } = useTranslation()
  const { theme, toggleTheme } = useTheme()
  const {
    state: gwState,
    loading: gwLoading,
    canStart,
    restartRequired,
    start,
    restart,
    stop,
  } = useGateway()

  const {
    state: videoState,
    loading: videoLoading,
    canStart: videoCanStart,
    start: videoStart,
    restart: videoRestart,
    stop: videoStop,
  } = useGo2RTC()

  const isRunning = gwState === "running"
  const isStarting = gwState === "starting"
  const isRestarting = gwState === "restarting"
  const isStopping = gwState === "stopping"
  const isStopped = gwState === "stopped" || gwState === "unknown"
  const showNotConnectedHint =
    !isRestarting &&
    !isStopping &&
    canStart &&
    (gwState === "stopped" || gwState === "error")

  const videoIsRunning = videoState === "running"
  const videoIsStarting = videoState === "starting"
  const videoIsRestarting = videoState === "restarting"
  const videoIsStopping = videoState === "stopping"
  const videoIsStopped = videoState === "stopped" || videoState === "unknown"

  const [showStopDialog, setShowStopDialog] = React.useState(false)
  const [showVideoStopDialog, setShowVideoStopDialog] = React.useState(false)

  const handleGatewayToggle = () => {
    if (gwLoading || isRestarting || isStopping || (!isRunning && !canStart)) {
      return
    }
    if (isRunning) {
      setShowStopDialog(true)
    } else {
      void start()
    }
  }

  const handleGatewayRestart = () => {
    if (gwLoading || isRestarting || !restartRequired || !canStart) return
    void restart()
  }

  const confirmStop = () => {
    setShowStopDialog(false)
    stop()
  }

  const handleVideoToggle = () => {
    if (videoLoading || videoIsRestarting || videoIsStopping || (!videoIsRunning && !videoCanStart)) {
      return
    }
    if (videoIsRunning) {
      setShowVideoStopDialog(true)
    } else {
      void videoStart()
    }
  }

  const handleVideoRestart = () => {
    if (videoLoading || videoIsRestarting || !videoIsRunning) return
    void videoRestart()
  }

  const confirmVideoStop = () => {
    setShowVideoStopDialog(false)
    videoStop()
  }

  return (
    <header className="bg-background/95 supports-backdrop-filter:bg-background/60 border-b-border/50 sticky top-0 z-50 flex h-14 shrink-0 items-center justify-between border-b px-4 backdrop-blur">
      <div className="flex items-center gap-2">
        <SidebarTrigger className="text-muted-foreground hover:bg-accent hover:text-foreground flex h-9 w-9 items-center justify-center rounded-lg sm:hidden [&>svg]:size-5">
          <IconMenu2 />
        </SidebarTrigger>
        <div className="hidden w-36 shrink-0 items-center sm:flex">
          <Link to="/">
            <img className="w-full" src="/logo_with_text.png" alt="Logo" />
          </Link>
        </div>
      </div>

      {/* Center prominent connection status */}
      <div className="pointer-events-none absolute left-1/2 hidden h-full -translate-x-1/2 items-center justify-center lg:flex">
        {showNotConnectedHint && (
          <div className="text-muted-foreground flex items-center gap-2 rounded-full border border-dashed px-4 py-1.5 text-xs shadow-sm backdrop-blur-md">
            <span className="bg-destructive/50 relative flex size-2 shrink-0 items-center justify-center rounded-full">
              <span className="bg-destructive absolute inline-flex size-full animate-ping rounded-full opacity-75"></span>
            </span>
            {t("chat.notConnected")}
          </div>
        )}
      </div>

      <AlertDialog open={showStopDialog} onOpenChange={setShowStopDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("header.gateway.stopDialog.title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("header.gateway.stopDialog.description")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={confirmStop}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {t("header.gateway.stopDialog.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <AlertDialog open={showVideoStopDialog} onOpenChange={setShowVideoStopDialog}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>
              {t("header.go2rtc.stopDialog.title")}
            </AlertDialogTitle>
            <AlertDialogDescription>
              {t("header.go2rtc.stopDialog.description")}
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{t("common.cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={confirmVideoStop}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              {t("header.go2rtc.stopDialog.confirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      <div className="text-muted-foreground flex items-center gap-1 text-sm font-medium md:gap-2">
        {restartRequired && (
          <Tooltip delayDuration={700}>
            <TooltipTrigger asChild>
              <Button
                variant="secondary"
                size="icon-sm"
                className="bg-amber-500/15 text-amber-700 hover:bg-amber-500/25 hover:text-amber-800 dark:text-amber-300 dark:hover:bg-amber-500/25"
                onClick={handleGatewayRestart}
                disabled={gwLoading || isRestarting || isStopping || !canStart}
                aria-label={t("header.gateway.action.restart")}
              >
                <IconRefresh className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>
              {t("header.gateway.restartRequired")}
            </TooltipContent>
          </Tooltip>
        )}

        {/* Gateway Start/Stop */}
        {isRunning ? (
          <Tooltip delayDuration={700}>
            <TooltipTrigger asChild>
              <Button
                variant="destructive"
                size="icon-sm"
                className="size-8"
                onClick={handleGatewayToggle}
                disabled={gwLoading}
                aria-label={t("header.gateway.action.stop")}
              >
                <IconPower className="h-4 w-4 opacity-80" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("header.gateway.action.stop")}</TooltipContent>
          </Tooltip>
        ) : (
          <Button
            variant={
              isStarting || isRestarting || isStopping ? "secondary" : "default"
            }
            size="sm"
            className={`h-8 gap-2 px-3 ${
              isStopped ? "bg-green-500 text-white hover:bg-green-600" : ""
            }`}
            onClick={handleGatewayToggle}
            disabled={
              gwLoading || isStarting || isRestarting || isStopping || !canStart
            }
          >
            {gwLoading || isStarting || isRestarting || isStopping ? (
              <IconLoader2 className="h-4 w-4 animate-spin opacity-70" />
            ) : (
              <IconPlayerPlay className="h-4 w-4 opacity-80" />
            )}
            <span className="text-xs font-semibold">
              {isStopping
                ? t("header.gateway.status.stopping")
                : isRestarting
                  ? t("header.gateway.status.restarting")
                  : isStarting
                    ? t("header.gateway.status.starting")
                    : t("header.gateway.action.start")}
            </span>
          </Button>
        )}

        {/* Video Start/Restart/Stop */}
        {videoIsRunning && (
          <Tooltip delayDuration={700}>
            <TooltipTrigger asChild>
              <Button
                variant="secondary"
                size="icon-sm"
                className="size-8 bg-amber-500/15 text-amber-700 hover:bg-amber-500/25 hover:text-amber-800 dark:text-amber-300 dark:hover:bg-amber-500/25"
                onClick={handleVideoRestart}
                disabled={videoLoading || videoIsRestarting || videoIsStopping}
                aria-label={t("header.go2rtc.action.restart")}
              >
                <IconRefresh className="size-4" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("header.go2rtc.action.restart")}</TooltipContent>
          </Tooltip>
        )}

        {videoIsRunning ? (
          <Tooltip delayDuration={700}>
            <TooltipTrigger asChild>
              <Button
                variant="destructive"
                size="icon-sm"
                className="size-8"
                onClick={handleVideoToggle}
                disabled={videoLoading}
                aria-label={t("header.go2rtc.action.stop")}
              >
                <IconVideoOff className="h-4 w-4 opacity-80" />
              </Button>
            </TooltipTrigger>
            <TooltipContent>{t("header.go2rtc.action.stop")}</TooltipContent>
          </Tooltip>
        ) : (
          <Button
            variant={
              videoIsStarting || videoIsRestarting || videoIsStopping ? "secondary" : "outline"
            }
            size="sm"
            className={`h-8 gap-2 px-3 ${
              videoIsStopped ? "border-blue-500 text-blue-600 hover:bg-blue-50 dark:hover:bg-blue-500/10" : ""
            }`}
            onClick={handleVideoToggle}
            disabled={
              videoLoading || videoIsStarting || videoIsRestarting || videoIsStopping || !videoCanStart
            }
          >
            {videoLoading || videoIsStarting || videoIsRestarting || videoIsStopping ? (
              <IconLoader2 className="h-4 w-4 animate-spin opacity-70" />
            ) : (
              <IconVideo className="h-4 w-4 opacity-80" />
            )}
            <span className="text-xs font-semibold">
              {videoIsStopping
                ? t("header.go2rtc.status.stopping")
                : videoIsRestarting
                  ? t("header.go2rtc.status.restarting")
                  : videoIsStarting
                    ? t("header.go2rtc.status.starting")
                    : t("header.go2rtc.action.start")}
            </span>
          </Button>
        )}

        <Separator
          className="mx-4 my-2 hidden md:block"
          orientation="vertical"
        />

        {/* Docs Link */}
        <Button variant="ghost" size="icon" className="size-8" asChild>
          <a href="https://docs.picoclaw.io" target="_blank" rel="noreferrer">
            <IconBook className="size-4.5" />
          </a>
        </Button>

        {/* Language Switcher */}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button variant="ghost" size="icon" className="size-8">
              <IconLanguage className="size-4.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem onClick={() => i18n.changeLanguage("en")}>
              English
            </DropdownMenuItem>
            <DropdownMenuItem onClick={() => i18n.changeLanguage("zh")}>
              简体中文
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>

        {/* Theme Toggle */}
        <Button
          variant="ghost"
          size="icon"
          className="size-8"
          onClick={toggleTheme}
        >
          {theme === "dark" ? (
            <IconSun className="size-4.5" />
          ) : (
            <IconMoon className="size-4.5" />
          )}
        </Button>
      </div>
    </header>
  )
}

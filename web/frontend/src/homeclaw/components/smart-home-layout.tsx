import {
  IconLoader2,
  IconRefresh,
  IconMessage2,
  IconWifi,
  IconWifiOff,
  IconAlertCircle,
  IconTrash,
  IconChevronRight,
  IconSend,
  IconDownload,
  IconAlertTriangle,
  IconTool,
} from "@tabler/icons-react"
import type { ReactNode } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip"
import type { DeviceControlWSStatus } from "@/homeclaw/api/device-control-websocket"
import type { CommunicationLog } from "@/homeclaw/hooks/use-smart-home-websocket"

// ── WS Status Badge ────────────────────────────────────────────────────────

interface WsStatusBadgeProps {
  status: DeviceControlWSStatus
}

function WsStatusBadge({ status }: WsStatusBadgeProps) {
  const { t } = useTranslation("homeclaw")

  if (status === "connected") {
    return (
      <Badge variant="outline" className="gap-1 text-green-600 border-green-600">
        <IconWifi className="size-3" />
        {t("smart_home.ws.connected")}
      </Badge>
    )
  }
  if (status === "connecting") {
    return (
      <Badge variant="outline" className="gap-1 text-yellow-600 border-yellow-600">
        <IconLoader2 className="size-3 animate-spin" />
        {t("smart_home.ws.connecting")}
      </Badge>
    )
  }
  if (status === "error") {
    return (
      <Badge variant="destructive" className="gap-1">
        <IconAlertCircle className="size-3" />
        {t("smart_home.ws.error")}
      </Badge>
    )
  }
  return (
    <Badge variant="outline" className="gap-1 text-muted-foreground">
      <IconWifiOff className="size-3" />
      {t("smart_home.ws.disconnected")}
    </Badge>
  )
}

// ── Log Entry ──────────────────────────────────────────────────────────────

interface LogEntryProps {
  log: CommunicationLog
}

function LogEntry({ log }: LogEntryProps) {
  const formatTime = (timestamp: number) => {
    return new Date(timestamp).toLocaleTimeString()
  }

  const TypeIcon =
    log.type === "send"
      ? IconSend
      : log.type === "receive"
        ? IconDownload
        : log.type === "tool"
          ? IconTool
          : IconAlertTriangle

  const typeColor =
    log.type === "send"
      ? "text-blue-600"
      : log.type === "receive"
        ? "text-green-600"
        : log.type === "tool"
          ? "text-purple-600"
          : log.title.includes("失败") || log.title.includes("错误")
            ? "text-destructive"
            : "text-yellow-600"

  return (
    <div className="flex items-start gap-2 py-1.5 border-b last:border-0 text-xs">
      <TypeIcon className={`size-3.5 mt-0.5 shrink-0 ${typeColor}`} />
      <div className="min-w-0 flex-1">
        <span className="font-medium">{log.title}</span>
        {log.content && (
          <pre className="text-muted-foreground mt-0.5 break-all whitespace-pre-wrap font-mono text-[10px]">
            {log.content}
          </pre>
        )}
      </div>
      <span className="text-muted-foreground shrink-0">{formatTime(log.timestamp)}</span>
    </div>
  )
}

// ── Main Layout Component ──────────────────────────────────────────────────

interface SmartHomeLayoutProps {
  /** Page title (shown in PageHeader) */
  title: string
  /** WebSocket connection status */
  wsStatus: DeviceControlWSStatus
  /** Communication logs */
  logs: CommunicationLog[]
  /** Whether to show the log panel */
  showLogPanel: boolean
  /** Ref for log container (for auto-scroll) */
  logContainerRef?: React.RefObject<HTMLDivElement | null>
  /** Children content */
  children: ReactNode
  /** Refresh handler */
  onRefresh?: () => void
  /** Toggle log panel */
  onToggleLogPanel: () => void
  /** Clear logs */
  onClearLogs: () => void
  /** Whether currently loading */
  isLoading?: boolean
}

export function SmartHomeLayout({
  title,
  wsStatus,
  logs,
  showLogPanel,
  logContainerRef,
  children,
  onRefresh,
  onToggleLogPanel,
  onClearLogs,
  isLoading = false,
}: SmartHomeLayoutProps) {
  return (
    <div className="flex h-full">
      {/* Main content area */}
      <div className={`flex h-full flex-col transition-all duration-300 ${
        showLogPanel ? "flex-1 mr-80" : "flex-1"
      }`}>
        {/* Header with WS status, refresh, and log toggle */}
        <div className="flex items-center gap-3 px-4 sm:px-6 pt-4 pb-2">
          <h1 className="text-xl font-semibold flex-1">{title}</h1>
          
          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <WsStatusBadge status={wsStatus} />
              </TooltipTrigger>
              <TooltipContent>
                <p>WebSocket 连接状态</p>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>

          {onRefresh && (
            <TooltipProvider>
              <Tooltip>
                <TooltipTrigger asChild>
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={onRefresh}
                    disabled={isLoading}
                  >
                    {isLoading ? (
                      <IconLoader2 className="size-4 animate-spin" />
                    ) : (
                      <IconRefresh className="size-4" />
                    )}
                  </Button>
                </TooltipTrigger>
                <TooltipContent>
                  <p>刷新</p>
                </TooltipContent>
              </Tooltip>
            </TooltipProvider>
          )}

          <TooltipProvider>
            <Tooltip>
              <TooltipTrigger asChild>
                <Button
                  variant={showLogPanel ? "default" : "ghost"}
                  size="sm"
                  onClick={onToggleLogPanel}
                  className="relative"
                >
                  <IconMessage2 className="size-4" />
                  {logs.length > 0 && (
                    <span className="absolute -top-1 -right-1 size-4 bg-red-500 rounded-full text-white text-[10px] flex items-center justify-center">
                      {logs.length > 9 ? "9+" : logs.length}
                    </span>
                  )}
                </Button>
              </TooltipTrigger>
              <TooltipContent>
                <p>通信记录 ({logs.length})</p>
              </TooltipContent>
            </Tooltip>
          </TooltipProvider>
        </div>

        {/* Page content (scrollable) */}
        <div className="min-h-0 flex-1 overflow-y-auto px-4 sm:px-6">
          {children}
        </div>
      </div>

      {/* Right side log panel (collapsible) */}
      {showLogPanel && (
        <div className="fixed right-0 top-0 bottom-0 w-80 border-l bg-background flex flex-col shadow-lg z-50">
          {/* Log panel header */}
          <div className="flex items-center justify-between px-4 py-3 border-b">
            <div className="flex items-center gap-2">
              <IconMessage2 className="size-4" />
              <span className="font-medium text-sm">通信记录</span>
              {logs.length > 0 && (
                <Badge variant="secondary" className="text-xs">
                  {logs.length}
                </Badge>
              )}
            </div>
            <div className="flex items-center gap-1">
              {logs.length > 0 && (
                <TooltipProvider>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <Button
                        variant="ghost"
                        size="sm"
                        className="h-6 w-6 p-0"
                        onClick={onClearLogs}
                      >
                        <IconTrash className="size-3.5" />
                      </Button>
                    </TooltipTrigger>
                    <TooltipContent>
                      <p>清空记录</p>
                    </TooltipContent>
                  </Tooltip>
                </TooltipProvider>
              )}
              <TooltipProvider>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant="ghost"
                      size="sm"
                      className="h-6 w-6 p-0"
                      onClick={onToggleLogPanel}
                    >
                      <IconChevronRight className="size-3.5" />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>
                    <p>关闭</p>
                  </TooltipContent>
                </Tooltip>
              </TooltipProvider>
            </div>
          </div>

          {/* Log entries */}
          <div
            ref={logContainerRef}
            className="flex-1 overflow-y-auto px-3 py-2"
          >
            {logs.length === 0 ? (
              <div className="flex h-full items-center justify-center text-xs text-muted-foreground text-center px-4">
                暂无通信记录
              </div>
            ) : (
              <>
                {logs.map((log) => (
                  <LogEntry key={log.id} log={log} />
                ))}
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

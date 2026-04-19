import { IconLoader2 } from "@tabler/icons-react"
import { useStore } from "jotai"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import {
  tuyaAtom,
  fetchTuyaRegions,
  fetchTuyaStatus,
  tuyaLogin,
  tuyaLogout,
  tuyaDeleteCredentials,
  tuyaSaveToken,
  tuyaDeleteToken,
} from "@/homeclaw/store/tuya"
import { useSmartHomeWebSocket } from "@/homeclaw/hooks/use-smart-home-websocket"
import { SmartHomeLayout } from "@/homeclaw/components/smart-home-layout"

export function TuyaPage() {
  const { t } = useTranslation("homeclaw")
  const store = useStore()

  const [state, setState] = useState(store.get(tuyaAtom))
  // Token form state
  const [token, setToken] = useState("")
  const [isSavingToken, setIsSavingToken] = useState(false)
  const [tokenError, setTokenError] = useState<string | null>(null)
  // Login form state
  const [region, setRegion] = useState("")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [isLoggingIn, setIsLoggingIn] = useState(false)
  const [loginError, setLoginError] = useState<string | null>(null)

  // Use shared smart home WebSocket hook
  const {
    wsStatus,
    logs,
    showLogPanel,
    logContainerRef,
    clearLogs,
    toggleLogPanel,
  } = useSmartHomeWebSocket()

  useEffect(() => {
    const unsub = store.sub(tuyaAtom, () => {
      setState(store.get(tuyaAtom))
    })
    return unsub
  }, [store])

  useEffect(() => {
    const init = async () => {
      const regions = await fetchTuyaRegions()
      const status = await fetchTuyaStatus()
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        regions,
        ...status,
      }))
    }
    void init()
  }, [store])

  const handleSaveToken = async () => {
    if (!token.trim()) {
      setTokenError(t("tuya.validation.tokenRequired"))
      return
    }

    setIsSavingToken(true)
    setTokenError(null)

    const result = await tuyaSaveToken(token.trim())
    setIsSavingToken(false)

    if (result.success) {
      const status = await fetchTuyaStatus()
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        ...status,
      }))
      setToken("")
    } else {
      setTokenError(result.error || t("tuya.token.saveError"))
    }
  }

  const handleDeleteToken = async () => {
    await tuyaDeleteToken()
    const status = await fetchTuyaStatus()
    store.set(tuyaAtom, (prev) => ({
      ...prev,
      ...status,
    }))
  }

  const handleLogin = async () => {
    if (!region || !username.trim() || !password.trim()) {
      setLoginError(t("tuya.validation.required"))
      return
    }

    setIsLoggingIn(true)
    setLoginError(null)

    const result = await tuyaLogin(region, username.trim(), password)
    setIsLoggingIn(false)

    if (result.success) {
      const status = await fetchTuyaStatus()
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        ...status,
      }))
      setUsername("")
      setPassword("")
    } else {
      setLoginError(result.error || t("tuya.login.error"))
    }
  }

  const handleLogout = async () => {
    await tuyaLogout()
    const status = await fetchTuyaStatus()
    store.set(tuyaAtom, (prev) => ({
      ...prev,
      ...status,
    }))
  }

  const handleDeleteCredentials = async () => {
    await tuyaDeleteCredentials()
    const status = await fetchTuyaStatus()
    store.set(tuyaAtom, (prev) => ({
      ...prev,
      ...status,
    }))
  }

  const handleRefresh = async () => {
    const status = await fetchTuyaStatus()
    store.set(tuyaAtom, (prev) => ({
      ...prev,
      ...status,
    }))
  }

  // Check if connected via token
  const isTokenConnected = state.authType === "token"
  // Check if connected via credentials
  const isCredentialsConnected = state.authType === "credentials"

  return (
    <SmartHomeLayout
      title={t("navigation.tuya")}
      wsStatus={wsStatus}
      logs={logs}
      showLogPanel={showLogPanel}
      logContainerRef={logContainerRef}
      onRefresh={handleRefresh}
      onToggleLogPanel={toggleLogPanel}
      onClearLogs={clearLogs}
      isLoading={state.isLoading}
    >
      <div className="pt-2">
        <p className="text-muted-foreground text-sm">
          {t("tuya.description")}
        </p>
      </div>

      {state.isLoading ? (
        <div className="text-muted-foreground flex items-center gap-2 py-10 text-sm">
          <IconLoader2 className="size-4 animate-spin" />
          {t("labels.loading")}
        </div>
      ) : (
        <>
          {/* Token Section */}
          <Card className="mt-4">
            <CardHeader>
              <CardTitle>{t("tuya.token.title")}</CardTitle>
              <CardDescription>{t("tuya.token.description")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              {isTokenConnected ? (
                <div className="space-y-2">
                  <div className="text-sm">
                    <span className="text-muted-foreground">
                      {t("tuya.field.authType")}:
                    </span>{" "}
                    <span className="font-medium">{t("tuya.authType.token")}</span>
                  </div>
                </div>
              ) : (
                <div className="space-y-2">
                  <Label htmlFor="token">{t("tuya.field.token")}</Label>
                  <Input
                    id="token"
                    type="password"
                    placeholder={t("tuya.field.tokenPlaceholder")}
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void handleSaveToken()
                    }}
                  />
                </div>
              )}
              {tokenError && (
                <div className="text-destructive text-sm">{tokenError}</div>
              )}
            </CardContent>
            <CardFooter>
              {isTokenConnected ? (
                <Button variant="destructive" onClick={() => void handleDeleteToken()}>
                  {t("tuya.action.deleteToken")}
                </Button>
              ) : (
                <Button onClick={() => void handleSaveToken()} disabled={isSavingToken}>
                  {isSavingToken ? (
                    <>
                      <IconLoader2 className="mr-2 size-4 animate-spin" />
                      {t("tuya.token.saving")}
                    </>
                  ) : (
                    t("tuya.token.save")
                  )}
                </Button>
              )}
            </CardFooter>
          </Card>

          {/* Login Section for RTSP */}
          <Card className="mt-4">
            <CardHeader>
              <CardTitle>{t("tuya.login.title")}</CardTitle>
              <CardDescription>{t("tuya.login.descriptionRtsp")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="rounded-md border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
                {t("tuya.login.overseasNote")}
              </div>
              {isCredentialsConnected ? (
                <div className="space-y-2">
                  <div className="text-sm">
                    <span className="text-muted-foreground">
                      {t("tuya.field.region")}:
                    </span>{" "}
                    <span className="font-medium">{state.region}</span>
                  </div>
                </div>
              ) : (
                <>
                  <div className="space-y-2">
                    <Label htmlFor="region">{t("tuya.field.region")}</Label>
                    <Select value={region} onValueChange={setRegion}>
                      <SelectTrigger>
                        <SelectValue placeholder={t("tuya.field.selectRegion")} />
                      </SelectTrigger>
                      <SelectContent>
                        {state.regions.map((r) => (
                          <SelectItem key={r.name} value={r.name}>
                            {r.description} ({r.name})
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="email">{t("tuya.field.email")}</Label>
                    <Input
                      id="email"
                      type="email"
                      placeholder={t("tuya.field.emailPlaceholder")}
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                    />
                  </div>
                  <div className="space-y-2">
                    <Label htmlFor="password">{t("tuya.field.password")}</Label>
                    <Input
                      id="password"
                      type="password"
                      placeholder={t("tuya.field.passwordPlaceholder")}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") void handleLogin()
                      }}
                    />
                  </div>
                </>
              )}
              {loginError && (
                <div className="text-destructive text-sm">{loginError}</div>
              )}
            </CardContent>
            <CardFooter className="flex gap-2">
              {isCredentialsConnected ? (
                <>
                  <Button variant="outline" onClick={() => void handleLogout()}>
                    {t("tuya.action.logout")}
                  </Button>
                  <Button variant="destructive" onClick={() => void handleDeleteCredentials()}>
                    {t("tuya.action.deleteCredentials")}
                  </Button>
                </>
              ) : (
                <Button onClick={() => void handleLogin()} disabled={isLoggingIn}>
                  {isLoggingIn ? (
                    <>
                      <IconLoader2 className="mr-2 size-4 animate-spin" />
                      {t("tuya.login.loggingIn")}
                    </>
                  ) : (
                    t("tuya.login.submit")
                  )}
                </Button>
              )}
            </CardFooter>
          </Card>
        </>
      )}
    </SmartHomeLayout>
  )
}

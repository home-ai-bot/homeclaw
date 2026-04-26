import { IconLoader2, IconCircleCheck } from "@tabler/icons-react"
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
  xiaomiAtom,
  fetchXiaomiStatus,
  xiaomiAuthLogin,
  xiaomiAuthCaptcha,
  xiaomiAuthVerify,
  xiaomiLogoutAction,
  resetLoginStep,
  syncXiaomiHomes,
  selectXiaomiHome,
  syncXiaomiDevices,
  loadXiaomiHomes,
  loadXiaomiDevices,
} from "@/homeclaw/store/xiaomi"
import { useDeviceControl } from "@/homeclaw/context/device-control-context"
import { SmartHomeLayout } from "@/homeclaw/components/smart-home-layout"
import { HomeSection } from "@/homeclaw/components/home-section"
import { DeviceListSection } from "@/homeclaw/components/device-list-section"
import { VideoSettingsSection } from "@/homeclaw/components/video-settings-section"
import { callTool } from "@/homeclaw/api/device-command-executor"

export function XiaomiPage() {
  const { t } = useTranslation("homeclaw")
  const store = useStore()
  const [initialized, setInitialized] = useState(false)

  const [state, setState] = useState(store.get(xiaomiAtom))
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [captcha, setCaptcha] = useState("")
  const [verify, setVerify] = useState("")
  const [isLoggingIn, setIsLoggingIn] = useState(false)
  const [isGeneratingOps, setIsGeneratingOps] = useState(false)
  const [isClearingOps, setIsClearingOps] = useState(false)

  const {
    wsStatus,
  } = useDeviceControl()

  useEffect(() => {
    const unsub = store.sub(xiaomiAtom, () => {
      setState(store.get(xiaomiAtom))
    })
    return unsub
  }, [store])

  useEffect(() => {
    // Wait for WebSocket to be connected before loading data
    // Only run initialization once
    if (initialized || wsStatus !== "connected") {
      if (wsStatus !== "connected") {
        console.log('[XiaomiPage] Waiting for WebSocket connection, current status:', wsStatus)
      }
      return
    }

    const initialize = async () => {
      console.log('[XiaomiPage] WebSocket connected, starting initialization')
      const status = await fetchXiaomiStatus()
      console.log('[XiaomiPage] Initial status:', status)
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        ...status,
      }))

      // Always load existing homes from backend
      const homes = await loadXiaomiHomes()
      console.log('[XiaomiPage] Loaded homes:', homes)
      // Only select home if one is marked as current (user must explicitly choose otherwise)
      const currentHome = homes.find((h) => h.current) || null
      console.log('[XiaomiPage] Selected home:', currentHome)
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        homes,
        selectedHomeId: currentHome?.id || null,
      }))

      // Load devices if logged in and have selected home
      if (status.isLoggedIn && currentHome?.id) {
        console.log('[XiaomiPage] Loading devices for home:', currentHome.id)
        const devices = await loadXiaomiDevices()
        console.log('[XiaomiPage] Loaded devices:', devices)
        store.set(xiaomiAtom, (prev) => ({
          ...prev,
          devices,
        }))
      }

      setInitialized(true)
    }

    void initialize()
  }, [store, wsStatus, initialized])

  const handleLogin = async () => {
    if (!username || !password) {
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        error: t("xiaomi.validation.required"),
      }))
      return
    }

    setIsLoggingIn(true)
    store.set(xiaomiAtom, (prev) => ({ ...prev, error: null }))

    const result = await xiaomiAuthLogin(username, password)
    setIsLoggingIn(false)

    if (result.success) {
      const status = await fetchXiaomiStatus()
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        ...status,
        loginStep: "login",
      }))
      setPassword("")
    } else {
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        error: result.error || null,
        loginStep: result.step || "login",
        captchaImage: result.captchaImage || null,
        verifyTarget: result.verifyTarget || null,
        verifyType: result.verifyType || null,
      }))
    }
  }

  const handleCaptcha = async () => {
    if (!captcha) {
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        error: t("xiaomi.validation.captchaRequired"),
      }))
      return
    }

    setIsLoggingIn(true)
    store.set(xiaomiAtom, (prev) => ({ ...prev, error: null }))

    const result = await xiaomiAuthCaptcha(captcha)
    setIsLoggingIn(false)

    if (result.success) {
      const status = await fetchXiaomiStatus()
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        ...status,
        loginStep: "login",
      }))
      setCaptcha("")
    } else {
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        error: result.error || null,
        loginStep: result.step || "login",
        captchaImage: result.captchaImage || null,
        verifyTarget: result.verifyTarget || null,
        verifyType: result.verifyType || null,
      }))
    }
  }

  const handleVerify = async () => {
    if (!verify) {
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        error: t("xiaomi.validation.verifyRequired"),
      }))
      return
    }

    setIsLoggingIn(true)
    store.set(xiaomiAtom, (prev) => ({ ...prev, error: null }))

    const result = await xiaomiAuthVerify(verify)
    setIsLoggingIn(false)

    if (result.success) {
      const status = await fetchXiaomiStatus()
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        ...status,
        loginStep: "login",
      }))
      setVerify("")
    } else {
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        error: result.error || null,
        loginStep: result.step || "login",
        captchaImage: result.captchaImage || null,
        verifyTarget: result.verifyTarget || null,
        verifyType: result.verifyType || null,
      }))
    }
  }

  const handleLogout = async () => {
    await xiaomiLogoutAction()
    store.set(xiaomiAtom, (prev) => ({
      ...prev,
      isLoggedIn: false,
      userId: null,
      homes: [],
      selectedHomeId: null,
      devices: [],
    }))
  }

  const handleReset = () => {
    store.set(xiaomiAtom, (prev) => ({
      ...prev,
      ...resetLoginStep(),
    }))
    setCaptcha("")
    setVerify("")
  }

  const handleSyncHomes = async () => {
    store.set(xiaomiAtom, (prev) => ({ ...prev, isSyncingHomes: true, error: null }))
    const result = await syncXiaomiHomes()
    // Reload homes from backend after sync
    const homes = await loadXiaomiHomes()
    // Preserve current selection: find the previously selected home in new list, or keep existing current
    const prevSelectedId = store.get(xiaomiAtom).selectedHomeId
    const currentHome = homes.find((h) => h.id === prevSelectedId) || homes.find((h) => h.current) || null
    store.set(xiaomiAtom, (prev) => ({
      ...prev,
      isSyncingHomes: false,
      homes,
      selectedHomeId: currentHome?.id || null,
      error: result.error || null,
    }))
    
    // Load devices for the selected home after sync
    if (currentHome?.id) {
      const devices = await loadXiaomiDevices()
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        devices,
      }))
    }
  }

  const handleSelectHome = async (homeId: string) => {
    store.set(xiaomiAtom, (prev) => ({ ...prev, selectedHomeId: homeId }))
    const result = await selectXiaomiHome(homeId)
    if (result.error) {
      store.set(xiaomiAtom, (prev) => ({ ...prev, error: result.error || null }))
      return
    }
    // Load devices for the newly selected home
    const devices = await loadXiaomiDevices()
    store.set(xiaomiAtom, (prev) => ({ ...prev, devices }))
  }

  const handleSyncDevices = async () => {
    if (!state.selectedHomeId) return
    store.set(xiaomiAtom, (prev) => ({ ...prev, isSyncingDevices: true, error: null }))
    const result = await syncXiaomiDevices(state.selectedHomeId)
    const devices = await loadXiaomiDevices()
    store.set(xiaomiAtom, (prev) => ({
      ...prev,
      isSyncingDevices: false,
      devices,
      error: result.error || null,
    }))
  }

  const handleGenerateOps = async () => {
    if (!state.selectedHomeId) return
    setIsGeneratingOps(true)
    try {
      // Call hc_llm batchAnalyzeDevicesAsync to generate operations for all devices without ops
      // Async method starts analysis in background and returns immediately
      const result = await callTool(
        {
          toolName: "hc_llm",
          method: "batchAnalyzeDevicesAsync",
          brand: "xiaomi",
          params: {
            brand: "xiaomi",
          },
        },
        {
          timeout: 10000, // 10 seconds is enough since it returns immediately
          successMessage: "设备操作分析已启动,请耐心等待分析完成",
        }
      )
  
      if (!result.success) {
        console.error("[XiaomiPage] Failed to start batch analyze devices:", result.error)
      }
    } finally {
      setIsGeneratingOps(false)
    }
  }
  
  const handleClearOps = async () => {
    if (!confirm("确定要清除小米品牌所有设备的操作配置吗?此操作不可撤销!")) {
      return
    }
    setIsClearingOps(true)
    try {
      const { clearDeviceOps } = await import("@/homeclaw/api/device-ops")
      const result = await clearDeviceOps("xiaomi")
        
      // Reload devices to reflect the cleared ops
      if (state.selectedHomeId) {
        const devices = await loadXiaomiDevices()
        store.set(xiaomiAtom, (prev) => ({
          ...prev,
          devices,
        }))
      }
        
      console.log("[XiaomiPage] Cleared ops:", result)
    } catch (error) {
      console.error("[XiaomiPage] Failed to clear ops:", error)
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        error: error instanceof Error ? error.message : "Failed to clear operations",
      }))
    } finally {
      setIsClearingOps(false)
    }
  }

  const renderLoginForm = () => (
    <Card className="mt-4">
      <CardHeader>
        <CardTitle>{t("xiaomi.login.title")}</CardTitle>
        <CardDescription>{t("xiaomi.login.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="space-y-2">
          <Label htmlFor="username">{t("xiaomi.field.username")}</Label>
          <Input
            id="username"
            type="text"
            placeholder={t("xiaomi.field.usernamePlaceholder")}
            value={username}
            onChange={(e) => setUsername(e.target.value)}
          />
        </div>

        <div className="space-y-2">
          <Label htmlFor="password">{t("xiaomi.field.password")}</Label>
          <Input
            id="password"
            type="password"
            placeholder={t("xiaomi.field.passwordPlaceholder")}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </div>

        {state.error && <div className="text-destructive text-sm">{state.error}</div>}
      </CardContent>
      <CardFooter>
        <Button onClick={handleLogin} disabled={isLoggingIn}>
          {isLoggingIn ? (
            <>
              <IconLoader2 className="mr-2 size-4 animate-spin" />
              {t("xiaomi.login.loggingIn")}
            </>
          ) : (
            t("xiaomi.login.submit")
          )}
        </Button>
      </CardFooter>
    </Card>
  )

  const renderCaptchaForm = () => (
    <Card className="mt-4">
      <CardHeader>
        <CardTitle>{t("xiaomi.captcha.title")}</CardTitle>
        <CardDescription>{t("xiaomi.captcha.description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {state.captchaImage && (
          <div className="flex justify-center">
            <img
              src={`data:image/jpeg;base64,${state.captchaImage}`}
              alt="Captcha"
              className="rounded border"
            />
          </div>
        )}
        <div className="space-y-2">
          <Label htmlFor="captcha">{t("xiaomi.field.captcha")}</Label>
          <Input
            id="captcha"
            type="text"
            placeholder={t("xiaomi.field.captchaPlaceholder")}
            value={captcha}
            onChange={(e) => setCaptcha(e.target.value)}
            className="max-w-[200px]"
          />
        </div>

        {state.error && <div className="text-destructive text-sm">{state.error}</div>}
      </CardContent>
      <CardFooter className="flex gap-2">
        <Button onClick={handleCaptcha} disabled={isLoggingIn}>
          {isLoggingIn ? (
            <>
              <IconLoader2 className="mr-2 size-4 animate-spin" />
              {t("xiaomi.captcha.submitting")}
            </>
          ) : (
            t("xiaomi.captcha.submit")
          )}
        </Button>
        <Button variant="outline" onClick={handleReset}>
          {t("xiaomi.action.cancel")}
        </Button>
      </CardFooter>
    </Card>
  )

  const renderVerifyForm = () => (
    <Card className="mt-4">
      <CardHeader>
        <CardTitle>{t("xiaomi.verify.title")}</CardTitle>
        <CardDescription>
          {state.verifyType === "phone"
            ? t("xiaomi.verify.descriptionPhone", { target: state.verifyTarget })
            : t("xiaomi.verify.descriptionEmail", { target: state.verifyTarget })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="rounded bg-muted p-3 text-center font-mono">
          {state.verifyTarget}
        </div>
        <div className="space-y-2">
          <Label htmlFor="verify">{t("xiaomi.field.verifyCode")}</Label>
          <Input
            id="verify"
            type="text"
            placeholder={t("xiaomi.field.verifyCodePlaceholder")}
            value={verify}
            onChange={(e) => setVerify(e.target.value)}
            className="max-w-[200px]"
          />
        </div>

        {state.error && <div className="text-destructive text-sm">{state.error}</div>}
      </CardContent>
      <CardFooter className="flex gap-2">
        <Button onClick={handleVerify} disabled={isLoggingIn}>
          {isLoggingIn ? (
            <>
              <IconLoader2 className="mr-2 size-4 animate-spin" />
              {t("xiaomi.verify.submitting")}
            </>
          ) : (
            t("xiaomi.verify.submit")
          )}
        </Button>
        <Button variant="outline" onClick={handleReset}>
          {t("xiaomi.action.cancel")}
        </Button>
      </CardFooter>
    </Card>
  )

  const renderAuthSection = () => {
    if (state.isLoggedIn) {
      return (
        <Card className="mt-4">
          <CardContent className="flex items-center justify-between py-3">
            <div className="flex items-center gap-2">
              <IconCircleCheck className="size-4 text-green-500" />
              {state.userId && (
                <span className="text-sm">
                  <span className="text-muted-foreground">
                    {t("xiaomi.field.userId")}:
                  </span>{" "}
                  <span className="font-medium">{state.userId}</span>
                </span>
              )}
            </div>
            <Button variant="outline" size="sm" className="h-8" onClick={() => void handleLogout()}>
              {t("xiaomi.action.logout")}
            </Button>
          </CardContent>
        </Card>
      )
    }

    if (state.loginStep === "captcha") {
      return renderCaptchaForm()
    }
    if (state.loginStep === "verify") {
      return renderVerifyForm()
    }
    return renderLoginForm()
  }

  return (
    <SmartHomeLayout
      title={t("navigation.xiaomi")}
      isLoading={state.isLoading}
    >
      {/* Section 1: Authorization */}
      {renderAuthSection()}

      {/* Section 2: Family Information (only show if logged in) */}
      {state.isLoggedIn && (
        <HomeSection
          homes={state.homes}
          selectedHomeId={state.selectedHomeId}
          isSyncing={state.isSyncingHomes}
          onSync={() => void handleSyncHomes()}
          onSelect={(id) => void handleSelectHome(id)}
        />
      )}

      {/* Section 3: Device List (always show when logged in) */}
      {state.isLoggedIn && (
        <DeviceListSection
          devices={state.devices}
          isSyncing={state.isSyncingDevices}
          onSync={() => void handleSyncDevices()}
          onGenerateOps={() => void handleGenerateOps()}
          isGeneratingOps={isGeneratingOps}
          onClearOps={() => void handleClearOps()}
          isClearingOps={isClearingOps}
          disabled={!state.selectedHomeId}
        />
      )}

      {/* Section 4: Video Settings (placeholder) */}
      <VideoSettingsSection />
    </SmartHomeLayout>
  )
}

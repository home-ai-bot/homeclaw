import { IconLoader2, IconCircleCheck } from "@tabler/icons-react"
import { useStore } from "jotai"
import { useEffect, useState } from "react"
import { useTranslation } from "react-i18next"

import { Button } from "@/components/ui/button"
import {
  Card,
  CardContent,
} from "@/components/ui/card"
import { Input } from "@/components/ui/input"
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
  syncTuyaHomes,
  selectTuyaHome,
  syncTuyaDevices,
  loadTuyaHomes,
  loadTuyaDevices,
} from "@/homeclaw/store/tuya"
import { SmartHomeLayout } from "@/homeclaw/components/smart-home-layout"
import { HomeSection } from "@/homeclaw/components/home-section"
import { DeviceListSection } from "@/homeclaw/components/device-list-section"
import { VideoSettingsSection } from "@/homeclaw/components/video-settings-section"
import { useDeviceControl } from "@/homeclaw/context/device-control-context"
import { callTool } from "@/homeclaw/api/device-command-executor"

export function TuyaPage() {
  const { t } = useTranslation("homeclaw")
  const store = useStore()
  const { wsStatus } = useDeviceControl()

  const [state, setState] = useState(store.get(tuyaAtom))
  const [initialized, setInitialized] = useState(false)
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
  const [isGeneratingOps, setIsGeneratingOps] = useState(false)
  const [isClearingOps, setIsClearingOps] = useState(false)

  useEffect(() => {
    const unsub = store.sub(tuyaAtom, () => {
      setState(store.get(tuyaAtom))
    })
    return unsub
  }, [store])

  useEffect(() => {
    // Load HTTP-based data immediately (regions, status)
    const loadHttpData = async () => {
      console.log('[TuyaPage] Loading HTTP data...')
      const regions = await fetchTuyaRegions()
      const status = await fetchTuyaStatus()
      console.log('[TuyaPage] Setting store with status:', status)
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        regions,
        ...status,
        isLoading: false, // Mark overall loading as complete
      }))
      console.log('[TuyaPage] Store updated, new state:', store.get(tuyaAtom))
    }
    void loadHttpData()
  }, [store])

  // Load homes and devices sequentially after WebSocket is connected
  useEffect(() => {
    if (initialized || wsStatus !== "connected") {
      return
    }

    const loadWsData = async () => {
      console.log('[TuyaPage] WebSocket connected, starting initialization')
      
      // Step 1: Check login status first (like Xiaomi does)
      const status = await fetchTuyaStatus()
      console.log('[TuyaPage] Login status:', status)
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        ...status,
      }))
      
      // Step 2: Load homes
      store.set(tuyaAtom, (prev) => ({ ...prev, isLoadingHomes: true }))
      try {
        const homesData = await loadTuyaHomes()
        console.log('[TuyaPage] Loaded homes:', homesData)
        const currentHome = homesData.find((h) => h.current) || null
        console.log('[TuyaPage] Selected home:', currentHome)
        store.set(tuyaAtom, (prev) => ({
          ...prev,
          homes: homesData,
          selectedHomeId: currentHome?.id || null,
          isLoadingHomes: false,
        }))

        // Step 3: Load devices only if logged in and have selected home (like Xiaomi)
        if (status.isLoggedIn && currentHome?.id) {
          console.log('[TuyaPage] Loading devices for home:', currentHome.id)
          store.set(tuyaAtom, (prev) => ({ ...prev, isLoadingDevices: true }))
          try {
            const devicesData = await loadTuyaDevices()
            console.log('[TuyaPage] Loaded devices:', devicesData)
            store.set(tuyaAtom, (prev) => ({
              ...prev,
              devices: devicesData,
              isLoadingDevices: false,
            }))
          } catch (error) {
            console.error("Failed to load devices:", error)
            store.set(tuyaAtom, (prev) => ({ ...prev, isLoadingDevices: false }))
          }
        }
      } catch (error) {
        console.error("Failed to load homes:", error)
        store.set(tuyaAtom, (prev) => ({ ...prev, isLoadingHomes: false }))
      }

      setInitialized(true)
    }
    void loadWsData()
  }, [store, wsStatus, initialized])

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
      homes: [],
      selectedHomeId: null,
      devices: [],
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

  const handleSyncHomes = async () => {
    store.set(tuyaAtom, (prev) => ({ ...prev, isSyncingHomes: true, error: null }))
    const result = await syncTuyaHomes()
    const homes = await loadTuyaHomes()
    // Preserve current selection: find the previously selected home in new list, or keep existing current
    const prevSelectedId = store.get(tuyaAtom).selectedHomeId
    const currentHome = homes.find((h) => h.id === prevSelectedId) || homes.find((h) => h.current) || null
    store.set(tuyaAtom, (prev) => ({
      ...prev,
      isSyncingHomes: false,
      homes,
      selectedHomeId: currentHome?.id || null,
      error: result.error || null,
    }))
    
    // Load devices for the selected home after sync
    if (currentHome?.id) {
      const devices = await loadTuyaDevices()
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        devices,
      }))
    }
  }

  const handleSelectHome = async (homeId: string) => {
    store.set(tuyaAtom, (prev) => ({ ...prev, selectedHomeId: homeId }))
    const result = await selectTuyaHome(homeId)
    if (result.error) {
      store.set(tuyaAtom, (prev) => ({ ...prev, error: result.error || null }))
      return
    }
    // Load devices for the newly selected home
    const devices = await loadTuyaDevices()
    store.set(tuyaAtom, (prev) => ({ ...prev, devices }))
  }

  const handleSyncDevices = async () => {
    if (!state.selectedHomeId) return
    store.set(tuyaAtom, (prev) => ({ ...prev, isSyncingDevices: true, error: null }))
    const result = await syncTuyaDevices(state.selectedHomeId)
    const devices = await loadTuyaDevices()
    store.set(tuyaAtom, (prev) => ({
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
          brand: "tuya",
          params: {
            brand: "tuya",
          },
        },
        {
          timeout: 10000, // 10 seconds is enough since it returns immediately
          successMessage: "设备操作分析已启动,请耐心等待分析完成",
        }
      )
  
      if (!result.success) {
        console.error("[TuyaPage] Failed to start batch analyze devices:", result.error)
      }
    } finally {
      setIsGeneratingOps(false)
    }
  }
  
  const handleClearOps = async () => {
    if (!confirm("确定要清除涂鸦品牌所有设备的操作配置吗?此操作不可撤销!")) {
      return
    }
    setIsClearingOps(true)
    try {
      const { clearDeviceOps } = await import("@/homeclaw/api/device-ops")
      const result = await clearDeviceOps("tuya")
        
      // Reload devices to reflect the cleared ops
      const devices = await loadTuyaDevices()
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        devices,
      }))
        
      console.log("[TuyaPage] Cleared ops:", result)
    } catch (error) {
      console.error("[TuyaPage] Failed to clear ops:", error)
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        error: error instanceof Error ? error.message : "Failed to clear operations",
      }))
    } finally {
      setIsClearingOps(false)
    }
  }

  // Check if connected via token
  const isTokenConnected = state.authType === "token"
  // Check if connected via credentials
  const isCredentialsConnected = state.authType === "credentials"

  // Debug logging
  console.log('[TuyaPage] Current state:', {
    isLoggedIn: state.isLoggedIn,
    authType: state.authType,
    isTokenConnected,
    isCredentialsConnected,
    isLoading: state.isLoading,
  })

  return (
    <SmartHomeLayout
      title={t("navigation.tuya")}
      isLoading={state.isLoading}
    >
      {/* Section 1: Authorization - Always shown */}
      <Card className="mt-4">
        <CardContent className="py-3">
          <div className="grid grid-cols-2 gap-4">
            {/* Token Auth Section */}
            <div className="space-y-2">
              <div className="text-sm font-medium">{t("tuya.token.title")}</div>
              {isTokenConnected ? (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <IconCircleCheck className="size-4 text-green-500" />
                    <span className="text-sm">{t("tuya.authType.token")}</span>
                  </div>
                  <Button variant="destructive" size="sm" className="h-7" onClick={() => void handleDeleteToken()}>
                    {t("tuya.action.deleteToken")}
                  </Button>
                </div>
              ) : (
                <div className="flex gap-2">
                  <Input
                    type="password"
                    placeholder={t("tuya.field.tokenPlaceholder")}
                    value={token}
                    onChange={(e) => setToken(e.target.value)}
                    onKeyDown={(e) => {
                      if (e.key === "Enter") void handleSaveToken()
                    }}
                    className="h-8"
                  />
                  <Button size="sm" className="h-8" onClick={() => void handleSaveToken()} disabled={isSavingToken}>
                    {isSavingToken ? (
                      <IconLoader2 className="size-4 animate-spin" />
                    ) : (
                      t("tuya.token.save")
                    )}
                  </Button>
                </div>
              )}
              {tokenError && (
                <div className="text-destructive text-xs">{tokenError}</div>
              )}
            </div>

            {/* Credentials Auth Section */}
            <div className="space-y-2">
              <div className="text-sm font-medium">{t("tuya.login.title")}</div>
              {isCredentialsConnected ? (
                <div className="flex items-center justify-between">
                  <div className="flex items-center gap-2">
                    <IconCircleCheck className="size-4 text-green-500" />
                    <span className="text-sm">{state.region}</span>
                  </div>
                  <div className="flex gap-2">
                    <Button variant="outline" size="sm" className="h-7" onClick={() => void handleLogout()}>
                      {t("tuya.action.logout")}
                    </Button>
                    <Button variant="destructive" size="sm" className="h-7" onClick={() => void handleDeleteCredentials()}>
                      {t("tuya.action.deleteCredentials")}
                    </Button>
                  </div>
                </div>
              ) : (
                <>
                  <div className="rounded-md border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800 dark:border-amber-800 dark:bg-amber-950 dark:text-amber-200">
                    {t("tuya.login.overseasNote")}
                  </div>
                  <div className="flex gap-2">
                    <Select value={region} onValueChange={setRegion}>
                      <SelectTrigger className="h-8">
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
                  <div className="flex gap-2">
                    <Input
                      type="email"
                      placeholder={t("tuya.field.emailPlaceholder")}
                      value={username}
                      onChange={(e) => setUsername(e.target.value)}
                      className="h-8"
                    />
                    <Input
                      type="password"
                      placeholder={t("tuya.field.passwordPlaceholder")}
                      value={password}
                      onChange={(e) => setPassword(e.target.value)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter") void handleLogin()
                      }}
                      className="h-8"
                    />
                  </div>
                  <Button size="sm" className="h-8" onClick={() => void handleLogin()} disabled={isLoggingIn}>
                    {isLoggingIn ? (
                      <IconLoader2 className="size-4 animate-spin" />
                    ) : (
                      t("tuya.login.submit")
                    )}
                  </Button>
                  {loginError && (
                    <div className="text-destructive text-xs">{loginError}</div>
                  )}
                </>
              )}
            </div>
          </div>
        </CardContent>
      </Card>

      {/* Section 2: Family Information - Always shown */}
      <HomeSection
        homes={state.homes}
        selectedHomeId={state.selectedHomeId}
        isSyncing={state.isSyncingHomes}
        isLoading={state.isLoadingHomes}
        onSync={() => void handleSyncHomes()}
        onSelect={(id) => void handleSelectHome(id)}
      />

      {/* Section 3: Device List - Always shown */}
      <DeviceListSection
        devices={state.devices}
        isSyncing={state.isSyncingDevices}
        isLoading={state.isLoadingDevices}
        onSync={() => void handleSyncDevices()}
        onGenerateOps={() => void handleGenerateOps()}
        isGeneratingOps={isGeneratingOps}
        onClearOps={() => void handleClearOps()}
        isClearingOps={isClearingOps}
        disabled={!state.selectedHomeId}
      />

      {/* Section 4: Video Settings (placeholder) */}
      <VideoSettingsSection />
    </SmartHomeLayout>
  )
}

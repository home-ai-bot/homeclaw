import { IconLoader2 } from "@tabler/icons-react"
import { useStore } from "jotai"
import { useEffect, useState } from "react"
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Input } from "@/components/ui/input"
import { Label } from "@/components/ui/label"
import {
  tuyaAtom,
  fetchTuyaRegions,
  fetchTuyaStatus,
  tuyaLogin,
  tuyaLogout,
  tuyaDeleteCredentials,
} from "@/homeclaw/store/tuya"

export function TuyaPage() {
  const { t } = useTranslation()
  const store = useStore()

  const [state, setState] = useState(store.get(tuyaAtom))
  const [region, setRegion] = useState("")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [isLoggingIn, setIsLoggingIn] = useState(false)
  const [loginError, setLoginError] = useState<string | null>(null)

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
      if (status.region) {
        setRegion(status.region)
      }
    }
    void init()
  }, [store])

  const handleLogin = async () => {
    if (!region || !username || !password) {
      setLoginError(t("tuya.validation.required"))
      return
    }

    setIsLoggingIn(true)
    setLoginError(null)

    const result = await tuyaLogin(region, username, password)
    setIsLoggingIn(false)

    if (result.success) {
      const status = await fetchTuyaStatus()
      store.set(tuyaAtom, (prev) => ({
        ...prev,
        ...status,
      }))
      setPassword("")
    } else {
      setLoginError(result.error || t("tuya.login.error"))
    }
  }

  const handleLogout = async () => {
    await tuyaLogout()
    store.set(tuyaAtom, (prev) => ({
      ...prev,
      isLoggedIn: false,
      user: null,
      region: null,
    }))
  }

  const handleDeleteCredentials = async () => {
    await tuyaDeleteCredentials()
    store.set(tuyaAtom, (prev) => ({
      ...prev,
      isLoggedIn: false,
      user: null,
      region: null,
    }))
  }

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.tuya")} />

      <div className="min-h-0 flex-1 overflow-y-auto px-4 sm:px-6">
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
        ) : state.isLoggedIn ? (
          <Card className="mt-4">
            <CardHeader>
              <CardTitle>{t("tuya.status.loggedIn")}</CardTitle>
              <CardDescription>
                {t("tuya.status.loggedInDesc")}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {state.region && (
                <div className="text-sm">
                  <span className="text-muted-foreground">
                    {t("tuya.field.region")}:
                  </span>{" "}
                  <span className="font-medium">{state.region}</span>
                </div>
              )}
            </CardContent>
            <CardFooter className="flex gap-2">
              <Button variant="outline" onClick={() => void handleLogout()}>
                {t("tuya.action.logout")}
              </Button>
              <Button
                variant="destructive"
                onClick={() => void handleDeleteCredentials()}
              >
                {t("tuya.action.deleteCredentials")}
              </Button>
            </CardFooter>
          </Card>
        ) : (
          <Card className="mt-4">
            <CardHeader>
              <CardTitle>{t("tuya.login.title")}</CardTitle>
              <CardDescription>{t("tuya.login.description")}</CardDescription>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="region">{t("tuya.field.region")}</Label>
                <Select value={region} onValueChange={setRegion}>
                  <SelectTrigger id="region">
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
                <Label htmlFor="username">{t("tuya.field.username")}</Label>
                <Input
                  id="username"
                  type="text"
                  placeholder={t("tuya.field.usernamePlaceholder")}
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
                />
              </div>

              {loginError && (
                <div className="text-destructive text-sm">{loginError}</div>
              )}
            </CardContent>
            <CardFooter>
              <Button onClick={handleLogin} disabled={isLoggingIn}>
                {isLoggingIn ? (
                  <>
                    <IconLoader2 className="mr-2 size-4 animate-spin" />
                    {t("tuya.login.loggingIn")}
                  </>
                ) : (
                  t("tuya.login.submit")
                )}
              </Button>
            </CardFooter>
          </Card>
        )}
      </div>
    </div>
  )
}

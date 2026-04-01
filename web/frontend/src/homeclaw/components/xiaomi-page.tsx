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
} from "@/homeclaw/store/xiaomi"

export function XiaomiPage() {
  const { t } = useTranslation()
  const store = useStore()

  const [state, setState] = useState(store.get(xiaomiAtom))
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [captcha, setCaptcha] = useState("")
  const [verify, setVerify] = useState("")
  const [isLoggingIn, setIsLoggingIn] = useState(false)

  useEffect(() => {
    const unsub = store.sub(xiaomiAtom, () => {
      setState(store.get(xiaomiAtom))
    })
    return unsub
  }, [store])

  useEffect(() => {
    const init = async () => {
      const status = await fetchXiaomiStatus()
      store.set(xiaomiAtom, (prev) => ({
        ...prev,
        ...status,
      }))
    }
    void init()
  }, [store])

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

  return (
    <div className="flex h-full flex-col">
      <PageHeader title={t("navigation.xiaomi")} />

      <div className="min-h-0 flex-1 overflow-y-auto px-4 sm:px-6">
        <div className="pt-2">
          <p className="text-muted-foreground text-sm">
            {t("xiaomi.description")}
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
              <CardTitle>{t("xiaomi.status.loggedIn")}</CardTitle>
              <CardDescription>
                {t("xiaomi.status.loggedInDesc")}
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-2">
              {state.userId && (
                <div className="text-sm">
                  <span className="text-muted-foreground">
                    {t("xiaomi.field.userId")}:
                  </span>{" "}
                  <span className="font-medium">{state.userId}</span>
                </div>
              )}
            </CardContent>
            <CardFooter>
              <Button variant="outline" onClick={() => void handleLogout()}>
                {t("xiaomi.action.logout")}
              </Button>
            </CardFooter>
          </Card>
        ) : state.loginStep === "captcha" ? (
          renderCaptchaForm()
        ) : state.loginStep === "verify" ? (
          renderVerifyForm()
        ) : (
          renderLoginForm()
        )}
      </div>
    </div>
  )
}


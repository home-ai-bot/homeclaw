import { IconLoader2, IconPlus, IconX } from "@tabler/icons-react"
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
  appleAtom,
  type HomeKitDevice,
} from "@/homeclaw/store/apple"
import {
  fetchHomeKitDevices,
  pairHomeKitDevice,
  unpairHomeKitDevice,
} from "@/homeclaw/api/apple"
import { SmartHomeLayout } from "@/homeclaw/components/smart-home-layout"

export function ApplePage() {
  const { t } = useTranslation("homeclaw")
  const store = useStore()

  const [state, setState] = useState(store.get(appleAtom))
  const [pairingDevice, setPairingDevice] = useState<HomeKitDevice | null>(null)
  const [pin, setPin] = useState("")
  const [isProcessing, setIsProcessing] = useState(false)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    const unsub = store.sub(appleAtom, () => {
      setState(store.get(appleAtom))
    })
    return unsub
  }, [store])

  useEffect(() => {
    void loadDevices()
  }, [])

  const loadDevices = async () => {
    store.set(appleAtom, (prev) => ({
      ...prev,
      isLoading: true,
      error: null,
    }))

    try {
      const { unpaired, paired } = await fetchHomeKitDevices()
      store.set(appleAtom, (prev) => ({
        ...prev,
        isLoading: false,
        unpairedDevices: unpaired,
        pairedDevices: paired,
      }))
    } catch (err) {
      store.set(appleAtom, (prev) => ({
        ...prev,
        isLoading: false,
        error: err instanceof Error ? err.message : "Failed to load devices",
      }))
    }
  }

  const handlePair = async (device: HomeKitDevice) => {
    if (!pin.trim()) {
      setError(t("apple.validation.pinRequired"))
      return
    }

    setIsProcessing(true)
    setError(null)

    try {
      await pairHomeKitDevice(device.id, device.ip, pin.trim())
      setPin("")
      setPairingDevice(null)
      await loadDevices()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to pair device")
    } finally {
      setIsProcessing(false)
    }
  }

  const handleUnpair = async (device: HomeKitDevice) => {
    if (!device.pairedId) return

    setIsProcessing(true)
    setError(null)

    try {
      await unpairHomeKitDevice(device.pairedId)
      await loadDevices()
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to unpair device")
    } finally {
      setIsProcessing(false)
    }
  }

  const renderDeviceCard = (device: HomeKitDevice, isPaired: boolean) => (
    <Card key={device.id} className="mb-3">
      <CardHeader className="pb-2">
        <CardTitle className="text-base">{device.name}</CardTitle>
        <CardDescription className="flex items-center gap-2">
          <span>{device.ip}</span>
          {device.info.status && (
            <span
              className={`inline-block h-2 w-2 rounded-full ${
                device.info.status === "1" ? "bg-amber-500" : "bg-green-500"
              }`}
            />
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className="pb-2 text-xs">
        <div className="space-y-1">
          {Object.entries(device.info)
            .filter(([key]) => key !== "status")
            .slice(0, 4)
            .map(([key, value]) => (
              <div key={key} className="flex">
                <span className="text-muted-foreground mr-2">{key}:</span>
                <span className="font-mono">{value}</span>
              </div>
            ))}
        </div>
      </CardContent>
      <CardFooter>
        {isPaired ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => void handleUnpair(device)}
            disabled={isProcessing}
          >
            <IconX className="mr-1 size-4" />
            {t("apple.action.unpair")}
          </Button>
        ) : (
          <Button
            size="sm"
            onClick={() => {
              setPairingDevice(device)
              setError(null)
            }}
          >
            <IconPlus className="mr-1 size-4" />
            {t("apple.action.pair")}
          </Button>
        )}
      </CardFooter>
    </Card>
  )

  const renderPairingDialog = () => {
    if (!pairingDevice) return null

    return (
      <Card className="fixed left-1/2 top-1/2 z-50 w-full max-w-md -translate-x-1/2 -translate-y-1/2 shadow-lg">
        <CardHeader>
          <CardTitle>{t("apple.pair.title")}</CardTitle>
          <CardDescription>
            {t("apple.pair.description", { name: pairingDevice.name })}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div className="space-y-2">
            <Label htmlFor="pin">{t("apple.field.pin")}</Label>
            <Input
              id="pin"
              type="text"
              placeholder={t("apple.field.pinPlaceholder")}
              value={pin}
              onChange={(e) => setPin(e.target.value)}
              maxLength={8}
            />
          </div>
          {error && <div className="text-destructive text-sm">{error}</div>}
        </CardContent>
        <CardFooter className="flex gap-2">
          <Button
            onClick={() => void handlePair(pairingDevice)}
            disabled={isProcessing}
          >
            {isProcessing ? (
              <>
                <IconLoader2 className="mr-2 size-4 animate-spin" />
                {t("apple.pair.pairing")}
              </>
            ) : (
              t("apple.action.pair")
            )}
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              setPairingDevice(null)
              setPin("")
              setError(null)
            }}
            disabled={isProcessing}
          >
            {t("apple.action.cancel")}
          </Button>
        </CardFooter>
      </Card>
    )
  }

  return (
    <SmartHomeLayout
      title={t("navigation.apple")}
      isLoading={state.isLoading}
    >
      <div className="pt-2">
        <p className="text-muted-foreground text-sm">
          {t("apple.description")}
        </p>
      </div>

      {state.isLoading ? (
        <div className="text-muted-foreground flex items-center gap-2 py-10 text-sm">
          <IconLoader2 className="size-4 animate-spin" />
          {t("labels.loading")}
        </div>
      ) : (
        <div className="mt-4 grid grid-cols-1 gap-4 md:grid-cols-2">
          {/* Unpaired Devices - Left Column */}
          <div>
            <h3 className="mb-3 text-lg font-semibold">
              {t("apple.unpaired.title")}
            </h3>
            {state.unpairedDevices.length === 0 ? (
              <div className="text-muted-foreground rounded border border-dashed p-8 text-center text-sm">
                {t("apple.unpaired.empty")}
              </div>
            ) : (
              <div className="max-h-[600px] overflow-y-auto pr-2">
                {state.unpairedDevices.map((device) =>
                  renderDeviceCard(device, false),
                )}
              </div>
            )}
          </div>

          {/* Paired Devices - Right Column */}
          <div>
            <h3 className="mb-3 text-lg font-semibold">
              {t("apple.paired.title")}
            </h3>
            {state.pairedDevices.length === 0 ? (
              <div className="text-muted-foreground rounded border border-dashed p-8 text-center text-sm">
                {t("apple.paired.empty")}
              </div>
            ) : (
              <div className="max-h-[600px] overflow-y-auto pr-2">
                {state.pairedDevices.map((device) =>
                  renderDeviceCard(device, true),
                )}
              </div>
            )}
          </div>
        </div>
      )}

      {error && !pairingDevice && (
        <div className="mt-4 text-destructive text-sm">{error}</div>
      )}

      {pairingDevice && renderPairingDialog()}
    </SmartHomeLayout>
  )
}

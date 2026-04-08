import type { HomeKitDevice } from "@/homeclaw/store/apple"

const API_BASE = "/api"

export interface DiscoveryResponse {
  sources: Array<{
    id: string
    name: string
    info: string
    url: string
    location: string
  }>
}

export async function fetchHomeKitDevices(): Promise<{
  unpaired: HomeKitDevice[]
  paired: HomeKitDevice[]
}> {
  const response = await fetch(`${API_BASE}/homekit/discovery`, {
    cache: "no-cache",
  })

  if (!response.ok) {
    throw new Error(await response.text())
  }

  const data: DiscoveryResponse = await response.json()

  const unpaired: HomeKitDevice[] = []
  const paired: HomeKitDevice[] = []

  for (const source of data.sources) {
    const isPaired = !source.info.includes("status=1")
    const device: HomeKitDevice = {
      id: source.id,
      name: source.name,
      ip: source.location || "",
      port: 0,
      info: parseInfoString(source.info),
      paired: isPaired,
      pairedId: isPaired ? source.url : undefined,
    }

    if (isPaired) {
      paired.push(device)
    } else {
      unpaired.push(device)
    }
  }

  return { unpaired, paired }
}

export async function pairHomeKitDevice(
  id: string,
  src: string,
  pin: string,
): Promise<void> {
  const params = new URLSearchParams()
  params.set("id", id)
  params.set("src", src)
  params.set("pin", pin)

  const response = await fetch(`${API_BASE}/homekit`, {
    method: "POST",
    body: params,
  })

  if (!response.ok) {
    throw new Error(await response.text())
  }
}

export async function unpairHomeKitDevice(id: string): Promise<void> {
  const params = new URLSearchParams()
  params.set("id", id)

  const response = await fetch(`${API_BASE}/homekit?${params.toString()}`, {
    method: "DELETE",
  })

  if (!response.ok) {
    throw new Error(await response.text())
  }
}

function parseInfoString(info: string): Record<string, string> {
  const result: Record<string, string> = {}
  const parts = info.split(" ")

  for (const part of parts) {
    const [key, value] = part.split("=")
    if (key && value) {
      result[key] = value
    }
  }

  return result
}

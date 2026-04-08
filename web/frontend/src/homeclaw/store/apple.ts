import { atom } from "jotai"

export interface HomeKitDevice {
  id: string
  name: string
  ip: string
  port: number
  info: Record<string, string>
  paired: boolean
  pairedId?: string
}

export interface AppleState {
  isLoading: boolean
  error: string | null
  unpairedDevices: HomeKitDevice[]
  pairedDevices: HomeKitDevice[]
}

export const appleAtom = atom<AppleState>({
  isLoading: false,
  error: null,
  unpairedDevices: [],
  pairedDevices: [],
})

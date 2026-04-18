import { atom } from "jotai"

import {
  type DeviceOpsByRoom,
  getAllDevicesWithOps,
} from "@/homeclaw/api/device-ops"
import type { DeviceControlWSStatus } from "@/homeclaw/api/device-control-websocket"

export type OperationLogStatus = "pending" | "success" | "failed"

export interface OperationLog {
  id: string
  timestamp: number
  deviceName: string
  opsName: string
  status: OperationLogStatus
  message: string
}

export interface DeviceOpsStoreState {
  rooms: DeviceOpsByRoom[]
  isLoading: boolean
  error: string | null
  executingOp: boolean
  executeError: string | null
  logs: OperationLog[]
  wsStatus: DeviceControlWSStatus
}

const DEFAULT_DEVICE_OPS_STATE: DeviceOpsStoreState = {
  rooms: [],
  isLoading: true,
  error: null,
  executingOp: false,
  executeError: null,
  logs: [],
  wsStatus: "disconnected",
}

export const deviceOpsAtom = atom<DeviceOpsStoreState>(DEFAULT_DEVICE_OPS_STATE)

export async function fetchDevicesWithOps(): Promise<Partial<DeviceOpsStoreState>> {
  try {
    const response = await getAllDevicesWithOps()
    return {
      rooms: response.rooms,
      isLoading: false,
      error: null,
    }
  } catch (error) {
    return {
      rooms: [],
      error: error instanceof Error ? error.message : "Unknown error",
      isLoading: false,
    }
  }
}

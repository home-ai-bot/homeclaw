import { atom } from "jotai"

import {
  type Device,
  type DeviceOpsByRoom,
  listDevices,
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

/**
 * Fetch devices via WebSocket, group by room, and sort by room name and device name.
 */
export async function fetchDevicesWithOps(): Promise<Partial<DeviceOpsStoreState>> {
  try {
    // Fetch flat list of devices
    const devices = await listDevices()

    // Group devices by room
    const roomMap = new Map<string, Device[]>()
    for (const device of devices) {
      const roomName = device.space_name || "Unassigned"
      if (!roomMap.has(roomName)) {
        roomMap.set(roomName, [])
      }
      roomMap.get(roomName)!.push(device)
    }

    // Sort devices within each room by device name
    for (const devices of roomMap.values()) {
      devices.sort((a, b) => a.name.localeCompare(b.name))
    }

    // Convert map to sorted array of rooms
    const rooms: DeviceOpsByRoom[] = Array.from(roomMap.entries())
      .map(([room_name, devices]) => ({ room_name, devices }))
      .sort((a, b) => a.room_name.localeCompare(b.room_name))

    return {
      rooms,
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

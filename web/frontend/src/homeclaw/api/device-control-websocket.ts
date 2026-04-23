// Dedicated WebSocket manager for the device control page.
// This is intentionally separate from the shared picoWS (chat) to avoid
// session conflicts: the chat controller owns /pico/ws?session_id={chatId},
// while device control uses its own connection without session_id parameter.

import { getPicoToken } from "@/api/pico"
import { getDefaultStore } from "jotai"
import { gatewayAtom } from "@/store/gateway"

export type DeviceControlWSStatus =
  | "disconnected"
  | "connecting"
  | "connected"
  | "error"

export type MessageHandler = (data: unknown) => void
export type StatusHandler = (status: DeviceControlWSStatus) => void

const MAX_RECONNECT_ATTEMPTS = 10
const BASE_RECONNECT_DELAY_MS = 1000
const MAX_RECONNECT_DELAY_MS = 15000

export class DeviceControlWebSocket {
  private ws: WebSocket | null = null
  private status: DeviceControlWSStatus = "disconnected"
  private messageHandlers: Set<MessageHandler> = new Set()
  private statusHandlers: Set<StatusHandler> = new Set()
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null
  private reconnectAttempts = 0
  private shouldReconnect = false
  private gatewayUnsubscribe: (() => void) | null = null
  private lastGatewayStatus: string | null = null

  // ── Connection lifecycle ────────────────────────────────────────────────

  async connect(): Promise<void> {
    if (
      this.ws &&
      (this.ws.readyState === WebSocket.OPEN ||
        this.ws.readyState === WebSocket.CONNECTING)
    ) {
      return
    }

    // Setup gateway status subscription
    this.setupGatewaySubscription()

    // Check if gateway is running before attempting connection
    const store = getDefaultStore()
    if (store.get(gatewayAtom).status !== "running") {
      console.log("[DeviceControlWS] Gateway not running, skipping connection")
      return
    }

    this.shouldReconnect = true
    this.reconnectAttempts = 0
    await this.openConnection()
  }

  disconnect(): void {
    this.shouldReconnect = false
    this.clearReconnectTimer()
    this.cleanupGatewaySubscription()
    if (this.ws) {
      this.ws.onclose = null // prevent auto-reconnect
      this.ws.close()
      this.ws = null
    }
    this.setStatus("disconnected")
  }

  // ── Send ────────────────────────────────────────────────────────────────

  /**
   * Wait until the WebSocket connection is open.
   * Resolves immediately if already connected.
   * Rejects after `timeout` ms.
   */
  waitForConnection(timeout = 15000): Promise<void> {
    if (this.ws && this.ws.readyState === WebSocket.OPEN) {
      return Promise.resolve()
    }
    return new Promise((resolve, reject) => {
      const tid = setTimeout(() => {
        this.offStatus(handler)
        reject(new Error("DeviceControlWS: connection timed out"))
      }, timeout)

      const handler: StatusHandler = (status) => {
        if (status === "connected") {
          clearTimeout(tid)
          this.offStatus(handler)
          resolve()
        }
      }
      this.onStatus(handler)
    })
  }

  sendMessage(message: unknown): void {
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
      throw new Error(
        `DeviceControlWS: not connected (status=${this.status})`,
      )
    }
    this.ws.send(JSON.stringify(message))
  }

  /**
   * Send a message and wait for the first response that satisfies matchFn.
   * Automatically waits for connection if not yet connected.
   * Rejects after `timeout` ms.
   */
  async sendAndWait(
    message: unknown,
    matchFn: (data: unknown) => boolean,
    timeout = 60000,
  ): Promise<unknown> {
    // Wait for connection first (up to 15s)
    await this.waitForConnection(Math.min(timeout, 15000))

    return new Promise((resolve, reject) => {
      const tid = setTimeout(() => {
        this.offMessage(handler)
        reject(new Error("DeviceControlWS: request timed out"))
      }, timeout)

      const handler: MessageHandler = (data) => {
        if (!matchFn(data)) return
        clearTimeout(tid)
        this.offMessage(handler)
        resolve(data)
      }

      this.onMessage(handler)

      try {
        this.sendMessage(message)
      } catch (err) {
        clearTimeout(tid)
        this.offMessage(handler)
        reject(err)
      }
    })
  }

  // ── Event subscriptions ─────────────────────────────────────────────────

  onMessage(handler: MessageHandler): void {
    this.messageHandlers.add(handler)
  }

  offMessage(handler: MessageHandler): void {
    this.messageHandlers.delete(handler)
  }

  onStatus(handler: StatusHandler): void {
    this.statusHandlers.add(handler)
    // Immediately notify of current status
    handler(this.status)
  }

  offStatus(handler: StatusHandler): void {
    this.statusHandlers.delete(handler)
  }

  getStatus(): DeviceControlWSStatus {
    return this.status
  }

  // ── Private helpers ─────────────────────────────────────────────────────

  private async openConnection(): Promise<void> {
    // Check if gateway is still running before opening connection
    const store = getDefaultStore()
    if (store.get(gatewayAtom).status !== "running") {
      console.log("[DeviceControlWS] Gateway not running, aborting connection")
      this.setStatus("disconnected")
      return
    }

    this.setStatus("connecting")

    let token: string
    try {
      const resp = await getPicoToken()
      token = resp.token
      if (!token) throw new Error("empty token")
    } catch (err) {
      console.error("[DeviceControlWS] Failed to get token:", err)
      this.setStatus("error")
      this.scheduleReconnect()
      return
    }

    const scheme = window.location.protocol === "https:" ? "wss:" : "ws:"
    const url = `${scheme}//${window.location.host}/pico/ws-tool`
    const socket = new WebSocket(url, [`token.${token}`])

    socket.onopen = () => {
      console.log("[DeviceControlWS] Connected to tool WebSocket")
      this.ws = socket
      this.reconnectAttempts = 0
      this.setStatus("connected")
    }

    socket.onmessage = (event) => {
      try {
        const data: unknown = JSON.parse(event.data as string)
        this.messageHandlers.forEach((h) => h(data))
      } catch {
        console.warn("[DeviceControlWS] Failed to parse message:", event.data)
      }
    }

    socket.onerror = () => {
      console.error("[DeviceControlWS] Connection error")
      this.setStatus("error")
    }

    socket.onclose = () => {
      console.log("[DeviceControlWS] Connection closed")
      if (this.ws === socket) {
        this.ws = null
      }
      if (this.status !== "disconnected") {
        this.setStatus("disconnected")
      }
      if (this.shouldReconnect) {
        this.scheduleReconnect()
      }
    }

    this.ws = socket
  }

  private scheduleReconnect(): void {
    if (!this.shouldReconnect) return
    
    // Check if gateway is still running before scheduling reconnect
    const store = getDefaultStore()
    if (store.get(gatewayAtom).status !== "running") {
      console.log("[DeviceControlWS] Gateway not running, skipping reconnect")
      this.setStatus("disconnected")
      return
    }
    
    if (this.reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      console.warn("[DeviceControlWS] Max reconnect attempts reached")
      this.setStatus("error")
      return
    }

    this.clearReconnectTimer()
    const delay = Math.min(
      BASE_RECONNECT_DELAY_MS * 2 ** this.reconnectAttempts,
      MAX_RECONNECT_DELAY_MS,
    )
    this.reconnectAttempts++
    console.log(
      `[DeviceControlWS] Reconnecting in ${delay}ms (attempt ${this.reconnectAttempts})`,
    )
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null
      void this.openConnection()
    }, delay)
  }

  private clearReconnectTimer(): void {
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer)
      this.reconnectTimer = null
    }
  }

  private setStatus(status: DeviceControlWSStatus): void {
    if (this.status === status) return
    this.status = status
    this.statusHandlers.forEach((h) => h(status))
  }

  // ── Gateway status subscription ─────────────────────────────────────────

  private setupGatewaySubscription(): void {
    this.cleanupGatewaySubscription()
    
    const store = getDefaultStore()
    const syncConnectionWithGateway = () => {
      const gatewayStatus = store.get(gatewayAtom).status
      
      // Skip if status hasn't changed
      if (gatewayStatus === this.lastGatewayStatus) {
        return
      }
      this.lastGatewayStatus = gatewayStatus

      // Gateway just started - try to connect
      if (gatewayStatus === "running") {
        console.log("[DeviceControlWS] Gateway started, attempting connection")
        if (!this.ws || this.ws.readyState !== WebSocket.OPEN) {
          this.shouldReconnect = true
          this.reconnectAttempts = 0
          void this.openConnection()
        }
        return
      }

      // Gateway stopped or errored - disconnect
      if (gatewayStatus === "stopped" || gatewayStatus === "error") {
        console.log(`[DeviceControlWS] Gateway ${gatewayStatus}, disconnecting`)
        this.shouldReconnect = false
        this.clearReconnectTimer()
        if (this.ws) {
          this.ws.onclose = null
          this.ws.close()
          this.ws = null
        }
        this.setStatus("disconnected")
      }
    }

    // Subscribe to gateway status changes
    this.gatewayUnsubscribe = store.sub(gatewayAtom, syncConnectionWithGateway)
    
    // Run immediately to sync with current status
    syncConnectionWithGateway()
  }

  private cleanupGatewaySubscription(): void {
    if (this.gatewayUnsubscribe) {
      this.gatewayUnsubscribe()
      this.gatewayUnsubscribe = null
    }
    this.lastGatewayStatus = null
  }
}

// Module-level singleton dedicated to device control.
// Do NOT import this from the chat controller or shared picoWS code.
export const deviceControlWS = new DeviceControlWebSocket()

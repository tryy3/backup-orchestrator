import { ref, readonly } from 'vue'
import type { WebSocketEventMap } from '../types/api'

export type ConnectionStatus = 'connecting' | 'connected' | 'disconnected'

export interface WebSocketEvent {
  type: string
  payload: unknown
}

type EventCallback = (payload: unknown) => void

const connectionStatus = ref<ConnectionStatus>('disconnected')

let socket: WebSocket | null = null
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let reconnectAttempts = 0
const MAX_RECONNECT_DELAY = 30_000

const listeners = new Map<string, Set<EventCallback>>()

function getWebSocketURL(): string {
  const base = import.meta.env.VITE_API_URL ?? '/api'

  // If absolute URL, convert http(s) to ws(s)
  if (base.startsWith('http')) {
    return base.replace(/^http/, 'ws').replace(/\/api\/?$/, '') + '/api/ws'
  }

  // Relative URL — derive from current page location
  const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${proto}//${window.location.host}${base}/ws`
}

function handleMessage(event: MessageEvent) {
  try {
    const data = JSON.parse(event.data) as WebSocketEvent
    const callbacks = listeners.get(data.type)
    if (callbacks) {
      for (const cb of callbacks) {
        cb(data.payload)
      }
    }
  } catch {
    // Ignore malformed messages
  }
}

function scheduleReconnect() {
  if (reconnectTimer) return
  const delay = Math.min(1000 * 2 ** reconnectAttempts, MAX_RECONNECT_DELAY)
  reconnectAttempts++
  connectionStatus.value = 'disconnected'
  reconnectTimer = setTimeout(() => {
    reconnectTimer = null
    connect()
  }, delay)
}

function connect() {
  if (socket?.readyState === WebSocket.OPEN || socket?.readyState === WebSocket.CONNECTING) {
    return
  }

  connectionStatus.value = 'connecting'

  const url = getWebSocketURL()
  socket = new WebSocket(url)

  socket.onopen = () => {
    connectionStatus.value = 'connected'
    reconnectAttempts = 0
  }

  socket.onmessage = handleMessage

  socket.onclose = () => {
    socket = null
    scheduleReconnect()
  }

  socket.onerror = () => {
    // onclose will fire after onerror, which handles reconnect
    socket?.close()
  }
}

function disconnect() {
  if (reconnectTimer) {
    clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
  reconnectAttempts = 0
  if (socket) {
    socket.onclose = null // prevent reconnect
    socket.close()
    socket = null
  }
  connectionStatus.value = 'disconnected'
}

/**
 * Subscribe to a specific event type pushed from the server.
 * Returns an unsubscribe function.
 */
export function subscribe<K extends keyof WebSocketEventMap>(
  eventType: K,
  callback: (payload: WebSocketEventMap[K]) => void,
): () => void {
  if (!listeners.has(eventType)) {
    listeners.set(eventType, new Set())
  }
  listeners.get(eventType)!.add(callback as EventCallback)
  return () => {
    listeners.get(eventType)?.delete(callback as EventCallback)
  }
}

/**
 * Connect the WebSocket. Should be called once at app startup.
 */
export function connectWebSocket() {
  connect()
}

/**
 * Disconnect the WebSocket. Called on app teardown if needed.
 */
export function disconnectWebSocket() {
  disconnect()
}

/**
 * Reactive connection status for UI indicators.
 */
export const wsConnectionStatus = readonly(connectionStatus)

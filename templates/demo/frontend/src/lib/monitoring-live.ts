export type MonitorStatusEvent = {
  type: string
  monitor_id: string
  monitor_name?: string
  monitor_type?: string
  target?: string
  phase?: string
  status?: string
  incident_id?: string
  summary?: string
  error_class?: string
  error_message?: string
  duration_ms?: number
  checked_at?: string
}

import { emitMonitoringSettingsUpdated } from '@/lib/monitoring-settings-events'

export type MonitorStatusSnapshot = {
  id?: string
  name?: string
  enabled?: boolean
  last_status?: string
}

type MonitorStatusListener = (event: MonitorStatusEvent) => void

const listeners = new Set<MonitorStatusListener>()
const transitionListeners = new Set<MonitorStatusListener>()
const latestStatusByMonitor = new Map<string, { status: string; name?: string }>()
let socket: WebSocket | null = null
let reconnectTimer: number | null = null
let intentionallyClosed = false

function monitoringStreamURL(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}/api/v1/monitoring/stream`
}

function notify(event: MonitorStatusEvent) {
  listeners.forEach((listener) => listener(event))
}

function notifyTransition(event: MonitorStatusEvent) {
  transitionListeners.forEach((listener) => listener(event))
}

function normalizeStatus(status: string | undefined, enabled: boolean | undefined): string {
  if (enabled === false) return 'paused'
  const normalized = (status || '').trim().toLowerCase()
  if (!normalized) return 'unknown'
  return normalized
}

function maybeEmitTransition(monitorID: string, nextStatus: string, monitorName?: string) {
  const previous = latestStatusByMonitor.get(monitorID)
  latestStatusByMonitor.set(monitorID, { status: nextStatus, name: monitorName || previous?.name })
  if (!previous) return
  if (previous.status === nextStatus) return
  const isUpDownTransition =
    (previous.status === 'up' && nextStatus === 'down') ||
    (previous.status === 'down' && nextStatus === 'up')
  if (!isUpDownTransition) return
  notifyTransition({
    type: nextStatus === 'up' ? 'monitor.recovered' : 'monitor.down',
    monitor_id: monitorID,
    monitor_name: monitorName || previous.name,
    status: nextStatus,
  })
}

function clearReconnectTimer() {
  if (reconnectTimer !== null) {
    window.clearTimeout(reconnectTimer)
    reconnectTimer = null
  }
}

function scheduleReconnect() {
  if (reconnectTimer !== null || intentionallyClosed || listeners.size === 0) return
  reconnectTimer = window.setTimeout(() => {
    reconnectTimer = null
    connect()
  }, 1000)
}

function connect() {
  if (socket || listeners.size === 0) return
  intentionallyClosed = false
  socket = new WebSocket(monitoringStreamURL())
  socket.onmessage = (message) => {
    try {
      const payload = JSON.parse(message.data) as MonitorStatusEvent
      if (!payload || typeof payload.type !== 'string') return
      if (
        payload.type !== 'monitor.down' &&
        payload.type !== 'monitor.recovered' &&
        payload.type !== 'monitor.polling' &&
        payload.type !== 'monitor.maintenance'
      ) return
      if (payload.type === 'monitor.maintenance') {
        emitMonitoringSettingsUpdated({
          active: (payload.status || '').toLowerCase() === 'active',
          endsAt: payload.checked_at,
        })
        notify(payload)
        return
      }
      if (payload.monitor_id) {
        if (payload.type === 'monitor.down' || payload.type === 'monitor.recovered') {
          maybeEmitTransition(payload.monitor_id, normalizeStatus(payload.status, true), payload.monitor_name)
        }
      }
      notify(payload)
    } catch {
      // Ignore malformed payloads in this best-effort stream.
    }
  }
  socket.onclose = () => {
    socket = null
    scheduleReconnect()
  }
  socket.onerror = () => {
    socket?.close()
  }
}

function closeIfIdle() {
  if (listeners.size > 0) return
  intentionallyClosed = true
  clearReconnectTimer()
  socket?.close()
  socket = null
}

export function subscribeMonitoringStatusEvents(listener: MonitorStatusListener): () => void {
  listeners.add(listener)
  connect()
  return () => {
    listeners.delete(listener)
    closeIfIdle()
  }
}

export function subscribeMonitoringTransitionEvents(listener: MonitorStatusListener): () => void {
  transitionListeners.add(listener)
  connect()
  return () => {
    transitionListeners.delete(listener)
    closeIfIdle()
  }
}

export function applyMonitorStatusSnapshot(monitors: MonitorStatusSnapshot[]) {
  monitors.forEach((monitor) => {
    const monitorID = String(monitor.id || '').trim()
    if (!monitorID) return
    maybeEmitTransition(
      monitorID,
      normalizeStatus(monitor.last_status, monitor.enabled),
      monitor.name,
    )
  })
}

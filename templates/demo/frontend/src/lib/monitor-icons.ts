import type { Component } from 'vue'
import {
  Activity,
  Cable,
  Container,
  Gamepad2,
  Globe,
  HelpCircle,
  Network,
  Radio,
  Search,
  SendHorizontal,
  ShieldCheck,
  Waypoints,
} from 'lucide-vue-next'

export type MonitorTypeOption = {
  value: string
  label: string
  icon: Component
}

const monitorTypeOptions: MonitorTypeOption[] = [
  { value: 'http', label: 'HTTP', icon: Globe },
  { value: 'http_keyword', label: 'HTTP Keyword', icon: Search },
  { value: 'http_json_query', label: 'HTTP JSON Query', icon: Waypoints },
  { value: 'websocket', label: 'WebSocket', icon: Radio },
  { value: 'tcp', label: 'TCP', icon: Cable },
  { value: 'ping', label: 'Ping', icon: Activity },
  { value: 'dns', label: 'DNS', icon: Network },
  { value: 'tls', label: 'TLS', icon: ShieldCheck },
  { value: 'steam', label: 'Steam Game Server', icon: Gamepad2 },
  { value: 'docker', label: 'Docker Container', icon: Container },
  { value: 'push', label: 'Push', icon: SendHorizontal },
]

const monitorTypeMap: Record<string, MonitorTypeOption> = monitorTypeOptions.reduce(
  (acc, option) => {
    acc[option.value] = option
    return acc
  },
  {} as Record<string, MonitorTypeOption>,
)

export const MONITOR_TYPE_OPTIONS = monitorTypeOptions

export function normalizeMonitorType(value?: string): string {
  return String(value || '').trim().toLowerCase()
}

export function monitorTypeOption(value?: string): MonitorTypeOption {
  const normalized = normalizeMonitorType(value)
  return monitorTypeMap[normalized] || { value: normalized || 'unknown', label: 'Unknown', icon: HelpCircle }
}

export function monitorTypeIcon(value?: string): Component {
  return monitorTypeOption(value).icon
}

export function monitorTypeLabel(value?: string): string {
  return monitorTypeOption(value).label
}

export function monitorSupportsFavicon(value?: string): boolean {
  const normalized = normalizeMonitorType(value)
  return (
    normalized === 'http' ||
    normalized === 'http_keyword' ||
    normalized === 'http_json_query' ||
    normalized === 'websocket'
  )
}

import type { Component } from 'vue'
import { i18n } from '@/i18n'
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
  labelKey: string
  icon: Component
}

const monitorTypeOptions: MonitorTypeOption[] = [
  { value: 'http', labelKey: 'monitorTypes.http', icon: Globe },
  { value: 'http_keyword', labelKey: 'monitorTypes.http_keyword', icon: Search },
  { value: 'http_json_query', labelKey: 'monitorTypes.http_json_query', icon: Waypoints },
  { value: 'websocket', labelKey: 'monitorTypes.websocket', icon: Radio },
  { value: 'tcp', labelKey: 'monitorTypes.tcp', icon: Cable },
  { value: 'ping', labelKey: 'monitorTypes.ping', icon: Activity },
  { value: 'dns', labelKey: 'monitorTypes.dns', icon: Network },
  { value: 'tls', labelKey: 'monitorTypes.tls', icon: ShieldCheck },
  { value: 'steam', labelKey: 'monitorTypes.steam', icon: Gamepad2 },
  { value: 'docker', labelKey: 'monitorTypes.docker', icon: Container },
  { value: 'push', labelKey: 'monitorTypes.push', icon: SendHorizontal },
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
  return monitorTypeMap[normalized] || { value: normalized || 'unknown', labelKey: 'monitorTypes.unknown', icon: HelpCircle }
}

export function monitorTypeIcon(value?: string): Component {
  return monitorTypeOption(value).icon
}

export function monitorTypeLabel(value?: string): string {
  return i18n.global.t(monitorTypeOption(value).labelKey)
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

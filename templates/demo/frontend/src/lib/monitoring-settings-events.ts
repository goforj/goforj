export const MONITORING_SETTINGS_UPDATED_EVENT = 'monitoring:settings-updated'

export type MonitoringMaintenanceSnapshot = {
  active: boolean
  startsAt?: string
  endsAt?: string
}

let maintenanceExpiryTimer: number | null = null

function clearMaintenanceExpiryTimer() {
  if (maintenanceExpiryTimer !== null) {
    window.clearTimeout(maintenanceExpiryTimer)
    maintenanceExpiryTimer = null
  }
}

function scheduleMaintenanceExpiry(maintenance?: MonitoringMaintenanceSnapshot) {
  clearMaintenanceExpiryTimer()
  if (!maintenance?.active || !maintenance.endsAt) return
  const endsAtMs = Date.parse(maintenance.endsAt)
  if (!Number.isFinite(endsAtMs)) return
  const delayMs = endsAtMs - Date.now()
  if (delayMs <= 0) {
    window.dispatchEvent(new CustomEvent<MonitoringMaintenanceSnapshot | undefined>(MONITORING_SETTINGS_UPDATED_EVENT, {
      detail: { active: false, startsAt: maintenance.startsAt, endsAt: maintenance.endsAt },
    }))
    return
  }
  maintenanceExpiryTimer = window.setTimeout(() => {
    maintenanceExpiryTimer = null
    window.dispatchEvent(new CustomEvent<MonitoringMaintenanceSnapshot | undefined>(MONITORING_SETTINGS_UPDATED_EVENT, {
      detail: { active: false, startsAt: maintenance.startsAt, endsAt: maintenance.endsAt },
    }))
  }, delayMs)
}

export function syncMonitoringMaintenanceSnapshot(maintenance?: MonitoringMaintenanceSnapshot) {
  scheduleMaintenanceExpiry(maintenance)
}

export function emitMonitoringSettingsUpdated(maintenance?: MonitoringMaintenanceSnapshot) {
  scheduleMaintenanceExpiry(maintenance)
  window.dispatchEvent(new CustomEvent<MonitoringMaintenanceSnapshot | undefined>(MONITORING_SETTINGS_UPDATED_EVENT, {
    detail: maintenance,
  }))
}

export function subscribeMonitoringSettingsUpdated(
  listener: (maintenance?: MonitoringMaintenanceSnapshot) => void,
) {
  const handler = (event: Event) => {
    const customEvent = event as CustomEvent<MonitoringMaintenanceSnapshot | undefined>
    listener(customEvent.detail)
  }
  window.addEventListener(MONITORING_SETTINGS_UPDATED_EVENT, handler)
  return () => window.removeEventListener(MONITORING_SETTINGS_UPDATED_EVENT, handler)
}

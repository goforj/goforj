<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppSidebar from '@/components/AppSidebar.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { Toaster, toast } from 'vue-sonner'
import { subscribeMonitoringTransitionEvents } from '@/lib/monitoring-live'
import { subscribeMonitoringSettingsUpdated, type MonitoringMaintenanceSnapshot } from '@/lib/monitoring-settings-events'

const route = useRoute()
const { t } = useI18n()
const isPublicShell = computed(() => route.meta?.publicShell === true)
let unsubscribeMonitoringToasts: (() => void) | null = null
let unsubscribeMaintenanceToasts: (() => void) | null = null
let previousMaintenanceSnapshot: MonitoringMaintenanceSnapshot | null = null

onMounted(() => {
  if (!unsubscribeMonitoringToasts) {
    unsubscribeMonitoringToasts = subscribeMonitoringTransitionEvents((event) => {
      if (route.meta?.publicShell === true) return
      const name = event.monitor_name || event.monitor_id || 'Monitor'
      const toastID = `monitor-transition:${event.monitor_id}:${event.type}`
      if (event.type === 'monitor.recovered') {
        toast.success(`${name} recovered`, {
          id: toastID,
          description: event.summary || 'Responding again.',
        })
        return
      }
      if (event.type === 'monitor.down') {
        toast.error(`${name} is down`, {
          id: toastID,
          description: event.summary || event.error_message || undefined,
        })
      }
    })
  }
  if (!unsubscribeMaintenanceToasts) {
    unsubscribeMaintenanceToasts = subscribeMonitoringSettingsUpdated((maintenance) => {
      if (route.meta?.publicShell === true) return
      const previous = previousMaintenanceSnapshot
      previousMaintenanceSnapshot = maintenance || null
      const endedNaturally =
        Boolean(previous?.active) &&
        !Boolean(maintenance?.active) &&
        typeof maintenance?.endsAt === 'string' &&
        maintenance.endsAt.length > 0
      if (!endedNaturally) return
      toast.success(t('settings.globalMaintenanceFinishedTitle'), {
        id: 'maintenance-finished',
        description: t('settings.globalMaintenanceFinishedDescription'),
      })
    })
  }
})

onUnmounted(() => {
  if (unsubscribeMonitoringToasts) {
    unsubscribeMonitoringToasts()
    unsubscribeMonitoringToasts = null
  }
  if (unsubscribeMaintenanceToasts) {
    unsubscribeMaintenanceToasts()
    unsubscribeMaintenanceToasts = null
  }
})
</script>

<template>
  <RouterView v-if="isPublicShell" />
  <SidebarProvider
    v-else
    :style="{
      '--sidebar-width': 'calc(var(--spacing) * 72)',
      '--header-height': 'calc(var(--spacing) * 12)',
    }"
  >
    <AppSidebar />
    <SidebarInset>
      <SiteHeader />
      <div class="flex flex-1 flex-col">
        <div class="@container/main flex flex-1 flex-col gap-2">
          <RouterView />
        </div>
      </div>
    </SidebarInset>
  </SidebarProvider>
  <Toaster />
</template>

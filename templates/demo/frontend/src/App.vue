<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'
import AppSidebar from '@/components/AppSidebar.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { Toaster, toast } from 'vue-sonner'
import { subscribeMonitoringTransitionEvents } from '@/lib/monitoring-live'
import { subscribeMonitoringSettingsUpdated, type MonitoringMaintenanceSnapshot } from '@/lib/monitoring-settings-events'
import logoMark from '@/assets/favicons/favicon-96x96.png'

const route = useRoute()
const { t } = useI18n()
const isPublicShell = computed(() => route.meta?.publicShell === true)
const showShellSplash = ref(true)
let shellSplashTimer: number | null = null
let unsubscribeMonitoringToasts: (() => void) | null = null
let unsubscribeMaintenanceToasts: (() => void) | null = null
let previousMaintenanceSnapshot: MonitoringMaintenanceSnapshot | null = null

onMounted(() => {
  shellSplashTimer = window.setTimeout(() => {
    showShellSplash.value = false
    shellSplashTimer = null
  }, 320)
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
  if (shellSplashTimer !== null) {
    window.clearTimeout(shellSplashTimer)
    shellSplashTimer = null
  }
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
    class="app-shell-provider"
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
    <Transition name="shell-splash-fade">
      <div v-if="showShellSplash" class="app-shell-splash">
        <div class="app-shell-splash__backdrop" />
        <div class="app-shell-splash__veil" />
        <div class="app-shell-splash__content">
          <div class="app-shell-splash__brand">
            <img :src="logoMark" alt="Uptime Gopher" class="app-shell-splash__mark" />
            <div class="app-shell-splash__copy">
              <p class="app-shell-splash__kicker">Uptime Gopher</p>
              <p class="app-shell-splash__title">{{ t('routes.monitoring') }}</p>
            </div>
          </div>
        </div>
      </div>
    </Transition>
  </SidebarProvider>
  <Toaster />
</template>

<style scoped>
.app-shell-provider {
  position: relative;
}

.app-shell-splash {
  position: absolute;
  inset: 0;
  z-index: 60;
  overflow: hidden;
  pointer-events: none;
}

.app-shell-splash__backdrop {
  position: absolute;
  inset: 0;
  background:
    radial-gradient(circle at 16% 18%, rgba(18, 151, 182, 0.18), transparent 20%),
    radial-gradient(circle at 84% 22%, rgba(0, 214, 143, 0.12), transparent 18%),
    radial-gradient(circle at 72% 78%, rgba(27, 78, 230, 0.18), transparent 22%),
    linear-gradient(135deg, #05070c 0%, #090b12 42%, #0a0d16 100%);
}

.app-shell-splash__veil {
  position: absolute;
  inset: 0;
  background:
    linear-gradient(180deg, rgba(8, 10, 16, 0.28), rgba(8, 10, 16, 0.46)),
    radial-gradient(circle at top left, rgba(255, 255, 255, 0.03), transparent 30%);
}

.app-shell-splash__content {
  position: absolute;
  inset: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 2rem;
}

.app-shell-splash__brand {
  display: flex;
  align-items: center;
  gap: 1rem;
  padding: 1.15rem 1.35rem;
  border-radius: 1.25rem;
  border: 1px solid rgba(255, 255, 255, 0.06);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.04), rgba(255, 255, 255, 0.015)),
    rgba(10, 12, 18, 0.52);
  backdrop-filter: blur(10px);
}

.app-shell-splash__mark {
  width: 3.5rem;
  height: 3.5rem;
  object-fit: contain;
  filter: drop-shadow(0 12px 28px rgba(0, 0, 0, 0.28));
}

.app-shell-splash__copy {
  display: grid;
  gap: 0.15rem;
}

.app-shell-splash__kicker {
  font-size: 0.72rem;
  letter-spacing: 0.28em;
  text-transform: uppercase;
  color: color-mix(in oklab, var(--foreground) 66%, transparent);
}

.app-shell-splash__title {
  font-size: clamp(1.75rem, 2.4vw, 2.1rem);
  line-height: 0.98;
  font-weight: 700;
  letter-spacing: -0.04em;
  color: var(--foreground);
}

.shell-splash-fade-enter-active,
.shell-splash-fade-leave-active {
  transition: opacity 280ms ease;
}

.shell-splash-fade-enter-from,
.shell-splash-fade-leave-to {
  opacity: 0;
}
</style>

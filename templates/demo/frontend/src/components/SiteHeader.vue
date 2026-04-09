<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Activity, Check, CirclePause, Clock3, Globe, HeartPulse, Server, ShieldAlert, Wrench } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import { Button } from '@/components/ui/button'
import { subscribeMonitoringSettingsUpdated, syncMonitoringMaintenanceSnapshot } from '@/lib/monitoring-settings-events'
import { subscribeMonitoringStatusEvents } from '@/lib/monitoring-live'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Separator } from '@/components/ui/separator'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { LOCALE_OPTIONS, setLocale, type AppLocale } from '@/i18n'

const route = useRoute()
const { locale, t } = useI18n()
let unsubscribeMonitoringLive: (() => void) | null = null
let unsubscribeMonitoringSettings: (() => void) | null = null

const title = computed(() => {
  switch (route.path) {
    case '/incidents':
      return t('routes.incidents')
    case '/status-pages':
      return t('routes.statusPages')
    case '/diagnostics':
      return t('routes.diagnostics')
    case '/settings':
      return t('routes.settings')
    default:
      return t('routes.monitoring')
  }
})

const summary = ref<any>(null)
const isMonitoringArea = computed(() => {
  return (
    route.path.startsWith('/monitors') ||
    route.path === '/incidents' ||
    route.path === '/status-pages' ||
    route.path === '/diagnostics' ||
    route.path === '/settings'
  )
})

async function loadSummary() {
  if (!isMonitoringArea.value) return
  const res = await fetch('/api/v1/monitoring/summary')
  if (!res.ok) return
  summary.value = await res.json()
  const maintenance = summary.value?.maintenance ?? {}
  syncMonitoringMaintenanceSnapshot({
    active: Boolean(maintenance?.active),
    startsAt: typeof maintenance?.starts_at === 'string' ? maintenance.starts_at : '',
    endsAt: typeof maintenance?.ends_at === 'string' ? maintenance.ends_at : '',
  })
}

const metricPills = computed(() => {
  const stats = summary.value?.stats || {}
  return [
    { label: t('nav.monitors'), value: stats.monitors_total ?? 0, tone: 'default', icon: Server },
    { label: t('status.up'), value: stats.monitors_up ?? 0, tone: 'success', icon: HeartPulse },
    { label: t('monitoring.paused'), value: stats.monitors_paused ?? 0, tone: 'warning', icon: CirclePause },
    { label: t('status.pending'), value: stats.monitors_pending ?? 0, tone: 'pending', icon: Clock3 },
    { label: t('status.down'), value: stats.monitors_down ?? 0, tone: 'danger', icon: ShieldAlert },
    { label: t('monitoring.checksOneHour'), value: stats.checks_last_hour ?? 0, tone: 'muted', icon: Activity },
  ]
})

const maintenanceBadge = computed(() => {
  const maintenance = summary.value?.maintenance ?? {}
  const active = Boolean(maintenance?.active)
  const startsAt = typeof maintenance?.starts_at === 'string' ? maintenance.starts_at : ''
  const endsAt = typeof maintenance?.ends_at === 'string' ? maintenance.ends_at : ''
  return { active, startsAt, endsAt }
})

function applyMaintenanceBadge(maintenance?: { active?: boolean; startsAt?: string; endsAt?: string }) {
  if (!summary.value || typeof summary.value !== 'object') {
    summary.value = { maintenance: { active: false, starts_at: '', ends_at: '' } }
  }
  const nextMaintenance = {
    ...(summary.value?.maintenance ?? {}),
    active: Boolean(maintenance?.active),
    starts_at: maintenance?.startsAt || '',
    ends_at: maintenance?.endsAt || '',
  }
  summary.value = {
    ...(summary.value ?? {}),
    maintenance: nextMaintenance,
  }
}

function onSwitchLocale(next: AppLocale) {
  if (locale.value === next) return
  setLocale(next)
}

onMounted(() => {
  void loadSummary()
  if (!unsubscribeMonitoringLive) {
    unsubscribeMonitoringLive = subscribeMonitoringStatusEvents(() => {
      void loadSummary()
    })
  }
  if (!unsubscribeMonitoringSettings) {
    unsubscribeMonitoringSettings = subscribeMonitoringSettingsUpdated((maintenance) => {
      applyMaintenanceBadge(maintenance)
      void loadSummary()
    })
  }
})

watch(
  () => route.path,
  () => {
    void loadSummary()
  },
)

let refreshTimer: number | null = null
onMounted(() => {
  refreshTimer = window.setInterval(() => {
    void loadSummary()
  }, 10000)
})
onUnmounted(() => {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
  if (unsubscribeMonitoringLive) {
    unsubscribeMonitoringLive()
    unsubscribeMonitoringLive = null
  }
  if (unsubscribeMonitoringSettings) {
    unsubscribeMonitoringSettings()
    unsubscribeMonitoringSettings = null
  }
})
</script>

<template>
  <header class="shrink-0 border-b transition-[width,height] ease-linear group-has-data-[collapsible=icon]/sidebar-wrapper:h-(--header-height)">
    <div class="flex h-(--header-height) w-full items-center gap-1 px-4 lg:gap-2 lg:px-6">
      <SidebarTrigger class="-ml-1" />
      <Separator
        orientation="vertical"
        class="mx-2 data-[orientation=vertical]:h-4"
      />
      <h1 class="text-base font-medium">
        {{ title }}
      </h1>
      <div class="ml-auto flex items-center gap-2 pr-2">
        <div v-if="isMonitoringArea" class="hidden items-center gap-2 overflow-x-auto md:flex">
          <div
            v-if="maintenanceBadge.active"
            class="flex min-w-max items-center gap-2 rounded-full border border-amber-500/40 bg-amber-500/10 px-2.5 py-1 text-xs text-amber-300"
            :title="t('settings.globalMaintenanceTooltip', { endsAt: maintenanceBadge.endsAt || t('settings.globalMaintenanceUntilUnknown') })"
          >
            <Wrench class="size-3.5" />
            <span class="font-semibold">{{ t('settings.globalMaintenanceActive') }}</span>
          </div>
          <div
            v-for="pill in metricPills"
            :key="pill.label"
            class="flex min-w-max items-center gap-2 rounded-full border border-border px-2.5 py-1 text-xs"
          >
            <component
              :is="pill.icon"
              class="size-3.5"
              :class="
                pill.tone === 'success'
                  ? 'text-emerald-400'
                  : pill.tone === 'warning'
                  ? 'text-amber-400'
                  : pill.tone === 'pending'
                  ? 'text-yellow-300'
                  : pill.tone === 'danger'
                  ? 'text-rose-400'
                  : 'text-muted-foreground'
              "
            />
            <span class="text-muted-foreground">{{ pill.label }}</span>
            <span
              class="font-semibold"
              :class="
                pill.tone === 'success'
                  ? 'text-emerald-400'
                  : pill.tone === 'warning'
                  ? 'text-amber-400'
                  : pill.tone === 'pending'
                  ? 'text-yellow-300'
                  : pill.tone === 'danger'
                  ? 'text-rose-400'
                  : 'text-foreground'
              "
            >
              {{ pill.value }}
            </span>
          </div>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger as-child>
            <Button
              variant="outline"
              size="sm"
              class="gap-2"
              :aria-label="t('language.label')"
            >
              <Globe class="size-4" />
              <span class="hidden sm:inline">{{ locale.toUpperCase() }}</span>
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuLabel>{{ t('language.label') }}</DropdownMenuLabel>
            <DropdownMenuSeparator />
            <DropdownMenuItem
              v-for="entry in LOCALE_OPTIONS"
              :key="entry.code"
              @click="onSwitchLocale(entry.code)"
            >
              <Check class="size-4" :class="locale === entry.code ? 'opacity-100' : 'opacity-0'" />
              {{ t(entry.labelKey) }}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
    <div
      v-if="isMonitoringArea"
      class="flex items-center gap-2 overflow-x-auto px-4 pb-2 md:hidden"
    >
      <div
        v-if="maintenanceBadge.active"
        class="flex min-w-max items-center gap-2 rounded-full border border-amber-500/40 bg-amber-500/10 px-2 py-1 text-[11px] text-amber-300"
        :title="t('settings.globalMaintenanceTooltip', { endsAt: maintenanceBadge.endsAt || t('settings.globalMaintenanceUntilUnknown') })"
      >
        <Wrench class="size-3" />
        <span class="font-semibold">{{ t('settings.globalMaintenanceActive') }}</span>
      </div>
      <div
        v-for="pill in metricPills"
        :key="`${pill.label}-mobile`"
        class="flex min-w-max items-center gap-2 rounded-full border border-border px-2 py-1 text-[11px]"
      >
        <component
          :is="pill.icon"
          class="size-3"
          :class="
            pill.tone === 'success'
              ? 'text-emerald-400'
              : pill.tone === 'warning'
              ? 'text-amber-400'
              : pill.tone === 'pending'
              ? 'text-yellow-300'
              : pill.tone === 'danger'
              ? 'text-rose-400'
              : 'text-muted-foreground'
          "
        />
        <span class="text-muted-foreground">{{ pill.label }}</span>
        <span
          class="font-semibold"
          :class="
            pill.tone === 'success'
              ? 'text-emerald-400'
              : pill.tone === 'warning'
              ? 'text-amber-400'
              : pill.tone === 'pending'
              ? 'text-yellow-300'
              : pill.tone === 'danger'
              ? 'text-rose-400'
              : 'text-foreground'
          "
        >
          {{ pill.value }}
        </span>
      </div>
    </div>
  </header>
</template>

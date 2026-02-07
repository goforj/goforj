<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from 'vue'
import { useRoute } from 'vue-router'
import { Activity, CirclePause, HeartPulse, Server, ShieldAlert } from 'lucide-vue-next'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { SidebarTrigger } from '@/components/ui/sidebar'
const route = useRoute()
const title = computed(() => {
  switch (route.path) {
    case '/incidents':
      return 'Incidents'
    case '/status-pages':
      return 'Status Pages'
    default:
      return 'Monitoring'
  }
})

const summary = ref<any>(null)
const isMonitoringArea = computed(() => {
  return (
    route.path.startsWith('/monitors') ||
    route.path === '/incidents' ||
    route.path === '/status-pages'
  )
})

async function loadSummary() {
  if (!isMonitoringArea.value) return
  const res = await fetch('/api/v1/monitoring/summary')
  if (!res.ok) return
  summary.value = await res.json()
}

const metricPills = computed(() => {
  const stats = summary.value?.stats || {}
  return [
    { label: 'Monitors', value: stats.monitors_total ?? 0, tone: 'default', icon: Server },
    { label: 'Up', value: stats.monitors_up ?? 0, tone: 'success', icon: HeartPulse },
    { label: 'Paused', value: stats.monitors_paused ?? 0, tone: 'warning', icon: CirclePause },
    { label: 'Down', value: stats.monitors_down ?? 0, tone: 'danger', icon: ShieldAlert },
    { label: 'Checks (1h)', value: stats.checks_last_hour ?? 0, tone: 'muted', icon: Activity },
  ]
})

onMounted(() => {
  void loadSummary()
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
      <div v-if="isMonitoringArea" class="ml-auto hidden items-center gap-2 overflow-x-auto pr-2 md:flex">
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
                : pill.tone === 'danger'
                ? 'text-rose-400'
                : 'text-foreground'
            "
          >
            {{ pill.value }}
          </span>
        </div>
      </div>
      <div class="ml-auto flex items-center gap-2 md:ml-0">
        <Button variant="ghost" as-child size="sm" class="hidden sm:flex">
          <a
            href="/__devconsole"
            rel="noopener noreferrer"
            target="_self"
            class="dark:text-foreground"
          >
            Dev Console
          </a>
        </Button>
      </div>
    </div>
    <div
      v-if="isMonitoringArea"
      class="flex items-center gap-2 overflow-x-auto px-4 pb-2 md:hidden"
    >
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

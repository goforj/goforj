<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from 'vue'
import { useRoute } from 'vue-router'
import { CirclePause, HeartPulse, Pause, Server, ShieldAlert } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import HeartbeatStrip from '@/components/HeartbeatStrip.vue'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'

type Monitor = {
  id?: string
  name?: string
  target?: string
  enabled?: boolean
  last_status?: string
}

const route = useRoute()
const loading = ref(false)
const monitors = ref<Monitor[]>([])
const heartbeats = ref<Record<string, string[]>>({})
const heartbeatPoints = ref<Record<string, Array<{ status?: string; checked_at?: string; latency_ms?: number }>>>({})
const query = ref('')
const state = ref<'all' | 'up' | 'down' | 'paused'>('all')

const selectedMonitorID = computed(() => String(route.params.id || ''))

const filtered = computed(() => {
  const q = query.value.trim().toLowerCase()
  return monitors.value.filter((m) => {
    if (q) {
      const haystack = `${m.name || ''} ${m.target || ''}`.toLowerCase()
      if (!haystack.includes(q)) return false
    }
    const s = (m.last_status || '').toLowerCase()
    if (state.value === 'up' && (m.enabled === false || s !== 'up')) return false
    if (state.value === 'down' && (m.enabled === false || s === 'up')) return false
    if (state.value === 'paused' && m.enabled !== false) return false
    return true
  })
})

async function load() {
  loading.value = true
  try {
    const [monitorRes, hbRes] = await Promise.all([
      fetch('/api/v1/monitoring/monitors'),
      fetch('/api/v1/monitoring/heartbeats?limit=12'),
    ])
    if (monitorRes.ok) {
      const payload = await monitorRes.json()
      monitors.value = Array.isArray(payload.monitors) ? payload.monitors : []
    }
    if (hbRes.ok) {
      const payload = await hbRes.json()
      heartbeats.value = payload.heartbeats && typeof payload.heartbeats === 'object' ? payload.heartbeats : {}
      heartbeatPoints.value =
        payload.heartbeat_points && typeof payload.heartbeat_points === 'object'
          ? payload.heartbeat_points
          : {}
    }
  } finally {
    loading.value = false
  }
}

onMounted(load)

let refreshTimer: number | null = null
onMounted(() => {
  refreshTimer = window.setInterval(() => {
    void load()
  }, 15000)
})
onUnmounted(() => {
  if (refreshTimer !== null) {
    window.clearInterval(refreshTimer)
    refreshTimer = null
  }
})

function filterButtonClass(filter: 'all' | 'up' | 'down' | 'paused') {
  if (state.value !== filter) return 'text-muted-foreground'
  if (filter === 'up') return 'border-emerald-500/40 text-emerald-400 bg-emerald-500/10'
  if (filter === 'down') return 'border-rose-500/40 text-rose-400 bg-rose-500/10'
  if (filter === 'paused') return 'border-amber-500/40 text-amber-400 bg-amber-500/10'
  return 'border-border text-foreground bg-muted/30'
}
</script>

<template>
  <SidebarGroup class="group-data-[collapsible=icon]:hidden">
    <SidebarGroupLabel>Monitors</SidebarGroupLabel>
    <SidebarGroupContent class="space-y-2">
      <Button as-child size="sm" class="w-full justify-start">
        <RouterLink to="/monitors/new">+ New Monitor</RouterLink>
      </Button>
      <Input v-model="query" placeholder="Search hosts..." class="h-8 text-xs" />
      <div class="grid grid-cols-4 gap-1">
        <Button
          size="sm"
          variant="outline"
          class="h-6 min-w-0 gap-1 px-1.5 text-[11px]"
          :class="filterButtonClass('all')"
          @click="state = 'all'"
        >
          <Server class="size-3" />
          All
        </Button>
        <Button
          size="sm"
          variant="outline"
          class="h-6 min-w-0 gap-1 px-1.5 text-[11px]"
          :class="filterButtonClass('up')"
          @click="state = 'up'"
        >
          <HeartPulse class="size-3" />
          Up
        </Button>
        <Button
          size="sm"
          variant="outline"
          class="h-6 min-w-0 gap-1 px-1.5 text-[11px]"
          :class="filterButtonClass('down')"
          @click="state = 'down'"
        >
          <ShieldAlert class="size-3" />
          Down
        </Button>
        <Button
          size="sm"
          variant="outline"
          class="h-6 min-w-0 gap-1 px-1.5 text-[11px]"
          :class="filterButtonClass('paused')"
          @click="state = 'paused'"
        >
          <CirclePause class="size-3" />
          Paused
        </Button>
      </div>
      <SidebarMenu>
        <SidebarMenuItem v-if="loading">
          <SidebarMenuButton disabled>
            <span>Loading monitors...</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
        <SidebarMenuItem v-else-if="!filtered.length">
          <SidebarMenuButton disabled>
            <span>No monitors</span>
          </SidebarMenuButton>
        </SidebarMenuItem>
        <SidebarMenuItem v-for="monitor in filtered.slice(0, 20)" :key="monitor.id || monitor.name">
          <SidebarMenuButton as-child :is-active="selectedMonitorID === (monitor.id || '')">
            <RouterLink :to="`/monitors/${monitor.id || ''}`" class="flex w-full items-center gap-2">
              <div class="min-w-0 flex flex-1 items-center justify-between gap-2">
                <div class="flex min-w-0 items-center gap-2">
                  <Badge
                    variant="outline"
                    class="h-4 min-w-8 justify-center rounded-full px-1 text-[10px]"
                    :class="
                      monitor.enabled === false
                        ? 'border-amber-500/40 text-amber-400'
                        : (monitor.last_status || '').toLowerCase() === 'up'
                        ? 'border-emerald-500/40 text-emerald-400'
                        : 'border-rose-500/40 text-rose-400'
                    "
                  >
                    <Pause v-if="monitor.enabled === false" class="size-3" />
                    <template v-else>
                      {{ (monitor.last_status || 'n/a').toLowerCase() === 'up' ? 'up' : 'down' }}
                    </template>
                  </Badge>
                  <span class="truncate">{{ monitor.name || monitor.target || 'Monitor' }}</span>
                </div>
                <HeartbeatStrip
                  class="shrink-0"
                  size="sm"
                  :statuses="heartbeats[monitor.id || ''] || Array(12).fill('unknown')"
                  :points="
                    (heartbeatPoints[monitor.id || ''] || []).map((point) => ({
                      status: point?.status,
                      checkedAt: point?.checked_at,
                      latencyMs: point?.latency_ms,
                    }))
                  "
                />
              </div>
            </RouterLink>
          </SidebarMenuButton>
        </SidebarMenuItem>
      </SidebarMenu>
    </SidebarGroupContent>
  </SidebarGroup>
</template>

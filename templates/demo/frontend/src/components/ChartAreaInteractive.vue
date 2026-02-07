<script setup lang="ts">
import { computed, ref } from 'vue'
import { VisArea, VisAxis, VisLine, VisXYContainer } from '@unovis/vue'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { ChartContainer, type ChartConfig } from '@/components/ui/chart'

type Check = {
  checked_at?: string
  duration_ms?: number
  status?: string
}

const props = defineProps<{
  monitorName?: string
  checks: Check[]
  range: '1h' | '24h' | '7d' | '30d'
}>()
const emit = defineEmits<{
  'update:range': [value: '1h' | '24h' | '7d' | '30d']
}>()
const range = computed({
  get: () => props.range,
  set: (value: '1h' | '24h' | '7d' | '30d') => emit('update:range', value),
})

const chartConfig = {
  latency: {
    label: 'Latency',
    color: 'var(--chart-2)',
  },
} satisfies ChartConfig

type ChartPoint = {
  ts: number
  label: string
  ms: number
}

function parseTime(value?: string): number {
  if (!value) return Date.now()
  const t = Date.parse(value)
  if (Number.isNaN(t)) return Date.now()
  return t
}

function horizonDurationMs(value: '1h' | '24h' | '7d' | '30d'): number {
  if (value === '1h') return 60 * 60 * 1000
  if (value === '7d') return 7 * 24 * 60 * 60 * 1000
  if (value === '30d') return 30 * 24 * 60 * 60 * 1000
  return 24 * 60 * 60 * 1000
}

const chartData = computed<ChartPoint[]>(() => {
  const now = Date.now()
  const horizonMs = horizonDurationMs(range.value)

  const rows = [...props.checks]
    .map((row) => {
      const ts = parseTime(row.checked_at)
      const status = String(row.status || '').toLowerCase()
      return {
        ts,
        ms: status === 'paused' ? 0 : Math.max(0, Number(row.duration_ms || 0)),
      }
    })
    .filter((row) => now - row.ts <= horizonMs)
  const merged = new Map<number, number>()
  for (const row of rows) {
    merged.set(row.ts, row.ms)
  }
  const normalized = Array.from(merged.entries())
    .map(([ts, ms]) => ({ ts, ms }))
    .sort((a, b) => a.ts - b.ts)

  const maxRows = range.value === '1h' ? 80 : range.value === '7d' ? 240 : range.value === '30d' ? 400 : 180
  return normalized.slice(-maxRows).map((row) => ({
    ts: row.ts,
    ms: row.ms,
    label: new Date(row.ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
  }))
})

const rangeSummary = computed(() => {
  const points = chartData.value
  const now = Date.now()
  const horizonStart = now - horizonDurationMs(range.value)
  const startLabel = new Date(horizonStart).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
  const endLabel = new Date(now).toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
  })
  return `${points.length} points · ${startLabel} - ${endLabel}`
})

const x = (d: ChartPoint) => d.ts
const y = (d: ChartPoint) => d.ms

const hoveredIndex = ref<number | null>(null)
const hoverX = ref<number>(0)

const hoveredPoint = computed(() => {
  if (hoveredIndex.value === null) return null
  return chartData.value[hoveredIndex.value] || null
})

function formatAxisTime(ts: number): string {
  const date = new Date(ts)
  if (range.value === '7d' || range.value === '30d') {
    return date.toLocaleString([], { month: 'short', day: 'numeric' })
  }
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

const chartBounds = computed(() => {
  const now = Date.now()
  const minTs = now - horizonDurationMs(range.value)
  return {
    minTs,
    maxTs: now,
  }
})

function xTickFormat(value: number) {
  return formatAxisTime(Number(value))
}

function onChartMove(event: MouseEvent) {
  if (!chartData.value.length) return
  const target = event.currentTarget as HTMLElement | null
  if (!target) return
  const rect = target.getBoundingClientRect()
  if (rect.width <= 0) return
  // Match hover coordinate to the actual series plotting area, not full card width.
  const plotLeft = 56
  const plotRight = 16
  const plotWidth = Math.max(1, rect.width - plotLeft - plotRight)
  const rawX = event.clientX - rect.left - plotLeft
  const clampedX = Math.max(0, Math.min(plotWidth, rawX))
  const ratio = clampedX / plotWidth
  const minTs = chartBounds.value.minTs
  const maxTs = chartBounds.value.maxTs
  if (maxTs <= minTs) {
    hoveredIndex.value = 0
    hoverX.value = plotLeft
    return
  }
  const hoverTs = minTs + ratio * (maxTs - minTs)
  let nearestIdx = 0
  let nearestDelta = Math.abs(chartData.value[0].ts - hoverTs)
  for (let i = 1; i < chartData.value.length; i++) {
    const delta = Math.abs(chartData.value[i].ts - hoverTs)
    if (delta < nearestDelta) {
      nearestDelta = delta
      nearestIdx = i
    }
  }
  hoveredIndex.value = nearestIdx
  const pointTs = chartData.value[nearestIdx].ts
  const pointRatio = (pointTs - minTs) / (maxTs - minTs)
  hoverX.value = plotLeft + pointRatio * plotWidth
}

function clearHover() {
  hoveredIndex.value = null
}
</script>

<template>
  <Card class="pt-0">
    <CardHeader class="flex items-center gap-2 space-y-0 border-b py-5 sm:flex-row">
      <div class="grid flex-1 gap-1">
        <CardTitle>Latency Trend</CardTitle>
        <CardDescription>
          {{ monitorName || 'Monitor' }} response time history
        </CardDescription>
      </div>
      <Select v-model="range">
        <SelectTrigger class="hidden w-[140px] rounded-lg sm:ml-auto sm:flex">
          <SelectValue placeholder="24h" />
        </SelectTrigger>
        <SelectContent class="rounded-xl">
          <SelectItem value="1h" class="rounded-lg">Last 1h</SelectItem>
          <SelectItem value="24h" class="rounded-lg">Last 24h</SelectItem>
          <SelectItem value="7d" class="rounded-lg">Last 7d</SelectItem>
          <SelectItem value="30d" class="rounded-lg">Last 30d</SelectItem>
        </SelectContent>
      </Select>
    </CardHeader>
    <CardContent class="px-2 pt-0 sm:px-5 sm:pt-0 pb-4">
      <div class="mb-3 flex items-center gap-4 text-xs text-muted-foreground">
        <div class="flex items-center gap-2">
          <span class="inline-block h-2 w-2 rounded-full bg-[var(--chart-2)]" />
          <span>Response (ms)</span>
        </div>
        <div class="ml-auto text-[11px] text-muted-foreground">{{ rangeSummary }}</div>
      </div>
      <div
        class="relative h-[320px] w-full rounded-md border border-border bg-background/40 p-3"
        @mousemove="onChartMove"
        @mouseleave="clearHover"
      >
        <div
          v-if="hoveredPoint"
          class="pointer-events-none absolute inset-y-3 z-10 w-px bg-primary/35"
          :style="{ left: `${hoverX}px` }"
        />
        <div
          v-if="hoveredPoint"
          class="pointer-events-none absolute top-2 z-20 -translate-x-1/2 rounded-md border border-border bg-card/95 px-2 py-1 text-xs shadow-sm"
          :style="{ left: `${hoverX}px` }"
        >
          <div class="font-medium text-foreground">{{ hoveredPoint.ms }}ms</div>
          <div class="text-muted-foreground">{{ hoveredPoint.label }}</div>
        </div>
        <ChartContainer :config="chartConfig" class="h-full w-full !aspect-auto !justify-start !block">
          <div v-if="!chartData.length" class="flex h-full items-center justify-center text-xs text-muted-foreground">
            no checks yet
          </div>
          <VisXYContainer
            v-else
            :data="chartData"
            :height="280"
            :xDomain="[chartBounds.minTs, chartBounds.maxTs]"
            class="h-full w-full"
          >
            <VisArea :x="x" :y="y" color="var(--chart-2)" :opacity="0.14" />
            <VisLine :x="x" :y="y" color="var(--chart-2)" />
            <VisAxis type="x" :numTicks="6" :tickFormat="xTickFormat" />
            <VisAxis type="y" :numTicks="5" />
          </VisXYContainer>
        </ChartContainer>
      </div>
    </CardContent>
  </Card>
</template>

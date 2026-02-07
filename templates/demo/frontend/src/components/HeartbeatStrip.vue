<script setup lang="ts">
import { computed } from 'vue'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '@/components/ui/tooltip'

type HeartbeatPoint = {
  status?: string
  checkedAt?: string
  latencyMs?: number
}

const props = withDefaults(
  defineProps<{
    statuses: string[]
    points?: Array<HeartbeatPoint | null>
    size?: 'sm' | 'md'
  }>(),
  {
    size: 'md',
  },
)

function statusClass(status: string) {
  const normalized = (status || '').toLowerCase()
  if (normalized === 'up') return 'bg-emerald-400'
  if (normalized === 'down') return 'bg-rose-400'
  if (normalized === 'paused') return 'bg-amber-400'
  if (normalized === 'pending') return 'bg-amber-400'
  return 'bg-muted-foreground/35'
}

function statusLabel(status: string): string {
  const normalized = (status || '').toLowerCase()
  if (normalized === 'up') return 'Up'
  if (normalized === 'down') return 'Down'
  if (normalized === 'paused') return 'Paused'
  if (normalized === 'pending') return 'Pending'
  return 'Unknown'
}

function formatCheckedAt(value?: string): string {
  if (!value) return 'n/a'
  const dt = new Date(value)
  if (Number.isNaN(dt.getTime())) return 'n/a'
  return dt.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

const items = computed(() =>
  props.statuses.map((status, idx) => ({
    status,
    point: props.points?.[idx] ?? null,
  })),
)
</script>

<template>
  <div class="flex items-center gap-1">
    <TooltipProvider>
      <Tooltip v-for="(item, idx) in items" :key="idx">
        <TooltipTrigger as-child>
          <span
            class="inline-block rounded-full"
            :class="[
              props.size === 'sm' ? 'h-3 w-1' : 'h-6 w-2',
              statusClass(item.status),
            ]"
          />
        </TooltipTrigger>
        <TooltipContent side="top" align="center" class="text-xs">
          <div class="font-medium">Status: {{ statusLabel(item.status) }}</div>
          <div class="text-muted-foreground">Time: {{ formatCheckedAt(item.point?.checkedAt) }}</div>
          <div class="text-muted-foreground">
            Latency:
            {{
              item.point?.latencyMs !== undefined && item.point?.latencyMs !== null
                ? `${Math.max(0, Number(item.point?.latencyMs || 0))}ms`
                : 'n/a'
            }}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  </div>
</template>

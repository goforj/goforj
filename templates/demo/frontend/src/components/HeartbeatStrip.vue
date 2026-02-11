<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'
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
const { t } = useI18n()

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
  if (normalized === 'up') return t('status.up')
  if (normalized === 'down') return t('status.down')
  if (normalized === 'paused') return t('monitoring.paused')
  if (normalized === 'pending') return t('status.pending')
  return t('status.unknown')
}

function formatCheckedAt(value?: string): string {
  if (!value) return t('common.na')
  const dt = new Date(value)
  if (Number.isNaN(dt.getTime())) return t('common.na')
  return dt.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

function formatRelativeTime(value?: string): string {
  if (!value) return t('common.na')
  const dt = new Date(value)
  if (Number.isNaN(dt.getTime())) return t('common.na')
  const diffMs = Date.now() - dt.getTime()
  if (diffMs < 0) return t('common.justNow')
  const sec = Math.floor(diffMs / 1000)
  if (sec < 10) return t('common.justNow')
  if (sec < 60) return t('relative.secondsAgo', { count: sec })
  const min = Math.floor(sec / 60)
  if (min < 60) return t('relative.minutesAgo', { count: min })
  const hr = Math.floor(min / 60)
  if (hr < 24) return t('relative.hoursAgo', { count: hr })
  const day = Math.floor(hr / 24)
  return t('relative.daysAgo', { count: day })
}

const items = computed(() => {
  const out = props.statuses.map((status, idx) => ({
    status,
    point: props.points?.[idx] ?? null,
  }))
  // Hide the still-open newest interval bucket so we do not render a
  // premature gray pill before the next monitor interval elapses.
  const tail = out[out.length - 1]
  if (tail && (tail.status || '').toLowerCase() === 'unknown' && !tail.point?.checkedAt) {
    out.pop()
  }
  return out
})
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
          <div class="font-medium">{{ t('heartbeat.status') }}: {{ statusLabel(item.status) }}</div>
          <div class="text-muted-foreground">
            {{ t('heartbeat.time') }}: {{ formatCheckedAt(item.point?.checkedAt) }} ({{ formatRelativeTime(item.point?.checkedAt) }})
          </div>
          <div class="text-muted-foreground">
            {{ t('heartbeat.latency') }}:
            {{
              item.point?.latencyMs !== undefined && item.point?.latencyMs !== null
                ? `${Math.max(0, Number(item.point?.latencyMs || 0))}ms`
                : t('common.na')
            }}
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  </div>
</template>

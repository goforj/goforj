<script setup lang="ts">
import { IconTrendingDown, IconTrendingUp } from "@tabler/icons-vue"
import { computed } from 'vue'

import { Badge } from '@/components/ui/badge'
import {
  Card,
  CardAction,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

type Summary = {
  stats?: {
    monitors_total?: number
    monitors_up?: number
    monitors_down?: number
    checks_last_hour?: number
  }
}

const props = defineProps<{
  summary: Summary | null
}>()

const total = computed(() => props.summary?.stats?.monitors_total ?? 0)
const up = computed(() => props.summary?.stats?.monitors_up ?? 0)
const down = computed(() => props.summary?.stats?.monitors_down ?? 0)
const checks = computed(() => props.summary?.stats?.checks_last_hour ?? 0)
</script>

<template>
  <div class="*:data-[slot=card]:from-primary/5 *:data-[slot=card]:to-card dark:*:data-[slot=card]:bg-card grid grid-cols-1 gap-4 px-4 *:data-[slot=card]:bg-gradient-to-t *:data-[slot=card]:shadow-xs lg:px-6 @xl/main:grid-cols-2 @5xl/main:grid-cols-4">
    <Card class="@container/card">
      <CardHeader>
        <CardDescription>Total Monitors</CardDescription>
        <CardTitle class="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {{ total }}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">
            <IconTrendingUp />
            Live
          </Badge>
        </CardAction>
      </CardHeader>
      <CardFooter class="flex-col items-start gap-1.5 text-sm">
        <div class="line-clamp-1 flex gap-2 font-medium">
          Active monitor inventory <IconTrendingUp class="size-4" />
        </div>
        <div class="text-muted-foreground">
          Tracks configured uptime checks
        </div>
      </CardFooter>
    </Card>
    <Card class="@container/card">
      <CardHeader>
        <CardDescription>Monitors Up</CardDescription>
        <CardTitle class="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {{ up }}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">
            <IconTrendingUp />
            Healthy
          </Badge>
        </CardAction>
      </CardHeader>
      <CardFooter class="flex-col items-start gap-1.5 text-sm">
        <div class="line-clamp-1 flex gap-2 font-medium">
          Currently reachable <IconTrendingUp class="size-4" />
        </div>
        <div class="text-muted-foreground">
          HTTP checks within threshold
        </div>
      </CardFooter>
    </Card>
    <Card class="@container/card">
      <CardHeader>
        <CardDescription>Monitors Down</CardDescription>
        <CardTitle class="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {{ down }}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">
            <IconTrendingDown />
            Alert
          </Badge>
        </CardAction>
      </CardHeader>
      <CardFooter class="flex-col items-start gap-1.5 text-sm">
        <div class="line-clamp-1 flex gap-2 font-medium">
          Requires investigation <IconTrendingDown class="size-4" />
        </div>
        <div class="text-muted-foreground">
          Monitors failing last check cycle
        </div>
      </CardFooter>
    </Card>
    <Card class="@container/card">
      <CardHeader>
        <CardDescription>Checks (1h)</CardDescription>
        <CardTitle class="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {{ checks }}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">
            <IconTrendingUp />
            Throughput
          </Badge>
        </CardAction>
      </CardHeader>
      <CardFooter class="flex-col items-start gap-1.5 text-sm">
        <div class="line-clamp-1 flex gap-2 font-medium">
          Check runner activity <IconTrendingUp class="size-4" />
        </div>
        <div class="text-muted-foreground">
          Completed in previous hour
        </div>
      </CardFooter>
    </Card>
  </div>
</template>

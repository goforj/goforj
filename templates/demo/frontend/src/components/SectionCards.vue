<script setup lang="ts">
import { IconTrendingDown, IconTrendingUp } from "@tabler/icons-vue"
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

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
const { t } = useI18n()
</script>

<template>
  <div class="*:data-[slot=card]:from-primary/5 *:data-[slot=card]:to-card dark:*:data-[slot=card]:bg-card grid grid-cols-1 gap-4 px-4 *:data-[slot=card]:bg-gradient-to-t *:data-[slot=card]:shadow-xs lg:px-6 @xl/main:grid-cols-2 @5xl/main:grid-cols-4">
    <Card class="@container/card">
      <CardHeader>
        <CardDescription>{{ t('sectionCards.totalMonitors') }}</CardDescription>
        <CardTitle class="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {{ total }}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">
            <IconTrendingUp />
            {{ t('sectionCards.live') }}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardFooter class="flex-col items-start gap-1.5 text-sm">
        <div class="line-clamp-1 flex gap-2 font-medium">
          {{ t('sectionCards.activeInventory') }} <IconTrendingUp class="size-4" />
        </div>
        <div class="text-muted-foreground">
          {{ t('sectionCards.tracksChecks') }}
        </div>
      </CardFooter>
    </Card>
    <Card class="@container/card">
      <CardHeader>
        <CardDescription>{{ t('sectionCards.monitorsUp') }}</CardDescription>
        <CardTitle class="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {{ up }}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">
            <IconTrendingUp />
            {{ t('sectionCards.healthy') }}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardFooter class="flex-col items-start gap-1.5 text-sm">
        <div class="line-clamp-1 flex gap-2 font-medium">
          {{ t('sectionCards.currentlyReachable') }} <IconTrendingUp class="size-4" />
        </div>
        <div class="text-muted-foreground">
          {{ t('sectionCards.httpWithinThreshold') }}
        </div>
      </CardFooter>
    </Card>
    <Card class="@container/card">
      <CardHeader>
        <CardDescription>{{ t('sectionCards.monitorsDown') }}</CardDescription>
        <CardTitle class="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {{ down }}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">
            <IconTrendingDown />
            {{ t('sectionCards.alert') }}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardFooter class="flex-col items-start gap-1.5 text-sm">
        <div class="line-clamp-1 flex gap-2 font-medium">
          {{ t('sectionCards.requiresInvestigation') }} <IconTrendingDown class="size-4" />
        </div>
        <div class="text-muted-foreground">
          {{ t('sectionCards.failingLastCycle') }}
        </div>
      </CardFooter>
    </Card>
    <Card class="@container/card">
      <CardHeader>
        <CardDescription>{{ t('monitoring.checksOneHour') }}</CardDescription>
        <CardTitle class="text-2xl font-semibold tabular-nums @[250px]/card:text-3xl">
          {{ checks }}
        </CardTitle>
        <CardAction>
          <Badge variant="outline">
            <IconTrendingUp />
            {{ t('sectionCards.throughput') }}
          </Badge>
        </CardAction>
      </CardHeader>
      <CardFooter class="flex-col items-start gap-1.5 text-sm">
        <div class="line-clamp-1 flex gap-2 font-medium">
          {{ t('sectionCards.checkRunnerActivity') }} <IconTrendingUp class="size-4" />
        </div>
        <div class="text-muted-foreground">
          {{ t('sectionCards.completedPreviousHour') }}
        </div>
      </CardFooter>
    </Card>
  </div>
</template>

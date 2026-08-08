<template>
  <section class="grid gap-6">
    <PageHeader
      eyebrow="Application"
      section="Overview"
      title="Dashboard"
      description="A working shell you can point at your own data. Replace these panels with the first workflow your product actually needs."
    />

    <div class="grid gap-6 sm:grid-cols-2 xl:grid-cols-4">
      <Card v-for="metric in metrics" :key="metric.label">
        <CardHeader class="gap-1">
          <CardDescription>{{ metric.label }}</CardDescription>
          <CardTitle class="text-2xl font-semibold tracking-tight">{{ metric.value }}</CardTitle>
        </CardHeader>
        <CardContent>
          <p class="flex items-center gap-1.5 text-sm text-muted-foreground">
            <component :is="metric.trend === 'up' ? ArrowUpRight : ArrowDownRight" class="size-3.5" />
            {{ metric.change }}
          </p>
        </CardContent>
      </Card>
    </div>

    <Card>
      <CardHeader>
        <CardTitle class="text-lg font-semibold tracking-tight">Requests and errors</CardTitle>
        <CardDescription>Traffic handled by the generated API over the last six months.</CardDescription>
      </CardHeader>
      <CardContent>
        <ChartContainer :config="chartConfig" class="h-64 w-full">
          <VisXYContainer :data="chartData">
            <VisGroupedBar
              :x="(d: ChartDatum) => d.date"
              :y="[(d: ChartDatum) => d.requests, (d: ChartDatum) => d.errors]"
              :color="[chartConfig.requests.color, chartConfig.errors.color]"
              :rounded-corners="4"
              bar-padding="0.1"
              group-padding="0"
            />
            <VisAxis
              type="x"
              :x="(d: ChartDatum) => d.date"
              :tick-line="false"
              :domain-line="false"
              :grid-line="false"
              :tick-values="chartData.map(d => d.date)"
              :tick-format="formatMonth"
            />
            <VisAxis
              type="y"
              :tick-line="false"
              :domain-line="false"
              :grid-line="true"
              :tick-format="formatCompact"
            />
            <ChartTooltip />
            <ChartCrosshair
              :template="crosshairTemplate"
              :color="[chartConfig.requests.color, chartConfig.errors.color]"
            />
          </VisXYContainer>
          <ChartLegendContent />
        </ChartContainer>
      </CardContent>
    </Card>

    <div class="grid gap-6 xl:grid-cols-[1.4fr_1fr]">
      <Card class="overflow-hidden">
        <CardHeader>
          <CardTitle class="text-lg font-semibold tracking-tight">Resources</CardTitle>
          <CardDescription>Routes and screens tracked by the shell.</CardDescription>
        </CardHeader>
        <CardContent class="px-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead class="pl-6">Resource</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Owner</TableHead>
                <TableHead class="pr-6 text-right">Updated</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              <TableRow v-for="row in resources" :key="row.name">
                <TableCell class="pl-6 font-medium">{{ row.name }}</TableCell>
                <TableCell>
                  <Badge :variant="row.variant">{{ row.status }}</Badge>
                </TableCell>
                <TableCell class="text-muted-foreground">{{ row.owner }}</TableCell>
                <TableCell class="pr-6 text-right text-muted-foreground">{{ row.updated }}</TableCell>
              </TableRow>
            </TableBody>
          </Table>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle class="text-lg font-semibold tracking-tight">Recent activity</CardTitle>
          <CardDescription>Events emitted while the app was starting up.</CardDescription>
        </CardHeader>
        <CardContent class="grid gap-3">
          <Item v-for="entry in activity" :key="entry.title" variant="outline" size="sm">
            <ItemMedia variant="icon">
              <component :is="entry.icon" class="size-4" />
            </ItemMedia>
            <ItemContent>
              <ItemTitle>{{ entry.title }}</ItemTitle>
              <ItemDescription>{{ entry.description }}</ItemDescription>
            </ItemContent>
            <ItemActions>
              <span class="text-xs text-muted-foreground">{{ entry.time }}</span>
            </ItemActions>
          </Item>
        </CardContent>
      </Card>
    </div>
  </section>
</template>

<script setup lang="ts">
import type { ChartConfig } from '@/components/ui/chart'
import { VisAxis, VisGroupedBar, VisXYContainer } from '@unovis/vue'
import { ArrowDownRight, ArrowUpRight, GitFork, ShieldCheck, Sparkles, Workflow } from '@lucide/vue'
import PageHeader from '@/components/PageHeader.vue'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import {
  ChartContainer,
  ChartCrosshair,
  ChartLegendContent,
  ChartTooltip,
  ChartTooltipContent,
  componentToString,
} from '@/components/ui/chart'
import { Item, ItemActions, ItemContent, ItemDescription, ItemMedia, ItemTitle } from '@/components/ui/item'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const metrics = [
  { label: 'Requests today', value: '18.4k', change: '12% vs last week', trend: 'up' as const },
  { label: 'Median latency', value: '84ms', change: '9ms faster', trend: 'down' as const },
  { label: 'Failed jobs', value: '12', change: 'Down from 31', trend: 'down' as const },
  { label: 'Active sessions', value: '327', change: '4% vs yesterday', trend: 'up' as const },
]

const chartData = [
  { date: new Date('2026-03-01'), requests: 12400, errors: 320 },
  { date: new Date('2026-04-01'), requests: 15200, errors: 280 },
  { date: new Date('2026-05-01'), requests: 14100, errors: 410 },
  { date: new Date('2026-06-01'), requests: 17600, errors: 240 },
  { date: new Date('2026-07-01'), requests: 16800, errors: 190 },
  { date: new Date('2026-08-01'), requests: 18400, errors: 120 },
]

type ChartDatum = typeof chartData[number]

const chartConfig = {
  requests: { label: 'Requests', color: 'var(--chart-1)' },
  errors: { label: 'Errors', color: 'var(--chart-2)' },
} satisfies ChartConfig

const crosshairTemplate = componentToString(chartConfig, ChartTooltipContent, {
  labelFormatter(d: number) {
    return new Date(d).toLocaleDateString('en-US', { month: 'long', year: 'numeric' })
  },
})

const resources = [
  { name: 'Dashboard shell', status: 'Ready', variant: 'default' as const, owner: 'Frontend', updated: 'Just now' },
  { name: 'Auth screens', status: 'Draft', variant: 'secondary' as const, owner: 'Platform', updated: '2h ago' },
  { name: 'Components gallery', status: 'Ready', variant: 'default' as const, owner: 'Frontend', updated: '15m ago' },
  { name: 'Settings views', status: 'Draft', variant: 'secondary' as const, owner: 'Platform', updated: '1d ago' },
  { name: 'Metrics endpoint', status: 'Blocked', variant: 'destructive' as const, owner: 'API', updated: 'Yesterday' },
]

const activity = [
  { title: 'Starter rendered', description: 'Vue shell copied into the generated frontend folder.', time: '00:37', icon: Sparkles },
  { title: 'Session bootstrapped', description: 'The shell resolved the current user from /api/v1/auth/me.', time: '00:37', icon: ShieldCheck },
  { title: 'Migrations applied', description: 'Schema is current with the generated migration set.', time: '00:38', icon: GitFork },
  { title: 'Queue worker online', description: 'Background jobs are being consumed.', time: '00:38', icon: Workflow },
]

function formatMonth(d: number) {
  return new Date(d).toLocaleDateString('en-US', { month: 'short' })
}

function formatCompact(d: number) {
  return Intl.NumberFormat('en-US', { notation: 'compact' }).format(d)
}
</script>

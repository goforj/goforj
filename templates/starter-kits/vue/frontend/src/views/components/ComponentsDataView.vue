<template>
  <section class="grid gap-6">
    <PageHeader
      eyebrow="Components"
      section="Data"
      title="Tables, pagination, and dates"
      description="Reference patterns for resource indexes, operational reporting, scheduling, and review workflows."
    />

    <div class="grid gap-6 md:grid-cols-3">
      <Card v-for="metric in metrics" :key="metric.label">
        <CardHeader class="gap-1">
          <CardDescription>{{ metric.label }}</CardDescription>
          <CardTitle class="text-2xl font-semibold tracking-tight">{{ metric.value }}</CardTitle>
        </CardHeader>
        <CardContent>
          <p class="text-sm text-muted-foreground">{{ metric.copy }}</p>
        </CardContent>
      </Card>
    </div>

    <Specimen
      name="Table"
      description="A low-level primitive with no opinion on chrome. These are three product treatments built from the same parts: an admin index, a dense event stream, and a financial summary."
      :also="['Badge', 'Button', 'InputGroup', 'Select']"
    >
      <div class="grid gap-3 lg:grid-cols-[1fr_auto_auto]">
        <InputGroup>
          <InputGroupAddon>
            <Search class="size-4" />
          </InputGroupAddon>
          <InputGroupInput v-model="searchQuery" placeholder="Search resources..." />
        </InputGroup>

        <Select v-model="statusFilter">
          <SelectTrigger class="min-w-40">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="ready">Ready</SelectItem>
            <SelectItem value="draft">Draft</SelectItem>
            <SelectItem value="blocked">Blocked</SelectItem>
          </SelectContent>
        </Select>

        <Button variant="outline" @click="exportResources">Export CSV</Button>
      </div>

      <div class="overflow-hidden rounded-lg border">
        <div class="flex items-center justify-between gap-4 border-b bg-muted/40 px-4 py-3">
          <div class="grid gap-0.5">
            <p class="text-sm font-medium leading-none">Resource index</p>
            <p class="text-sm text-muted-foreground">A standard admin listing with filters, ownership, and route-level context.</p>
          </div>
          <Badge variant="outline">{{ filteredRows.length }} visible</Badge>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Resource</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Owner</TableHead>
              <TableHead>Route</TableHead>
              <TableHead class="text-right">Updated</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="row in filteredRows" :key="row.name">
              <TableCell class="font-medium">{{ row.name }}</TableCell>
              <TableCell>
                <Badge :variant="row.statusVariant">{{ row.status }}</Badge>
              </TableCell>
              <TableCell>{{ row.owner }}</TableCell>
              <TableCell class="font-mono text-xs text-muted-foreground">{{ row.route }}</TableCell>
              <TableCell class="text-right text-muted-foreground">{{ row.updated }}</TableCell>
            </TableRow>
            <TableRow v-if="!filteredRows.length">
              <TableCell colspan="5" class="py-8 text-center text-sm text-muted-foreground">
                No resources match that search.
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>

      <div class="overflow-hidden rounded-lg border">
        <div class="flex items-center justify-between gap-4 border-b bg-muted/40 px-4 py-3">
          <div class="grid gap-0.5">
            <p class="text-sm font-medium leading-none">Audit log</p>
            <p class="text-sm text-muted-foreground">A dense event stream with subdued chrome and clear status emphasis.</p>
          </div>
          <Badge variant="outline">Realtime</Badge>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Event</TableHead>
              <TableHead>Actor</TableHead>
              <TableHead>Status</TableHead>
              <TableHead class="text-right">Time</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="entry in auditRows" :key="entry.event" class="hover:bg-muted/40">
              <TableCell class="font-medium">{{ entry.event }}</TableCell>
              <TableCell class="text-muted-foreground">{{ entry.actor }}</TableCell>
              <TableCell>
                <Badge :variant="entry.variant">{{ entry.status }}</Badge>
              </TableCell>
              <TableCell class="text-right text-xs text-muted-foreground">{{ entry.time }}</TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>

      <div class="overflow-hidden rounded-lg border">
        <div class="flex items-center justify-between gap-4 border-b bg-muted/40 px-4 py-3">
          <div class="grid gap-0.5">
            <p class="text-sm font-medium leading-none">Invoice summary</p>
            <p class="text-sm text-muted-foreground">A financial table with stronger row separation and right-aligned values.</p>
          </div>
          <Button variant="outline" size="sm">Download all</Button>
        </div>
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Invoice</TableHead>
              <TableHead>Customer</TableHead>
              <TableHead>State</TableHead>
              <TableHead class="text-right">Amount</TableHead>
              <TableHead class="text-right">Action</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            <TableRow v-for="invoice in invoiceRows" :key="invoice.number">
              <TableCell>
                <div class="grid gap-0.5">
                  <span class="font-medium">{{ invoice.number }}</span>
                  <span class="text-xs text-muted-foreground">{{ invoice.period }}</span>
                </div>
              </TableCell>
              <TableCell>{{ invoice.customer }}</TableCell>
              <TableCell>
                <Badge :variant="invoice.variant">{{ invoice.state }}</Badge>
              </TableCell>
              <TableCell class="text-right font-medium">{{ invoice.amount }}</TableCell>
              <TableCell class="text-right">
                <Button variant="ghost" size="sm">View</Button>
              </TableCell>
            </TableRow>
          </TableBody>
        </Table>
      </div>
    </Specimen>

    <Specimen
      name="Pagination"
      description="Page controls with ellipsis collapsing. Drives the page number through v-model:page."
    >
      <div class="flex flex-wrap items-center justify-between gap-3">
        <p class="text-sm text-muted-foreground">Page {{ currentPage }} of 10</p>
        <Pagination v-model:page="currentPage" :items-per-page="10" :total="100">
          <PaginationContent v-slot="{ items }">
            <PaginationPrevious />
            <template v-for="(item, index) in items" :key="index">
              <PaginationItem
                v-if="item.type === 'page'"
                :value="item.value"
                :is-active="item.value === currentPage"
              >
                {{ item.value }}
              </PaginationItem>
              <PaginationEllipsis v-else :index="index" />
            </template>
            <PaginationNext />
          </PaginationContent>
        </Pagination>
      </div>
    </Specimen>

    <div class="grid gap-6 xl:grid-cols-2">
      <Specimen
        name="Calendar"
        description="Single date selection for launch planning and reporting windows."
        :also="['Input']"
      >
        <div class="grid justify-items-center gap-3">
          <Input :model-value="formattedSingleDate" readonly class="h-8 w-44 text-center text-sm" />
          <Calendar v-model="singleDate" class="rounded-lg border p-3" />
        </div>
      </Specimen>

      <Specimen
        name="RangeCalendar"
        description="Start and end selection for booking flows and release coordination."
        :also="['Input']"
      >
        <div class="grid justify-items-center gap-3">
          <div class="flex flex-wrap justify-center gap-2">
            <Input :model-value="formattedRangeStart" readonly class="h-8 w-44 text-center text-sm" />
            <Input :model-value="formattedRangeEnd" readonly class="h-8 w-44 text-center text-sm" />
          </div>
          <RangeCalendar v-model="dateRange" class="rounded-lg border p-3" />
        </div>
      </Specimen>
    </div>

    <Specimen
      name="Item"
      description="A compact row with content and trailing actions. Use for queues, scheduled windows, and other operational lists."
      :also="['Badge']"
    >
      <ShowcaseRow label="Upcoming windows">
        <Item v-for="window in releaseWindows" :key="window.title" size="sm" variant="outline">
          <ItemContent>
            <ItemTitle>{{ window.title }}</ItemTitle>
            <ItemDescription>{{ window.description }}</ItemDescription>
          </ItemContent>
          <ItemActions>
            <Badge :variant="window.variant">{{ window.status }}</Badge>
          </ItemActions>
        </Item>
      </ShowcaseRow>

      <ShowcaseRow label="Queue and dataset states">
        <Item v-for="job in jobs" :key="job.title" variant="outline" size="sm">
          <ItemContent>
            <ItemTitle>{{ job.title }}</ItemTitle>
            <ItemDescription>{{ job.description }}</ItemDescription>
          </ItemContent>
          <ItemActions>
            <Badge :variant="job.variant">{{ job.status }}</Badge>
          </ItemActions>
        </Item>
      </ShowcaseRow>
    </Specimen>

    <Showcase
      title="Reporting snapshots"
      description="Small summary blocks help table and calendar sections read as a dashboard instead of isolated widgets."
      content-class="md:grid-cols-3"
    >
      <ShowcaseRow v-for="snapshot in snapshots" :key="snapshot.label" class="gap-1">
        <p class="text-sm text-muted-foreground">{{ snapshot.label }}</p>
        <p class="text-2xl font-semibold tracking-tight">{{ snapshot.value }}</p>
        <p class="text-sm text-muted-foreground">{{ snapshot.copy }}</p>
      </ShowcaseRow>
    </Showcase>
  </section>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { getLocalTimeZone, parseDate, today } from '@internationalized/date'
import { Search } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Calendar } from '@/components/ui/calendar'
import { PageHeader, Showcase, ShowcaseRow, Specimen } from '@/components/showcase'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Item, ItemActions, ItemContent, ItemDescription, ItemTitle } from '@/components/ui/item'
import {
  Pagination,
  PaginationContent,
  PaginationEllipsis,
  PaginationItem,
  PaginationNext,
  PaginationPrevious,
} from '@/components/ui/pagination'
import { RangeCalendar } from '@/components/ui/range-calendar'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const currentPage = ref(3)
const statusFilter = ref('all')
const searchQuery = ref('')
const singleDate = ref(today(getLocalTimeZone()))
const dateRange = ref({
  start: parseDate('2026-04-23'),
  end: parseDate('2026-04-30'),
})

const metrics = [
  { label: 'Indexed resources', value: '128', copy: 'Routes, screens, and generated surfaces tracked by the shell.' },
  { label: 'Ready to ship', value: '42', copy: 'Resources currently in a production-ready state.' },
  { label: 'Scheduled windows', value: '3', copy: 'Upcoming launches, cutovers, or maintenance windows.' },
]

const rows = [
  { name: 'Dashboard shell', status: 'Ready', statusVariant: 'default' as const, owner: 'Frontend', route: '/', updated: 'Just now' },
  { name: 'Auth screens', status: 'Draft', statusVariant: 'secondary' as const, owner: 'Platform', route: '/login', updated: '2h ago' },
  { name: 'Metrics cards', status: 'Blocked', statusVariant: 'destructive' as const, owner: 'API', route: '/metrics', updated: 'Yesterday' },
  { name: 'Components gallery', status: 'Ready', statusVariant: 'default' as const, owner: 'Frontend', route: '/components', updated: '15m ago' },
  { name: 'Settings view', status: 'Draft', statusVariant: 'secondary' as const, owner: 'Platform', route: '/settings', updated: '1d ago' },
]

const releaseWindows = [
  { title: 'Starter kit polish', description: 'UI audit and cleanup pass across components routes.', status: 'Today', variant: 'default' as const },
  { title: 'Auth route handoff', description: 'Wire account pages to generated auth endpoints.', status: 'Tomorrow', variant: 'secondary' as const },
  { title: 'API metrics rollout', description: 'Blocked pending backend instrumentation fixes.', status: 'Blocked', variant: 'destructive' as const },
]

const jobs = [
  { title: 'Export component inventory', description: 'Generate a source-owned report of installed UI primitives.', status: 'Queued', variant: 'secondary' as const },
  { title: 'Render preview build', description: 'Assemble a production bundle for design review.', status: 'Running', variant: 'default' as const },
  { title: 'Sync usage analytics', description: 'Blocked until the metrics query logger lands.', status: 'Blocked', variant: 'destructive' as const },
]

const snapshots = [
  { label: 'Daily events', value: '18.4k', copy: 'Across API, auth, and renderer flows.' },
  { label: 'Failed jobs', value: '12', copy: 'Down from 31 after the last deploy.' },
  { label: 'Median latency', value: '84ms', copy: 'Healthy for the current load profile.' },
]

const auditRows = [
  { event: 'Starter rendered', actor: 'forj cli', status: 'Success', variant: 'default' as const, time: '00:37:07' },
  { event: 'Auth me bootstrap', actor: 'frontend app', status: 'Success', variant: 'default' as const, time: '00:37:12' },
  { event: 'Migration applied', actor: 'forj cli', status: 'Success', variant: 'default' as const, time: '00:37:15' },
  { event: 'Queue worker connect', actor: 'worker', status: 'Failed', variant: 'destructive' as const, time: '00:37:19' },
]

const invoiceRows = [
  { number: 'INV-2048', period: 'April 2026', customer: 'Acme Labs', state: 'Paid', variant: 'default' as const, amount: '$1,240.00' },
  { number: 'INV-2049', period: 'April 2026', customer: 'Northwind', state: 'Pending', variant: 'secondary' as const, amount: '$860.00' },
  { number: 'INV-2050', period: 'April 2026', customer: 'Orbit Systems', state: 'Overdue', variant: 'destructive' as const, amount: '$2,410.00' },
]

const filteredRows = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()

  return rows.filter((row) => {
    const matchesStatus = statusFilter.value === 'all' || row.status.toLowerCase() === statusFilter.value
    const matchesQuery = !query
      || [row.name, row.owner, row.route, row.status].some(value => value.toLowerCase().includes(query))

    return matchesStatus && matchesQuery
  })
})

const dateFormatter = new Intl.DateTimeFormat('en-US', {
  month: 'short',
  day: 'numeric',
  year: 'numeric',
})

const formattedSingleDate = computed(() => formatDateValue(singleDate.value))
const formattedRangeStart = computed(() => formatDateValue(dateRange.value?.start) || 'Start date')
const formattedRangeEnd = computed(() => formatDateValue(dateRange.value?.end) || 'End date')

function exportResources() {
  const header = ['Resource', 'Status', 'Owner', 'Route', 'Updated']
  const lines = filteredRows.value.map(row => [row.name, row.status, row.owner, row.route, row.updated])
  const csv = [header, ...lines]
    .map(columns => columns.map(value => `"${String(value).replaceAll('"', '""')}"`).join(','))
    .join('\n')

  const blob = new Blob([csv], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = 'starter-resources.csv'
  link.click()
  URL.revokeObjectURL(url)
}

function formatDateValue(value?: { toDate: (timeZone: string) => Date } | null) {
  if (!value) {
    return ''
  }

  return dateFormatter.format(value.toDate(getLocalTimeZone()))
}
</script>

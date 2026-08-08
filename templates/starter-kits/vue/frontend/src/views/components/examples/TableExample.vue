<script setup lang="ts">
import { computed, ref } from 'vue'
import { Search } from '@lucide/vue'
import { Badge } from '@/components/ui/badge'
import { InputGroup, InputGroupAddon, InputGroupInput } from '@/components/ui/input-group'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'

const search = ref('')
const status = ref('all')

const rows = [
  { name: 'Dashboard shell', status: 'Ready', variant: 'default' as const, owner: 'Frontend', route: '/', updated: 'Just now' },
  { name: 'Auth screens', status: 'Draft', variant: 'secondary' as const, owner: 'Platform', route: '/login', updated: '2h ago' },
  { name: 'Metrics cards', status: 'Blocked', variant: 'destructive' as const, owner: 'API', route: '/metrics', updated: 'Yesterday' },
  { name: 'Components gallery', status: 'Ready', variant: 'default' as const, owner: 'Frontend', route: '/components', updated: '15m ago' },
  { name: 'Settings view', status: 'Draft', variant: 'secondary' as const, owner: 'Platform', route: '/settings', updated: '1d ago' },
]

const visible = computed(() => {
  const q = search.value.trim().toLowerCase()
  return rows.filter((row) => {
    const matchesStatus = status.value === 'all' || row.status.toLowerCase() === status.value
    const matchesQuery = !q || [row.name, row.owner, row.route, row.status].some(v => v.toLowerCase().includes(q))
    return matchesStatus && matchesQuery
  })
})
</script>

<template>
  <div class="grid gap-4">
    <div class="grid gap-3 lg:grid-cols-[1fr_auto]">
      <InputGroup>
        <InputGroupAddon>
          <Search class="size-4" />
        </InputGroupAddon>
        <InputGroupInput v-model="search" placeholder="Search resources..." />
      </InputGroup>

      <Select v-model="status">
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
    </div>

    <div class="overflow-hidden rounded-lg border">
      <div class="flex items-center justify-between gap-4 border-b bg-muted/40 px-4 py-3">
        <div class="grid gap-0.5">
          <p class="text-sm font-medium leading-none">Resource index</p>
          <p class="text-sm text-muted-foreground">An admin listing with filters, ownership, and route context.</p>
        </div>
        <Badge variant="outline">{{ visible.length }} visible</Badge>
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
          <TableRow v-for="row in visible" :key="row.name">
            <TableCell class="font-medium">{{ row.name }}</TableCell>
            <TableCell>
              <Badge :variant="row.variant">{{ row.status }}</Badge>
            </TableCell>
            <TableCell>{{ row.owner }}</TableCell>
            <TableCell class="font-mono text-xs text-muted-foreground">{{ row.route }}</TableCell>
            <TableCell class="text-right text-muted-foreground">{{ row.updated }}</TableCell>
          </TableRow>
          <TableRow v-if="!visible.length">
            <TableCell colspan="5" class="py-8 text-center text-sm text-muted-foreground">
              No resources match that search.
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from 'vue'
import { Pencil, Play, Trash2 } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { displayTargetFromFields } from '@/lib/monitor-target'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

type Monitor = {
  id?: string
  name?: string
  type?: string
  target?: string
  target_url?: string
  target_host?: string
  target_port?: number
  target_record_type?: string
  target_keyword?: string
  target_expected?: string
  target_container?: string
  target_docker_host?: string
  target_push_token?: string
  interval_seconds?: number
  enabled?: boolean
  uptime_24h?: number
  last_status?: string
}

const props = defineProps<{
  data: Monitor[]
  selectedId?: string
  loading?: boolean
}>()

const emit = defineEmits<{
  select: [id: string]
  checkNow: [id: string]
  create: []
  edit: [id: string]
  remove: [id: string]
}>()

const search = ref('')
const typeFilter = ref<'all' | 'http' | 'tcp' | 'ping'>('all')
const stateFilter = ref<'all' | 'up' | 'down' | 'pending'>('all')

function monitorDisplayTarget(monitor: Monitor): string {
  return displayTargetFromFields(monitor.type || '', {
    target: monitor.target,
    target_url: monitor.target_url,
    target_host: monitor.target_host,
    target_port: monitor.target_port,
    target_record_type: monitor.target_record_type,
    target_keyword: monitor.target_keyword,
    target_expected: monitor.target_expected,
    target_container: monitor.target_container,
    target_docker_host: monitor.target_docker_host,
    target_push_token: monitor.target_push_token,
  })
}

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return props.data.filter((m) => {
    if (q) {
      const haystack = `${m.name || ''} ${monitorDisplayTarget(m) || ''}`.toLowerCase()
      if (!haystack.includes(q)) return false
    }
    if (typeFilter.value !== 'all' && (m.type || '').toLowerCase() !== typeFilter.value) return false
    const state = (m.last_status || (m.enabled ? 'up' : 'down')).toLowerCase()
    if (stateFilter.value === 'up' && state !== 'up') return false
    if (stateFilter.value === 'down' && state !== 'down') return false
    if (stateFilter.value === 'pending' && state !== 'pending') return false
    return true
  })
})
</script>

<template>
  <div class="px-4 lg:px-6">
    <div class="mb-3 flex flex-wrap items-center gap-2">
      <Input v-model="search" placeholder="Search monitors..." class="w-60" />
      <Button variant="outline" size="sm" @click="typeFilter = 'all'">All</Button>
      <Button variant="outline" size="sm" @click="typeFilter = 'http'">HTTP</Button>
      <Button variant="outline" size="sm" @click="typeFilter = 'tcp'">TCP</Button>
      <Button variant="outline" size="sm" @click="typeFilter = 'ping'">Ping</Button>
      <Button variant="outline" size="sm" @click="stateFilter = 'all'">Any</Button>
      <Button variant="outline" size="sm" @click="stateFilter = 'up'">Up</Button>
      <Button variant="outline" size="sm" @click="stateFilter = 'down'">Down</Button>
      <Button variant="outline" size="sm" @click="stateFilter = 'pending'">Pending</Button>
      <div class="ml-auto">
        <Button size="sm" @click="emit('create')">New Monitor</Button>
      </div>
    </div>
    <div class="overflow-hidden rounded-lg border border-border bg-card">
      <div class="border-b border-border px-4 py-3">
        <h2 class="text-sm font-semibold">Monitors</h2>
      </div>
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Type</TableHead>
            <TableHead>Target</TableHead>
            <TableHead>Interval</TableHead>
            <TableHead>Uptime (24h)</TableHead>
            <TableHead>Status</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          <TableRow v-if="loading">
            <TableCell colspan="6" class="h-20 text-center text-muted-foreground">
              Loading monitors...
            </TableCell>
          </TableRow>
          <TableRow v-else-if="!filtered.length">
            <TableCell colspan="6" class="h-20 text-center text-muted-foreground">
              No monitors found. Run migrations and seed data.
            </TableCell>
          </TableRow>
          <TableRow
            v-for="m in filtered"
            :key="m.id || m.name"
            class="cursor-pointer"
            :class="props.selectedId === m.id ? 'bg-muted/40' : ''"
            @click="m.id && emit('select', m.id)"
          >
            <TableCell class="font-medium">{{ m.name || '-' }}</TableCell>
            <TableCell class="text-muted-foreground">{{ m.type || '-' }}</TableCell>
            <TableCell class="text-muted-foreground">{{ monitorDisplayTarget(m) || '-' }}</TableCell>
            <TableCell class="text-muted-foreground">{{ m.interval_seconds || 0 }}s</TableCell>
            <TableCell>
              <Badge
                variant="outline"
                :class="
                  (m.uptime_24h || 0) >= 99
                    ? 'text-emerald-400 border-emerald-500/40'
                    : (m.uptime_24h || 0) >= 95
                    ? 'text-amber-300 border-amber-500/40'
                    : 'text-rose-400 border-rose-500/40'
                "
              >
                {{ Number(m.uptime_24h || 0).toFixed(2) }}%
              </Badge>
            </TableCell>
            <TableCell>
              <div class="flex items-center gap-2">
                <Badge
                  :variant="(m.last_status || '').toLowerCase() === 'up' ? 'default' : 'outline'"
                  :class="
                    (m.last_status || '').toLowerCase() === 'pending'
                      ? 'border-yellow-500/40 text-yellow-300'
                      : (m.last_status || '').toLowerCase() === 'down'
                      ? 'border-rose-500/40 text-rose-400'
                      : ''
                  "
                >
                  {{ (m.last_status || (m.enabled ? 'up' : 'down')).toLowerCase() }}
                </Badge>
                <Button
                  v-if="m.id"
                  type="button"
                  variant="outline"
                  size="sm"
                  class="h-7 px-2"
                  @click.stop="emit('checkNow', m.id)"
                >
                  <Play class="size-3.5" />
                </Button>
                <Button
                  v-if="m.id"
                  type="button"
                  variant="outline"
                  size="sm"
                  class="h-7 px-2"
                  @click.stop="emit('edit', m.id)"
                >
                  <Pencil class="size-3.5" />
                  Edit
                </Button>
                <Button
                  v-if="m.id"
                  type="button"
                  variant="destructive"
                  size="sm"
                  class="h-7 px-2"
                  @click.stop="emit('remove', m.id)"
                >
                  <Trash2 class="size-3.5" />
                  Delete
                </Button>
              </div>
            </TableCell>
          </TableRow>
        </TableBody>
      </Table>
    </div>
  </div>
</template>

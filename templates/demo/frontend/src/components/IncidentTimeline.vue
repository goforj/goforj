<script setup lang="ts">
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'

type Incident = {
  id?: string
  monitor_id?: string
  monitor_name?: string
  opened_at?: string
  resolved_at?: string | null
  summary?: string
}

defineProps<{
  incidents: Incident[]
  state: 'all' | 'open' | 'resolved'
}>()
const emit = defineEmits<{
  stateChange: [state: 'all' | 'open' | 'resolved']
}>()

function formatDuration(openedAt?: string, resolvedAt?: string | null) {
  if (!openedAt) return '-'
  const opened = new Date(openedAt)
  const end = resolvedAt ? new Date(resolvedAt) : new Date()
  const ms = Math.max(0, end.getTime() - opened.getTime())
  const mins = Math.floor(ms / 60000)
  if (mins < 60) return `${mins}m`
  const hours = Math.floor(mins / 60)
  const rem = mins % 60
  return `${hours}h ${rem}m`
}
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>Incidents</CardTitle>
      <CardDescription>Recent uptime events and recoveries.</CardDescription>
      <div class="mt-2 flex gap-2">
        <Badge variant="outline" class="cursor-pointer" @click="emit('stateChange', 'all')">All</Badge>
        <Badge variant="outline" class="cursor-pointer" @click="emit('stateChange', 'open')">Open</Badge>
        <Badge variant="outline" class="cursor-pointer" @click="emit('stateChange', 'resolved')">Resolved</Badge>
      </div>
    </CardHeader>
    <CardContent>
      <div class="space-y-2">
        <div
          v-for="incident in incidents.slice(0, 8)"
          :key="incident.id"
          class="rounded-md border border-border p-3"
        >
          <div class="flex items-center justify-between gap-3">
            <p class="text-sm font-medium">{{ incident.monitor_name || incident.monitor_id }}</p>
            <Badge :variant="incident.resolved_at ? 'secondary' : 'destructive'">
              {{ incident.resolved_at ? 'resolved' : 'open' }}
            </Badge>
          </div>
          <p class="mt-1 text-sm text-muted-foreground">{{ incident.summary }}</p>
          <p class="mt-1 text-xs text-muted-foreground">
            opened: {{ incident.opened_at }}<span v-if="incident.resolved_at"> • resolved: {{ incident.resolved_at }}</span> • duration:
            {{ formatDuration(incident.opened_at, incident.resolved_at) }}
          </p>
        </div>
        <p v-if="!incidents.length" class="text-sm text-muted-foreground">No incidents yet.</p>
      </div>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

const payload = ref<any>(null)
const grouped = computed<Record<string, any[]>>(() => payload.value?.grouped ?? {})
const latestIncident = computed<any[]>(() => payload.value?.latestIncident ?? [])
const lastUpdated = computed<string>(() => payload.value?.lastUpdated ?? '')

async function load() {
  const res = await fetch('/api/v1/monitoring/status-page')
  if (!res.ok) return
  payload.value = await res.json()
}

onMounted(load)
</script>

<template>
  <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
    <div class="px-4 lg:px-6">
      <Card>
        <CardHeader>
          <CardTitle>Public Status Page</CardTitle>
          <CardDescription>
            Service health rollup for enabled monitors. Last updated: {{ lastUpdated || 'n/a' }}
          </CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <Button as="a" href="/status" target="_blank" rel="noopener noreferrer" variant="outline" size="sm">
            Open public status page
          </Button>
          <div class="space-y-2">
            <p class="text-sm font-medium">Open incidents</p>
            <div
              v-for="incident in latestIncident"
              :key="`${incident.monitor_id}-${incident.opened_at}`"
              class="rounded-md border border-border p-3"
            >
              <p class="text-sm font-medium">{{ incident.monitor_name }}</p>
              <p class="text-xs text-muted-foreground">{{ incident.summary }}</p>
            </div>
            <p v-if="!latestIncident.length" class="text-xs text-muted-foreground">No open incidents.</p>
          </div>
          <div v-for="(services, kind) in grouped" :key="kind" class="space-y-2">
            <p class="text-sm font-medium uppercase tracking-wide text-muted-foreground">{{ kind }}</p>
            <div
              v-for="service in services"
              :key="service.id"
              class="flex items-center justify-between rounded-md border border-border p-3"
            >
              <div>
                <p class="font-medium">{{ service.name }}</p>
                <p class="text-xs text-muted-foreground">{{ service.target }}</p>
              </div>
              <Badge
                :variant="(service.last_status || '').toLowerCase() === 'up' ? 'default' : 'outline'"
                :class="
                  (service.last_status || '').toLowerCase() === 'pending'
                    ? 'border-yellow-500/40 text-yellow-300'
                    : (service.last_status || '').toLowerCase() === 'down'
                    ? 'border-rose-500/40 text-rose-400'
                    : ''
                "
              >
                {{ (service.last_status || 'unknown').toLowerCase() }}
              </Badge>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  </div>
</template>

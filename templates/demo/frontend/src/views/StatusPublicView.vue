<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

const { t } = useI18n()
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

function serviceStatusLabel(status?: string): string {
  const value = String(status || 'unknown').toLowerCase()
  if (value === 'up') return t('status.up')
  if (value === 'down') return t('status.down')
  if (value === 'maintenance') return t('monitoring.maintenance')
  if (value === 'pending') return t('status.pending')
  return t('status.unknown')
}
</script>

<template>
  <main class="mx-auto flex min-h-screen w-full max-w-5xl flex-col gap-4 px-4 py-8">
    <Card>
      <CardHeader>
        <CardTitle>{{ t('publicStatus.title') }}</CardTitle>
        <CardDescription>
          {{ t('publicStatus.lastUpdated', { value: lastUpdated || t('common.na') }) }}
        </CardDescription>
      </CardHeader>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle>{{ t('statusPages.openIncidents') }}</CardTitle>
      </CardHeader>
      <CardContent class="space-y-2">
        <div
          v-for="incident in latestIncident"
          :key="`${incident.monitor_id}-${incident.opened_at}`"
          class="rounded-md border border-border p-3"
        >
          <p class="text-sm font-medium">{{ incident.monitor_name }}</p>
          <p class="text-xs text-muted-foreground">{{ incident.summary }}</p>
        </div>
        <p v-if="!latestIncident.length" class="text-sm text-muted-foreground">{{ t('statusPages.noOpenIncidents') }}</p>
      </CardContent>
    </Card>

    <Card>
      <CardHeader>
        <CardTitle>{{ t('publicStatus.services') }}</CardTitle>
      </CardHeader>
      <CardContent class="space-y-4">
        <div v-for="(services, kind) in grouped" :key="kind" class="space-y-2">
          <h3 class="text-sm font-semibold uppercase tracking-wide text-muted-foreground">{{ kind }}</h3>
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
                  : (service.last_status || '').toLowerCase() === 'maintenance'
                  ? 'border-amber-500/40 text-amber-400'
                  : (service.last_status || '').toLowerCase() === 'down'
                  ? 'border-rose-500/40 text-rose-400'
                  : ''
              "
            >
              {{ serviceStatusLabel(service.last_status) }}
            </Badge>
          </div>
        </div>
      </CardContent>
    </Card>
  </main>
</template>

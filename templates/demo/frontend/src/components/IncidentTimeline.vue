<script setup lang="ts">
import { ref } from 'vue'
import { AlertTriangle, CheckCircle2 } from 'lucide-vue-next'
import { useI18n } from 'vue-i18n'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Badge } from '@/components/ui/badge'
import { monitorSupportsFavicon, monitorTypeIcon } from '@/lib/monitor-icons'
import { notificationProviderLabel } from '@/lib/notification-providers'

type Incident = {
  id?: string
  monitor_id?: string
  monitor_name?: string
  monitor_type?: string
  opened_at?: string
  resolved_at?: string | null
  summary?: string
  channels?: IncidentChannel[]
}

type IncidentChannel = {
  channel_id?: number
  channel_name?: string
  provider?: string
  delivery?: string
  created_at?: string
}

defineProps<{
  incidents: Incident[]
  state: 'all' | 'open' | 'resolved'
}>()
const emit = defineEmits<{
  stateChange: [state: 'all' | 'open' | 'resolved']
}>()
const { t } = useI18n()
const faviconFailedByID = ref<Record<string, boolean>>({})

function incidentFaviconSrc(incident: Incident): string {
  const id = String(incident.monitor_id || '')
  if (!id || faviconFailedByID.value[id]) return ''
  const monitorType = String(incident.monitor_type || '').trim()
  if (monitorType && !monitorSupportsFavicon(monitorType)) return ''
  return `/api/v1/monitoring/monitors/${id}/favicon`
}

function markIncidentFaviconFailed(incident: Incident) {
  const id = String(incident.monitor_id || '')
  if (!id) return
  faviconFailedByID.value = { ...faviconFailedByID.value, [id]: true }
}

function incidentTypeIcon(incident: Incident) {
  return monitorTypeIcon(incident.monitor_type)
}

function deliveryTone(delivery?: string): 'default' | 'secondary' | 'destructive' | 'outline' {
  const status = String(delivery || '').toLowerCase()
  if (status === 'delivered' || status === 'sent' || status === 'success') return 'secondary'
  if (status === 'failed' || status === 'error') return 'destructive'
  return 'outline'
}

function channelLabel(channel: IncidentChannel): string {
  const name = String(channel.channel_name || '').trim()
  if (name) return name
  return notificationProviderLabel(String(channel.provider || 'channel'))
}

function formatDuration(openedAt?: string, resolvedAt?: string | null) {
  if (!openedAt) return t('common.notAvailable')
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
      <CardTitle>{{ t('routes.incidents') }}</CardTitle>
      <CardDescription>{{ t('incidents.recentEvents') }}</CardDescription>
      <div class="mt-2 flex gap-2">
        <Badge variant="outline" class="cursor-pointer" @click="emit('stateChange', 'all')">{{ t('common.all') }}</Badge>
        <Badge variant="outline" class="cursor-pointer" @click="emit('stateChange', 'open')">{{ t('common.open') }}</Badge>
        <Badge variant="outline" class="cursor-pointer" @click="emit('stateChange', 'resolved')">{{ t('common.resolved') }}</Badge>
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
            <p class="flex min-w-0 items-center gap-2 text-sm font-medium">
              <img
                v-if="incidentFaviconSrc(incident)"
                :src="incidentFaviconSrc(incident)"
                :alt="`${incident.monitor_name || incident.monitor_id || t('nav.monitors')} favicon`"
                class="size-4 rounded-sm object-contain"
                loading="lazy"
                @error="markIncidentFaviconFailed(incident)"
              />
              <component v-else :is="incidentTypeIcon(incident)" class="size-4 text-muted-foreground" />
              <span class="truncate">{{ incident.monitor_name || incident.monitor_id }}</span>
            </p>
            <Badge :variant="incident.resolved_at ? 'secondary' : 'destructive'">
              <CheckCircle2 v-if="incident.resolved_at" class="size-3.5" />
              <AlertTriangle v-else class="size-3.5" />
              {{ incident.resolved_at ? t('common.resolved') : t('common.open') }}
            </Badge>
          </div>
          <p class="mt-1 text-sm text-muted-foreground">{{ incident.summary }}</p>
          <div v-if="incident.channels?.length" class="mt-2 flex flex-wrap items-center gap-1.5">
            <span class="text-xs text-muted-foreground">{{ t('incidents.notified') }}</span>
            <Badge
              v-for="(channel, idx) in incident.channels"
              :key="`${incident.id || 'incident'}-channel-${idx}`"
              :variant="deliveryTone(channel.delivery)"
              class="gap-1"
            >
              <span>{{ channelLabel(channel) }}</span>
              <span class="text-[10px] lowercase opacity-80">({{ channel.delivery || t('common.sent') }})</span>
            </Badge>
          </div>
          <p class="mt-1 text-xs text-muted-foreground">
            {{ t('common.opened') }}: {{ incident.opened_at }}<span v-if="incident.resolved_at"> • {{ t('common.resolved') }}: {{ incident.resolved_at }}</span> • {{ t('common.duration') }}:
            {{ formatDuration(incident.opened_at, incident.resolved_at) }}
          </p>
        </div>
        <p v-if="!incidents.length" class="text-sm text-muted-foreground">{{ t('incidents.noneYet') }}</p>
      </div>
    </CardContent>
  </Card>
</template>

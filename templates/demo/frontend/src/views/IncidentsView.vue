<script setup lang="ts">
import { onMounted, ref } from 'vue'
import IncidentTimeline from '@/components/IncidentTimeline.vue'
import { apiFetch } from '@/lib/auth'
import { fetchMonitors } from '@/lib/monitoring-requests'

const state = ref<'all' | 'open' | 'resolved'>('all')
const incidents = ref<any[]>([])

type MonitorListRow = {
  id?: string
  type?: string
  monitor_type?: string
}

type IncidentChannel = {
  channel_id?: number
  channel_name?: string
  provider?: string
  delivery?: string
  created_at?: string
}

async function load() {
  const [incidentResp, monitorsPayload] = await Promise.all([
    apiFetch(`/api/v1/monitoring/incidents?state=${state.value}`),
    fetchMonitors().catch(() => ({} as any)),
  ])
  if (!incidentResp.ok) return
  const incidentPayload = await incidentResp.json()
  const rawIncidents = Array.isArray(incidentPayload.incidents) ? incidentPayload.incidents : []
  const incidentChannels = incidentPayload?.incident_channels && typeof incidentPayload.incident_channels === 'object'
    ? (incidentPayload.incident_channels as Record<string, IncidentChannel[]>)
    : {}
  const monitorRows = Array.isArray(monitorsPayload?.monitors) ? (monitorsPayload.monitors as MonitorListRow[]) : []
  const monitorTypeByID = monitorRows.reduce<Record<string, string>>((acc, row) => {
    const id = String(row.id || '')
    const typ = String(row.type || row.monitor_type || '')
    if (id && typ) {
      acc[id] = typ
    }
    return acc
  }, {})
  incidents.value = rawIncidents.map((incident: any) => ({
    ...incident,
    monitor_type: incident?.monitor_type || monitorTypeByID[String(incident?.monitor_id || '')] || '',
    channels: Array.isArray(incidentChannels[String(incident?.id || '')])
      ? incidentChannels[String(incident?.id || '')]
      : [],
  }))
}

async function setState(next: 'all' | 'open' | 'resolved') {
  state.value = next
  await load()
}

onMounted(load)
</script>

<template>
  <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
    <div class="px-4 lg:px-6">
      <IncidentTimeline :incidents="incidents" :state="state" @state-change="setState" />
    </div>
  </div>
</template>

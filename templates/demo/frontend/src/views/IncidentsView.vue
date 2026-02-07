<script setup lang="ts">
import { onMounted, ref } from 'vue'
import IncidentTimeline from '@/components/IncidentTimeline.vue'

const state = ref<'all' | 'open' | 'resolved'>('all')
const incidents = ref<any[]>([])

async function load() {
  const res = await fetch(`/api/v1/monitoring/incidents?state=${state.value}`)
  if (!res.ok) return
  const payload = await res.json()
  incidents.value = Array.isArray(payload.incidents) ? payload.incidents : []
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

<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import MonitorDetailPanel from '@/components/MonitorDetailPanel.vue'
import { Input } from '@/components/ui/input'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'

const monitors = ref<any[]>([])
const selectedID = ref('')
const selected = ref<any | null>(null)
const checks = ref<any[]>([])
const range = ref<'1h'|'24h'|'7d'>('24h')
const { t } = useI18n()

async function loadMonitors() {
  const res = await fetch('/api/v1/monitoring/monitors')
  if (!res.ok) return
  const payload = await res.json()
  monitors.value = Array.isArray(payload.monitors) ? payload.monitors : []
  if (!selectedID.value && monitors.value.length > 0) {
    selectedID.value = monitors.value[0].id || ''
  }
}

async function loadChecks() {
  if (!selectedID.value) return
  const [detailRes, checksRes] = await Promise.all([
    fetch(`/api/v1/monitoring/monitors/${selectedID.value}`),
    fetch(`/api/v1/monitoring/monitors/${selectedID.value}/checks?range=${range.value}`),
  ])
  if (detailRes.ok) selected.value = (await detailRes.json()).monitor || null
  if (checksRes.ok) checks.value = (await checksRes.json()).checks || []
}

watch([selectedID, range], () => { void loadChecks() })
onMounted(async () => { await loadMonitors(); await loadChecks() })
</script>

<template>
  <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
    <div class="px-4 lg:px-6">
      <Card>
        <CardHeader>
          <CardTitle>{{ t('checkHistory.title') }}</CardTitle>
        </CardHeader>
        <CardContent class="flex flex-wrap gap-2">
          <Input v-model="selectedID" :placeholder="t('checkHistory.monitorIdPlaceholder')" class="w-72" />
          <Button variant="outline" size="sm" @click="range='1h'">{{ t('common.lastRange', { range: '1h' }) }}</Button>
          <Button variant="outline" size="sm" @click="range='24h'">{{ t('common.lastRange', { range: '24h' }) }}</Button>
          <Button variant="outline" size="sm" @click="range='7d'">{{ t('common.lastRange', { range: '7d' }) }}</Button>
        </CardContent>
      </Card>
    </div>
    <div class="px-4 lg:px-6">
      <MonitorDetailPanel :monitor="selected" :checks="checks" :check-range="range as any" />
    </div>
  </div>
</template>

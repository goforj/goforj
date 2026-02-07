<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ArrowLeft } from 'lucide-vue-next'
import MonitorEditor from '@/components/MonitorEditor.vue'
import { Button } from '@/components/ui/button'

const route = useRoute()
const router = useRouter()
const monitor = ref<any | null>(null)
const loading = ref(false)

const monitorID = computed(() => String(route.params.id || ''))
const isCreate = computed(() => route.path === '/monitors/new')

async function load() {
  if (isCreate.value || !monitorID.value) {
    monitor.value = null
    return
  }
  loading.value = true
  try {
    const res = await fetch(`/api/v1/monitoring/monitors/${monitorID.value}`)
    if (!res.ok) return
    const payload = await res.json()
    monitor.value = payload.monitor || null
  } finally {
    loading.value = false
  }
}

function onSaved(id: string) {
  const targetID = id || monitorID.value
  if (!targetID) {
    void router.push('/monitors')
    return
  }
  void router.push(`/monitors/${targetID}`)
}

function onDeleted(id: string) {
  if (id === monitorID.value) {
    void router.push('/monitors')
    return
  }
  void router.push(`/monitors/${monitorID.value}`)
}

onMounted(load)
watch(() => route.params.id, () => { void load() })
</script>

<template>
  <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
    <div class="px-4 lg:px-6">
      <div class="mb-3 flex items-center justify-between">
        <h1 class="text-lg font-semibold">{{ isCreate ? 'Create Monitor' : 'Edit Monitor' }}</h1>
        <Button
          variant="outline"
          size="sm"
          class="gap-1.5 px-2.5"
          @click="router.push(monitorID ? `/monitors/${monitorID}` : '/monitors')"
        >
          <ArrowLeft class="size-4" />
          Back
        </Button>
      </div>
      <p v-if="loading" class="text-sm text-muted-foreground">Loading monitor...</p>
      <MonitorEditor v-else :monitor="monitor" @saved="onSaved" @deleted="onDeleted" />
    </div>
  </div>
</template>

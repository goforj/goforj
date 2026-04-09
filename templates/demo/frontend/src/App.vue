<script setup lang="ts">
import { computed, onMounted, onUnmounted } from 'vue'
import { useRoute } from 'vue-router'
import AppSidebar from '@/components/AppSidebar.vue'
import SiteHeader from '@/components/SiteHeader.vue'
import { SidebarInset, SidebarProvider } from '@/components/ui/sidebar'
import { Toaster, toast } from 'vue-sonner'
import { subscribeMonitoringTransitionEvents } from '@/lib/monitoring-live'

const route = useRoute()
const isPublicShell = computed(() => route.meta?.publicShell === true)
let unsubscribeMonitoringToasts: (() => void) | null = null

onMounted(() => {
  if (unsubscribeMonitoringToasts) return
  unsubscribeMonitoringToasts = subscribeMonitoringTransitionEvents((event) => {
    if (route.meta?.publicShell === true) return
    const name = event.monitor_name || event.monitor_id || 'Monitor'
    const toastID = `monitor-transition:${event.monitor_id}:${event.type}`
    if (event.type === 'monitor.recovered') {
      toast.success(`${name} recovered`, {
        id: toastID,
        description: event.summary || 'Responding again.',
      })
      return
    }
    if (event.type === 'monitor.down') {
      toast.error(`${name} is down`, {
        id: toastID,
        description: event.summary || event.error_message || undefined,
      })
    }
  })
})

onUnmounted(() => {
  if (!unsubscribeMonitoringToasts) return
  unsubscribeMonitoringToasts()
  unsubscribeMonitoringToasts = null
})
</script>

<template>
  <RouterView v-if="isPublicShell" />
  <SidebarProvider
    v-else
    :style="{
      '--sidebar-width': 'calc(var(--spacing) * 72)',
      '--header-height': 'calc(var(--spacing) * 12)',
    }"
  >
    <AppSidebar />
    <SidebarInset>
      <SiteHeader />
      <div class="flex flex-1 flex-col">
        <div class="@container/main flex flex-1 flex-col gap-2">
          <RouterView />
        </div>
      </div>
    </SidebarInset>
  </SidebarProvider>
  <Toaster />
</template>

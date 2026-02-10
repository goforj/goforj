<script setup lang="ts">
import { onMounted, ref } from 'vue'
import { Loader2, Save, Trash2 } from 'lucide-vue-next'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import {
  clearMonitoringFaviconCache,
  fetchMonitoringSettings,
  updateMonitoringSettings,
} from '@/lib/monitoring-requests'

const faviconCacheTTLSeconds = ref(604800)
const loading = ref(true)
const saving = ref(false)
const clearingCache = ref(false)
const error = ref('')
const notice = ref('')

function clearMessages() {
  error.value = ''
  notice.value = ''
}

async function loadSettings() {
  loading.value = true
  clearMessages()
  try {
    const payload = await fetchMonitoringSettings()
    const raw = Number(payload?.settings?.favicon_cache_ttl_seconds ?? 604800)
    faviconCacheTTLSeconds.value = Number.isFinite(raw) && raw > 0 ? Math.floor(raw) : 604800
  } catch {
    error.value = 'Failed to load settings.'
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  clearMessages()
  const ttl = Math.floor(Number(faviconCacheTTLSeconds.value))
  if (!Number.isFinite(ttl) || ttl < 60 || ttl > 2592000) {
    error.value = 'Favicon cache TTL must be between 60 and 2592000 seconds.'
    return
  }
  saving.value = true
  try {
    await updateMonitoringSettings({ favicon_cache_ttl_seconds: ttl })
    notice.value = 'Settings saved.'
  } catch (err: any) {
    error.value = typeof err?.message === 'string' ? err.message : 'Failed to save settings.'
  } finally {
    saving.value = false
  }
}

async function clearFaviconCache() {
  clearMessages()
  clearingCache.value = true
  try {
    const payload = await clearMonitoringFaviconCache()
    const removed = Number(payload?.removed_files ?? 0)
    notice.value = `Favicon cache cleared (${removed} files removed).`
  } catch (err: any) {
    error.value = typeof err?.message === 'string' ? err.message : 'Failed to clear cache.'
  } finally {
    clearingCache.value = false
  }
}

onMounted(() => {
  void loadSettings()
})
</script>

<template>
  <div class="flex flex-col gap-4 py-4 md:gap-6 md:py-6">
    <div class="px-4 lg:px-6">
      <Card>
        <CardHeader>
          <CardTitle>Application settings</CardTitle>
          <CardDescription>Configure runtime behavior for monitoring and UI helpers.</CardDescription>
        </CardHeader>
        <CardContent class="space-y-6">
          <div class="grid gap-2 md:max-w-md">
            <Label for="favicon-cache-ttl">Favicon cache TTL (seconds)</Label>
            <Input
              id="favicon-cache-ttl"
              v-model.number="faviconCacheTTLSeconds"
              type="number"
              min="60"
              max="2592000"
              :disabled="loading || saving"
            />
            <p class="text-xs text-muted-foreground">
              Default is one week (604800). Range: 60 to 2592000.
            </p>
          </div>

          <div v-if="error" class="text-sm text-rose-400">{{ error }}</div>
          <div v-else-if="notice" class="text-sm text-emerald-400">{{ notice }}</div>

          <div class="flex flex-wrap gap-2">
            <Button type="button" class="gap-2" :disabled="loading || saving || clearingCache" @click="saveSettings">
              <Loader2 v-if="saving" class="size-4 animate-spin" />
              <Save v-else class="size-4" />
              Save settings
            </Button>
            <Button
              type="button"
              variant="outline"
              class="gap-2"
              :disabled="loading || saving || clearingCache"
              @click="clearFaviconCache"
            >
              <Loader2 v-if="clearingCache" class="size-4 animate-spin" />
              <Trash2 v-else class="size-4" />
              Clear favicon cache
            </Button>
          </div>
        </CardContent>
      </Card>
    </div>
  </div>
</template>

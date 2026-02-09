<script setup lang="ts">
import { reactive, watch } from 'vue'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'

type Monitor = {
  id?: string
  name?: string
  type?: string
  target?: string
  interval_seconds?: number
  timeout_ms?: number
  retry_attempts?: number
  retry_backoff_ms?: number
  schedule_jitter_ms?: number
  enabled?: boolean
}

const props = defineProps<{
  monitor: Monitor | null
}>()

const emit = defineEmits<{
  saved: [id: string]
  deleted: [id: string]
}>()

const form = reactive({
  name: '',
  type: 'http',
  target: '',
  interval_seconds: 60,
  timeout_ms: 5000,
  retry_attempts: 2,
  retry_backoff_ms: 250,
  schedule_jitter_ms: 150,
  enabled: true,
})
const errorMessage = reactive({ text: '' })
const fieldErrors = reactive<Record<string, string>>({})

watch(
  () => props.monitor,
  (m) => {
    errorMessage.text = ''
    Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key])
    if (!m) {
      form.name = ''
      form.type = 'http'
      form.target = ''
      form.interval_seconds = 60
      form.timeout_ms = 5000
      form.retry_attempts = 2
      form.retry_backoff_ms = 250
      form.schedule_jitter_ms = 150
      form.enabled = true
      return
    }
    form.name = m.name || ''
    form.type = m.type || 'http'
    form.target = m.target || ''
    form.interval_seconds = m.interval_seconds || 60
    form.timeout_ms = m.timeout_ms || 5000
    form.retry_attempts = m.retry_attempts ?? 2
    form.retry_backoff_ms = m.retry_backoff_ms ?? 250
    form.schedule_jitter_ms = m.schedule_jitter_ms ?? 150
    form.enabled = Boolean(m.enabled)
  },
  { immediate: true },
)

async function save() {
  errorMessage.text = ''
  Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key])
  const payload = {
    name: form.name,
    type: form.type,
    target: form.target,
    interval_seconds: Number(form.interval_seconds),
    timeout_ms: Number(form.timeout_ms),
    retry_attempts: Number(form.retry_attempts),
    retry_backoff_ms: Number(form.retry_backoff_ms),
    schedule_jitter_ms: Number(form.schedule_jitter_ms),
    enabled: Boolean(form.enabled),
  }
  const isUpdate = !!props.monitor?.id
  const url = isUpdate ? `/api/v1/monitoring/monitors/${props.monitor?.id}` : '/api/v1/monitoring/monitors'
  const method = isUpdate ? 'PUT' : 'POST'
  const resp = await fetch(url, {
    method,
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  if (!resp.ok) {
    try {
      const payload = await resp.json()
      errorMessage.text = payload.error || 'failed to save monitor'
      if (payload.fields && typeof payload.fields === 'object') {
        Object.assign(fieldErrors, payload.fields)
      }
    } catch {
      errorMessage.text = 'failed to save monitor'
    }
    return
  }
  let savedID = props.monitor?.id || ''
  try {
    const payload = await resp.json()
    savedID = payload.id || savedID
  } catch {
    // keep fallback id
  }
  emit('saved', savedID)
}

async function remove() {
  if (!props.monitor?.id) return
  if (!confirm('Delete this monitor?')) return
  const resp = await fetch(`/api/v1/monitoring/monitors/${props.monitor.id}`, { method: 'DELETE' })
  if (!resp.ok) {
    errorMessage.text = 'failed to delete monitor'
    return
  }
  emit('deleted', props.monitor.id)
}
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>{{ monitor?.id ? 'Edit Monitor' : 'Create Monitor' }}</CardTitle>
      <CardDescription>Configure endpoint checks and intervals.</CardDescription>
    </CardHeader>
    <CardContent class="space-y-3">
      <div class="grid gap-2">
        <Label>Name</Label>
        <Input v-model="form.name" placeholder="Google" />
        <p v-if="fieldErrors.name" class="text-xs text-destructive">{{ fieldErrors.name }}</p>
      </div>
      <div class="grid gap-2">
        <Label>Type</Label>
        <Select v-model="form.type">
          <SelectTrigger>
            <SelectValue placeholder="Select monitor type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="http">HTTP</SelectItem>
            <SelectItem value="http_keyword">HTTP Keyword</SelectItem>
            <SelectItem value="http_json_query">HTTP JSON Query</SelectItem>
            <SelectItem value="websocket">WebSocket</SelectItem>
            <SelectItem value="tcp">TCP</SelectItem>
            <SelectItem value="ping">Ping</SelectItem>
            <SelectItem value="dns">DNS</SelectItem>
            <SelectItem value="tls">TLS</SelectItem>
            <SelectItem value="push">Push</SelectItem>
          </SelectContent>
        </Select>
        <p v-if="fieldErrors.type" class="text-xs text-destructive">{{ fieldErrors.type }}</p>
      </div>
      <div class="grid gap-2">
        <Label>Target</Label>
        <Input v-model="form.target" placeholder="https://example.com" />
        <p class="text-xs text-muted-foreground">
          HTTP keyword: <code>https://example.com|expected text</code>. JSON query: <code>https://httpbin.org/json|slideshow.author|Yours Truly</code>. DNS: <code>host|A</code>. Push: <code>token-or-name</code>.
        </p>
        <p v-if="fieldErrors.target" class="text-xs text-destructive">{{ fieldErrors.target }}</p>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div class="grid gap-2">
          <Label>Interval (sec)</Label>
          <Input v-model="form.interval_seconds" type="number" min="5" />
          <p v-if="fieldErrors.interval_seconds" class="text-xs text-destructive">{{ fieldErrors.interval_seconds }}</p>
        </div>
        <div class="grid gap-2">
          <Label>Timeout (ms)</Label>
          <Input v-model="form.timeout_ms" type="number" min="500" />
          <p v-if="fieldErrors.timeout_ms" class="text-xs text-destructive">{{ fieldErrors.timeout_ms }}</p>
        </div>
      </div>
      <div class="grid grid-cols-3 gap-3">
        <div class="grid gap-2">
          <Label>Retry attempts</Label>
          <Input v-model="form.retry_attempts" type="number" min="0" />
          <p v-if="fieldErrors.retry_attempts" class="text-xs text-destructive">{{ fieldErrors.retry_attempts }}</p>
        </div>
        <div class="grid gap-2">
          <Label>Retry backoff (ms)</Label>
          <Input v-model="form.retry_backoff_ms" type="number" min="50" />
          <p v-if="fieldErrors.retry_backoff_ms" class="text-xs text-destructive">{{ fieldErrors.retry_backoff_ms }}</p>
        </div>
        <div class="grid gap-2">
          <Label>Schedule jitter (ms)</Label>
          <Input v-model="form.schedule_jitter_ms" type="number" min="0" />
          <p v-if="fieldErrors.schedule_jitter_ms" class="text-xs text-destructive">{{ fieldErrors.schedule_jitter_ms }}</p>
        </div>
      </div>
      <div class="flex items-center justify-between rounded-md border border-border p-2">
        <Label>Enabled</Label>
        <Checkbox :model-value="form.enabled" @update:model-value="(v) => (form.enabled = !!v)" />
      </div>
      <Button class="w-full" @click="save">
        {{ monitor?.id ? 'Save Monitor' : 'Create Monitor' }}
      </Button>
      <Button v-if="monitor?.id" variant="destructive" class="w-full" @click="remove">
        Delete Monitor
      </Button>
      <p v-if="errorMessage.text" class="text-sm text-destructive">
        {{ errorMessage.text }}
      </p>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { computed, reactive, watch } from 'vue'
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
import { canonicalTargetFromFields, normalizeTargetFields } from '@/lib/monitor-target'
import { MONITOR_TYPE_OPTIONS, monitorTypeOption } from '@/lib/monitor-icons'

type Monitor = {
  id?: string
  name?: string
  type?: string
  target?: string
  target_url?: string
  target_host?: string
  target_port?: number
  target_record_type?: string
  target_keyword?: string
  target_expected?: string
  target_container?: string
  target_docker_host?: string
  target_push_token?: string
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
  target_url: '',
  target_host: '',
  target_port: 0,
  target_record_type: '',
  target_keyword: '',
  target_expected: '',
  target_container: '',
  target_docker_host: '',
  target_push_token: '',
  interval_seconds: 60,
  timeout_ms: 5000,
  retry_attempts: 2,
  retry_backoff_ms: 250,
  schedule_jitter_ms: 150,
  enabled: true,
})
const errorMessage = reactive({ text: '' })
const fieldErrors = reactive<Record<string, string>>({})
const selectedTypeOption = computed(() => monitorTypeOption(form.type))

const derivedTarget = computed(() =>
  canonicalTargetFromFields(form.type, {
    target_url: form.target_url,
    target_host: form.target_host,
    target_port: Number(form.target_port),
    target_record_type: form.target_record_type,
    target_keyword: form.target_keyword,
    target_expected: form.target_expected,
    target_container: form.target_container,
    target_docker_host: form.target_docker_host,
    target_push_token: form.target_push_token,
  }),
)

watch(
  () => props.monitor,
  (m) => {
    errorMessage.text = ''
    Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key])
    if (!m) {
      form.name = ''
      form.type = 'http'
      form.target = ''
      form.target_url = ''
      form.target_host = ''
      form.target_port = 0
      form.target_record_type = ''
      form.target_keyword = ''
      form.target_expected = ''
      form.target_container = ''
      form.target_docker_host = ''
      form.target_push_token = ''
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
    const parsed = normalizeTargetFields(form.type, form.target)
    form.target_url = m.target_url || parsed.target_url || ''
    form.target_host = m.target_host || parsed.target_host || ''
    form.target_port = Number(m.target_port || parsed.target_port || 0)
    form.target_record_type = m.target_record_type || parsed.target_record_type || ''
    form.target_keyword = m.target_keyword || parsed.target_keyword || ''
    form.target_expected = m.target_expected || parsed.target_expected || ''
    form.target_container = m.target_container || parsed.target_container || ''
    form.target_docker_host = m.target_docker_host || parsed.target_docker_host || ''
    form.target_push_token = m.target_push_token || parsed.target_push_token || ''
    form.interval_seconds = m.interval_seconds || 60
    form.timeout_ms = m.timeout_ms || 5000
    form.retry_attempts = m.retry_attempts ?? 2
    form.retry_backoff_ms = m.retry_backoff_ms ?? 250
    form.schedule_jitter_ms = m.schedule_jitter_ms ?? 150
    form.enabled = Boolean(m.enabled)
  },
  { immediate: true },
)

watch(
  () => [form.type, form.target],
  ([type, target]) => {
    const parsed = normalizeTargetFields(String(type || ''), String(target || ''))
    form.target_url = parsed.target_url || ''
    form.target_host = parsed.target_host || ''
    form.target_port = Number(parsed.target_port || 0)
    form.target_record_type = parsed.target_record_type || ''
    form.target_keyword = parsed.target_keyword || ''
    form.target_expected = parsed.target_expected || ''
    form.target_container = parsed.target_container || ''
    form.target_docker_host = parsed.target_docker_host || ''
    form.target_push_token = parsed.target_push_token || ''
  },
)

async function save() {
  errorMessage.text = ''
  Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key])
  const payload = {
    name: form.name,
    type: form.type,
    target: derivedTarget.value || form.target,
    target_url: form.target_url,
    target_host: form.target_host,
    target_port: Number(form.target_port),
    target_record_type: form.target_record_type,
    target_keyword: form.target_keyword,
    target_expected: form.target_expected,
    target_container: form.target_container,
    target_docker_host: form.target_docker_host,
    target_push_token: form.target_push_token,
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
            <SelectValue placeholder="Select monitor type">
              <span class="inline-flex items-center gap-2">
                <component :is="selectedTypeOption.icon" class="size-4 text-muted-foreground" />
                <span>{{ selectedTypeOption.label }}</span>
              </span>
            </SelectValue>
          </SelectTrigger>
          <SelectContent>
            <SelectItem
              v-for="option in MONITOR_TYPE_OPTIONS"
              :key="option.value"
              :value="option.value"
            >
              <span class="inline-flex items-center gap-2">
                <component :is="option.icon" class="size-4 text-muted-foreground" />
                <span>{{ option.label }}</span>
              </span>
            </SelectItem>
          </SelectContent>
        </Select>
        <p v-if="fieldErrors.type" class="text-xs text-destructive">{{ fieldErrors.type }}</p>
      </div>
      <div class="grid gap-2">
        <Label>Target Fields</Label>

        <div v-if="form.type === 'http' || form.type === 'websocket' || form.type === 'http_keyword' || form.type === 'http_json_query'" class="grid gap-3">
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">Target URL</Label>
            <Input
              v-model="form.target_url"
              :placeholder="form.type === 'websocket' ? 'wss://echo.websocket.org' : 'https://example.com/health'"
            />
          </div>
          <div v-if="form.type === 'http_keyword'" class="grid gap-2">
            <Label class="text-xs text-muted-foreground">Keyword</Label>
            <Input v-model="form.target_keyword" placeholder="Expected body text" />
          </div>
          <div v-if="form.type === 'http_json_query'" class="grid grid-cols-2 gap-3">
            <div class="grid gap-2">
              <Label class="text-xs text-muted-foreground">JSON Path</Label>
              <Input v-model="form.target_keyword" placeholder="slideshow.author" />
            </div>
            <div class="grid gap-2">
              <Label class="text-xs text-muted-foreground">Expected</Label>
              <Input v-model="form.target_expected" placeholder="Yours Truly" />
            </div>
          </div>
        </div>

        <div v-else-if="form.type === 'tcp' || form.type === 'steam' || form.type === 'tls'" class="grid grid-cols-2 gap-3">
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">Host</Label>
            <Input v-model="form.target_host" placeholder="example.com" />
          </div>
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">Port</Label>
            <Input v-model="form.target_port" type="number" min="1" :placeholder="form.type === 'tls' ? '443' : '80'" />
          </div>
        </div>

        <div v-else-if="form.type === 'ping'" class="grid gap-2">
          <Label class="text-xs text-muted-foreground">Host</Label>
          <Input v-model="form.target_host" placeholder="example.com" />
        </div>

        <div v-else-if="form.type === 'dns'" class="grid grid-cols-2 gap-3">
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">Host</Label>
            <Input v-model="form.target_host" placeholder="example.com" />
          </div>
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">Record Type</Label>
            <Select v-model="form.target_record_type">
              <SelectTrigger>
                <SelectValue placeholder="A" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="A">A</SelectItem>
                <SelectItem value="AAAA">AAAA</SelectItem>
                <SelectItem value="CNAME">CNAME</SelectItem>
                <SelectItem value="MX">MX</SelectItem>
                <SelectItem value="NS">NS</SelectItem>
                <SelectItem value="TXT">TXT</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </div>

        <div v-else-if="form.type === 'docker'" class="grid grid-cols-2 gap-3">
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">Container</Label>
            <Input v-model="form.target_container" placeholder="nginx" />
          </div>
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">Docker Host (optional)</Label>
            <Input v-model="form.target_docker_host" placeholder="unix:///var/run/docker.sock" />
          </div>
        </div>

        <div v-else-if="form.type === 'push'" class="grid gap-2">
          <Label class="text-xs text-muted-foreground">Push Token</Label>
          <Input v-model="form.target_push_token" placeholder="token-or-name" />
        </div>

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

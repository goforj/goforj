<script setup lang="ts">
import { computed, onMounted, reactive, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import {
  siDiscord,
  siGooglechat,
  siGooglesheets,
  siGrafana,
  siJirasoftware,
  siLine,
  siNtfy,
  siOpsgenie,
  siPagerduty,
  siRocketdotchat,
  siSendgrid,
  siSlack,
  siSplunk,
  siTelegram,
  siTwilio,
} from 'simple-icons'
import { FileText, Mail, Webhook } from 'lucide-vue-next'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
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
import { toast } from 'vue-sonner'
import { Loader2, Save, Trash2 } from 'lucide-vue-next'
import { normalizeTargetFields } from '@/lib/monitor-target'
import { MONITOR_TYPE_OPTIONS, monitorTypeOption } from '@/lib/monitor-icons'
import { normalizeProviderID, notificationProviderLabel, type NotificationProvider } from '@/lib/notification-providers'
import { fetchNotificationChannels } from '@/lib/monitoring-requests'
import type { NotificationChannel } from '@/lib/monitoring-requests'

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
  notification_channel_ids?: number[]
  interval_seconds?: number
  timeout_ms?: number
  retry_attempts?: number
  retry_backoff_ms?: number
  schedule_jitter_ms?: number
  down_confirm_attempts?: number
  recovery_confirm_attempts?: number
  resend_interval_checks?: number
  enabled?: boolean
}

const props = defineProps<{
  monitor: Monitor | null
}>()
const { t } = useI18n()

const emit = defineEmits<{
  saved: [id: string]
  deleted: [id: string]
}>()

const form = reactive({
  name: '',
  type: 'http',
  target_url: '',
  target_host: '',
  target_port: 0,
  target_record_type: '',
  target_keyword: '',
  target_expected: '',
  target_container: '',
  target_docker_host: '',
  target_push_token: '',
  notification_channel_ids: [] as number[],
  interval_seconds: 60,
  timeout_ms: 5000,
  retry_attempts: 2,
  retry_backoff_ms: 250,
  schedule_jitter_ms: 150,
  down_confirm_attempts: 1,
  recovery_confirm_attempts: 1,
  resend_interval_checks: 0,
  enabled: true,
})
const errorMessage = reactive({ text: '' })
const fieldErrors = reactive<Record<string, string>>({})
const selectedTypeOption = computed(() => monitorTypeOption(form.type))
const notificationChannels = reactive<NotificationChannel[]>([])
const channelsLoading = reactive({ value: false })
const channelsError = reactive({ text: '' })
const saveLoading = reactive({ value: false })
const deleteLoading = reactive({ value: false })
const sortedNotificationChannels = computed(() =>
  [...notificationChannels].sort(
    (a, b) => Number(Boolean(b.is_enabled)) - Number(Boolean(a.is_enabled)) || a.name.localeCompare(b.name),
  ),
)

function providerLabel(provider: NotificationProvider | string): string {
  return notificationProviderLabel(provider)
}

function providerIcon(provider: NotificationProvider | string) {
  const normalized = normalizeProviderID(provider)
  if (normalized === 'log') return FileText
  if (normalized === 'email' || normalized === 'smtp') return Mail
  return Webhook
}

const providerBrandIcons: Record<string, { path: string }> = {
  discord: siDiscord,
  googlechat: siGooglechat,
  googlesheets: siGooglesheets,
  grafanaoncall: siGrafana,
  jiraservicemanagement: siJirasoftware,
  line: siLine,
  ntfy: siNtfy,
  opsgenie: siOpsgenie,
  pagerduty: siPagerduty,
  'rocket.chat': siRocketdotchat,
  sendgrid: siSendgrid,
  slack: siSlack,
  splunk: siSplunk,
  telegram: siTelegram,
  twilio: siTwilio,
}

function providerBrandIcon(provider: NotificationProvider | string) {
  return providerBrandIcons[normalizeProviderID(provider)] ?? null
}

watch(
  () => props.monitor,
  (m) => {
    errorMessage.text = ''
    Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key])
    if (!m) {
      form.name = ''
      form.type = 'http'
      form.target_url = ''
      form.target_host = ''
      form.target_port = 0
      form.target_record_type = ''
      form.target_keyword = ''
      form.target_expected = ''
      form.target_container = ''
      form.target_docker_host = ''
      form.target_push_token = ''
      form.notification_channel_ids = []
      form.interval_seconds = 60
      form.timeout_ms = 5000
      form.retry_attempts = 2
      form.retry_backoff_ms = 250
      form.schedule_jitter_ms = 150
      form.down_confirm_attempts = 1
      form.recovery_confirm_attempts = 1
      form.resend_interval_checks = 0
      form.enabled = true
      return
    }
    form.name = m.name || ''
    form.type = m.type || 'http'
    const parsed = normalizeTargetFields(form.type, m.target || '')
    form.target_url = m.target_url || parsed.target_url || ''
    form.target_host = m.target_host || parsed.target_host || ''
    form.target_port = Number(m.target_port || parsed.target_port || 0)
    form.target_record_type = m.target_record_type || parsed.target_record_type || ''
    form.target_keyword = m.target_keyword || parsed.target_keyword || ''
    form.target_expected = m.target_expected || parsed.target_expected || ''
    form.target_container = m.target_container || parsed.target_container || ''
    form.target_docker_host = m.target_docker_host || parsed.target_docker_host || ''
    form.target_push_token = m.target_push_token || parsed.target_push_token || ''
    form.notification_channel_ids = Array.isArray(m.notification_channel_ids)
      ? m.notification_channel_ids.map((id) => Number(id)).filter((id) => Number.isFinite(id) && id > 0)
      : []
    form.interval_seconds = m.interval_seconds || 60
    form.timeout_ms = m.timeout_ms || 5000
    form.retry_attempts = m.retry_attempts ?? 2
    form.retry_backoff_ms = m.retry_backoff_ms ?? 250
    form.schedule_jitter_ms = m.schedule_jitter_ms ?? 150
    form.down_confirm_attempts = m.down_confirm_attempts ?? 1
    form.recovery_confirm_attempts = m.recovery_confirm_attempts ?? 1
    form.resend_interval_checks = m.resend_interval_checks ?? 0
    form.enabled = Boolean(m.enabled)
  },
  { immediate: true },
)

async function save() {
  if (saveLoading.value) return
  saveLoading.value = true
  errorMessage.text = ''
  Object.keys(fieldErrors).forEach((key) => delete fieldErrors[key])
  const payload = {
    name: form.name,
    type: form.type,
    target: '',
    target_url: form.target_url,
    target_host: form.target_host,
    target_port: Number(form.target_port),
    target_record_type: form.target_record_type,
    target_keyword: form.target_keyword,
    target_expected: form.target_expected,
    target_container: form.target_container,
    target_docker_host: form.target_docker_host,
    target_push_token: form.target_push_token,
    notification_channel_ids: form.notification_channel_ids.map((id) => Number(id)).filter((id) => Number.isFinite(id) && id > 0),
    interval_seconds: Number(form.interval_seconds),
    timeout_ms: Number(form.timeout_ms),
    retry_attempts: Number(form.retry_attempts),
    retry_backoff_ms: Number(form.retry_backoff_ms),
    schedule_jitter_ms: Number(form.schedule_jitter_ms),
    down_confirm_attempts: Number(form.down_confirm_attempts),
    recovery_confirm_attempts: Number(form.recovery_confirm_attempts),
    resend_interval_checks: Number(form.resend_interval_checks),
    enabled: Boolean(form.enabled),
  }
  const isUpdate = !!props.monitor?.id
  const url = isUpdate ? `/api/v1/monitoring/monitors/${props.monitor?.id}` : '/api/v1/monitoring/monitors'
  const method = isUpdate ? 'PUT' : 'POST'
  try {
    const resp = await fetch(url, {
      method,
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(payload),
    })
    if (!resp.ok) {
      try {
        const payload = await resp.json()
        errorMessage.text = payload.error || t('monitorEditor.failedSave')
        if (payload.fields && typeof payload.fields === 'object') {
          Object.assign(fieldErrors, payload.fields)
        }
      } catch {
        errorMessage.text = t('monitorEditor.failedSave')
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
    if (isUpdate) {
      toast.success(t('monitorEditor.saved'))
    }
    emit('saved', savedID)
  } catch {
    errorMessage.text = t('monitorEditor.failedSave')
  } finally {
    saveLoading.value = false
  }
}

async function remove() {
  if (!props.monitor?.id) return
  if (deleteLoading.value) return
  if (!confirm(t('monitoring.confirmDeleteMonitor'))) return
  deleteLoading.value = true
  try {
    const resp = await fetch(`/api/v1/monitoring/monitors/${props.monitor.id}`, { method: 'DELETE' })
    if (!resp.ok) {
      errorMessage.text = t('monitorEditor.failedDelete')
      return
    }
    toast.success(t('monitorEditor.deleted'))
    emit('deleted', props.monitor.id)
  } catch {
    errorMessage.text = t('monitorEditor.failedDelete')
  } finally {
    deleteLoading.value = false
  }
}

function toggleNotificationChannel(id: number, checked: boolean) {
  if (!Number.isFinite(id) || id <= 0) return
  const next = new Set(form.notification_channel_ids)
  if (checked) {
    next.add(id)
  } else {
    next.delete(id)
  }
  form.notification_channel_ids = Array.from(next).sort((a, b) => a - b)
}

async function loadNotificationChannels() {
  channelsLoading.value = true
  channelsError.text = ''
  try {
    const payload = await fetchNotificationChannels()
    const rows = Array.isArray(payload?.channels) ? payload.channels : []
    notificationChannels.splice(0, notificationChannels.length, ...(rows as NotificationChannel[]))
  } catch (err: any) {
    channelsError.text = typeof err?.message === 'string' ? err.message : t('settings.channels.loadFailed')
  } finally {
    channelsLoading.value = false
  }
}

onMounted(() => {
  void loadNotificationChannels()
})
</script>

<template>
  <Card>
    <CardHeader>
      <CardTitle>{{ monitor?.id ? t('routes.editMonitor') : t('routes.newMonitor') }}</CardTitle>
      <CardDescription>{{ t('monitorEditor.description') }}</CardDescription>
    </CardHeader>
    <CardContent class="space-y-3">
      <div class="grid gap-2">
        <Label>{{ t('monitoring.name') }}</Label>
        <Input v-model="form.name" :placeholder="t('monitorEditor.namePlaceholder')" />
        <p v-if="fieldErrors.name" class="text-xs text-destructive">{{ fieldErrors.name }}</p>
      </div>
      <div class="grid gap-2">
        <Label>{{ t('monitoring.type') }}</Label>
        <Select v-model="form.type">
          <SelectTrigger>
            <SelectValue :placeholder="t('monitorEditor.selectMonitorType')">
              <span class="inline-flex items-center gap-2">
                <component :is="selectedTypeOption.icon" class="size-4 text-muted-foreground" />
                <span>{{ t(selectedTypeOption.labelKey) }}</span>
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
                <span>{{ t(option.labelKey) }}</span>
              </span>
            </SelectItem>
          </SelectContent>
        </Select>
        <p v-if="fieldErrors.type" class="text-xs text-destructive">{{ fieldErrors.type }}</p>
      </div>
      <div class="grid gap-2">
        <Label>{{ t('monitorEditor.targetFields') }}</Label>

        <div v-if="form.type === 'http' || form.type === 'websocket' || form.type === 'http_keyword' || form.type === 'http_json_query'" class="grid gap-3">
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.url') }}</Label>
            <Input
              v-model="form.target_url"
              :placeholder="form.type === 'websocket' ? t('monitorEditor.websocketUrlPlaceholder') : t('monitorEditor.httpUrlPlaceholder')"
            />
          </div>
          <div v-if="form.type === 'http_keyword'" class="grid gap-2">
            <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.keyword') }}</Label>
            <Input v-model="form.target_keyword" :placeholder="t('monitorEditor.expectedBodyText')" />
          </div>
          <div v-if="form.type === 'http_json_query'" class="grid grid-cols-2 gap-3">
            <div class="grid gap-2">
              <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.jsonPath') }}</Label>
              <Input v-model="form.target_keyword" :placeholder="t('monitorEditor.jsonPathPlaceholder')" />
            </div>
            <div class="grid gap-2">
              <Label class="text-xs text-muted-foreground">{{ t('monitorEditor.expected') }}</Label>
              <Input v-model="form.target_expected" :placeholder="t('monitorEditor.expectedValuePlaceholder')" />
            </div>
          </div>
        </div>

        <div v-else-if="form.type === 'tcp' || form.type === 'steam' || form.type === 'tls'" class="grid grid-cols-2 gap-3">
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.host') }}</Label>
            <Input v-model="form.target_host" :placeholder="t('monitorEditor.hostPlaceholder')" />
          </div>
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.port') }}</Label>
            <Input v-model="form.target_port" type="number" min="1" :placeholder="form.type === 'tls' ? t('monitorEditor.tlsPortPlaceholder') : t('monitorEditor.defaultPortPlaceholder')" />
          </div>
        </div>

        <div v-else-if="form.type === 'ping'" class="grid gap-2">
          <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.host') }}</Label>
          <Input v-model="form.target_host" :placeholder="t('monitorEditor.hostPlaceholder')" />
        </div>

        <div v-else-if="form.type === 'dns'" class="grid grid-cols-2 gap-3">
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.host') }}</Label>
            <Input v-model="form.target_host" :placeholder="t('monitorEditor.hostPlaceholder')" />
          </div>
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.recordType') }}</Label>
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
            <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.container') }}</Label>
            <Input v-model="form.target_container" :placeholder="t('monitorEditor.containerPlaceholder')" />
          </div>
          <div class="grid gap-2">
            <Label class="text-xs text-muted-foreground">{{ t('monitorEditor.dockerHostOptional') }}</Label>
            <Input v-model="form.target_docker_host" :placeholder="t('monitorEditor.dockerHostPlaceholder')" />
          </div>
        </div>

        <div v-else-if="form.type === 'push'" class="grid gap-2">
          <Label class="text-xs text-muted-foreground">{{ t('monitorDetail.pushToken') }}</Label>
          <Input v-model="form.target_push_token" :placeholder="t('monitorEditor.pushTokenPlaceholder')" />
        </div>

        <p v-if="fieldErrors.target" class="text-xs text-destructive">{{ fieldErrors.target }}</p>
      </div>
      <div class="grid grid-cols-2 gap-3">
        <div class="grid gap-2">
          <Label>{{ t('monitorEditor.intervalSec') }}</Label>
          <Input v-model="form.interval_seconds" type="number" min="5" />
          <p v-if="fieldErrors.interval_seconds" class="text-xs text-destructive">{{ fieldErrors.interval_seconds }}</p>
        </div>
        <div class="grid gap-2">
          <Label>{{ t('monitorEditor.timeoutMs') }}</Label>
          <Input v-model="form.timeout_ms" type="number" min="500" />
          <p v-if="fieldErrors.timeout_ms" class="text-xs text-destructive">{{ fieldErrors.timeout_ms }}</p>
        </div>
      </div>
      <div class="grid grid-cols-3 gap-3">
        <div class="grid gap-2">
          <Label>{{ t('monitorEditor.retryAttempts') }}</Label>
          <Input v-model="form.retry_attempts" type="number" min="0" />
          <p v-if="fieldErrors.retry_attempts" class="text-xs text-destructive">{{ fieldErrors.retry_attempts }}</p>
        </div>
        <div class="grid gap-2">
          <Label>{{ t('monitorEditor.retryBackoffMs') }}</Label>
          <Input v-model="form.retry_backoff_ms" type="number" min="50" />
          <p v-if="fieldErrors.retry_backoff_ms" class="text-xs text-destructive">{{ fieldErrors.retry_backoff_ms }}</p>
        </div>
        <div class="grid gap-2">
          <Label>{{ t('monitorEditor.scheduleJitterMs') }}</Label>
          <Input v-model="form.schedule_jitter_ms" type="number" min="0" />
          <p v-if="fieldErrors.schedule_jitter_ms" class="text-xs text-destructive">{{ fieldErrors.schedule_jitter_ms }}</p>
        </div>
      </div>
      <div class="grid grid-cols-3 gap-3">
        <div class="grid gap-2">
          <Label>{{ t('monitorEditor.downConfirmChecks') }}</Label>
          <Input v-model="form.down_confirm_attempts" type="number" min="1" />
          <p v-if="fieldErrors.down_confirm_attempts" class="text-xs text-destructive">{{ fieldErrors.down_confirm_attempts }}</p>
        </div>
        <div class="grid gap-2">
          <Label>{{ t('monitorEditor.recoveryConfirmChecks') }}</Label>
          <Input v-model="form.recovery_confirm_attempts" type="number" min="1" />
          <p v-if="fieldErrors.recovery_confirm_attempts" class="text-xs text-destructive">{{ fieldErrors.recovery_confirm_attempts }}</p>
        </div>
        <div class="grid gap-2">
          <Label>{{ t('monitorEditor.repeatDownEvery') }}</Label>
          <Input v-model="form.resend_interval_checks" type="number" min="0" />
          <p v-if="fieldErrors.resend_interval_checks" class="text-xs text-destructive">{{ fieldErrors.resend_interval_checks }}</p>
        </div>
      </div>
      <p class="text-xs text-muted-foreground">
        {{ t('monitorEditor.alertPolicyHelp') }}
      </p>
      <div class="grid gap-2">
        <Label>{{ t('settings.channels.title') }}</Label>
        <div v-if="channelsLoading.value" class="text-xs text-muted-foreground">{{ t('settings.channels.loading') }}</div>
        <div v-else-if="channelsError.text" class="text-xs text-destructive">{{ channelsError.text }}</div>
        <div v-else-if="notificationChannels.length === 0" class="text-xs text-muted-foreground">
          {{ t('monitorEditor.noChannelsFound') }}
        </div>
        <div v-else class="grid gap-2 rounded-md border border-border p-3">
          <div
            v-for="channel in sortedNotificationChannels"
            :key="channel.id"
            class="flex items-center justify-between gap-3"
          >
            <div class="min-w-0">
              <p class="truncate text-sm font-medium">{{ channel.name }}</p>
              <div class="mt-1 flex items-center gap-2">
                <Badge variant="outline" class="gap-1.5">
                  <svg
                    v-if="providerBrandIcon(channel.provider)"
                    viewBox="0 0 24 24"
                    class="size-3.5 text-muted-foreground"
                    fill="currentColor"
                    aria-hidden="true"
                  >
                    <path :d="providerBrandIcon(channel.provider)!.path" />
                  </svg>
                  <component v-else :is="providerIcon(channel.provider)" class="size-3.5 text-muted-foreground" />
                  <span>{{ providerLabel(channel.provider) }}</span>
                </Badge>
                <Badge :variant="channel.is_enabled ? 'default' : 'outline'">
                  {{ channel.is_enabled ? t('settings.channels.enabled') : t('settings.channels.disabled') }}
                </Badge>
              </div>
            </div>
            <Switch
              :model-value="form.notification_channel_ids.includes(Number(channel.id))"
              :aria-label="t('monitorEditor.enableChannelAria', { name: channel.name })"
              @update:model-value="(v) => toggleNotificationChannel(Number(channel.id), Boolean(v))"
            />
          </div>
          <p class="text-xs text-muted-foreground">
            {{ t('monitorEditor.leaveEmptyAlerts') }}
          </p>
        </div>
        <p v-if="fieldErrors.notification_channel_ids" class="text-xs text-destructive">
          {{ fieldErrors.notification_channel_ids }}
        </p>
      </div>
      <div class="flex items-center justify-between rounded-md border border-border p-2">
        <Label>{{ t('settings.channels.enabled') }}</Label>
        <Switch
          :model-value="form.enabled"
          :aria-label="t('monitorEditor.monitorEnabledAria')"
          @update:model-value="(v) => (form.enabled = !!v)"
        />
      </div>
      <Button variant="outline" class="w-full gap-2" :disabled="saveLoading.value" @click="save">
        <Loader2 v-if="saveLoading.value" class="size-4 animate-spin" />
        <Save v-else class="size-4" />
        {{ monitor?.id ? t('monitorEditor.saveMonitor') : t('monitorEditor.createMonitor') }}
      </Button>
      <Button
        v-if="monitor?.id"
        variant="outline"
        class="w-full gap-2 border-destructive/40 text-destructive hover:bg-destructive/10 hover:text-destructive"
        :disabled="deleteLoading.value"
        @click="remove"
      >
        <Loader2 v-if="deleteLoading.value" class="size-4 animate-spin" />
        <Trash2 v-else class="size-4" />
        {{ t('monitorEditor.deleteMonitor') }}
      </Button>
      <p v-if="errorMessage.text" class="text-sm text-destructive">
        {{ errorMessage.text }}
      </p>
    </CardContent>
  </Card>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from 'vue'
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
import { BellRing, FileText, Loader2, Mail, Pencil, Plus, Save, Trash2, Webhook } from 'lucide-vue-next'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Separator } from '@/components/ui/separator'
import { Switch } from '@/components/ui/switch'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  clearMonitoringFaviconCache,
  createNotificationChannel,
  deleteNotificationChannel,
  fetchNotificationChannels,
  type NotificationChannel,
  fetchMonitoringSettings,
  updateNotificationChannel,
  updateMonitoringSettings,
} from '@/lib/monitoring-requests'
import {
  isSupportedNotificationProvider,
  normalizeProviderID,
  NOTIFICATION_PROVIDER_OPTIONS,
  notificationProviderLabel,
  type NotificationProvider,
} from '@/lib/notification-providers'

type ProviderFieldLocation = 'config' | 'secret'
type ProviderFieldType = 'text' | 'password' | 'number' | 'select'

type ProviderFieldOption = {
  label: string
  value: string
}

type ProviderField = {
  key: string
  label: string
  location: ProviderFieldLocation
  required?: boolean
  placeholder?: string
  type?: ProviderFieldType
  options?: ProviderFieldOption[]
  defaultValue?: string
  className?: string
}

const timeoutField: ProviderField = {
  key: 'timeout_seconds',
  label: 'Timeout (seconds)',
  location: 'config',
  type: 'number',
  defaultValue: '5',
}

const genericWebhookFields: ProviderField[] = [
  {
    key: 'url',
    label: 'Webhook URL',
    location: 'config',
    required: true,
    placeholder: 'https://hooks.example.com/...',
    className: 'md:col-span-6',
  },
  timeoutField,
  {
    key: 'bearer_token',
    label: 'Bearer Token (optional)',
    location: 'secret',
    type: 'password',
  },
  {
    key: 'authorization',
    label: 'Authorization Header (optional)',
    location: 'secret',
    type: 'password',
  },
]

const simpleWebhookFields: ProviderField[] = [
  {
    key: 'url',
    label: 'Webhook URL',
    location: 'config',
    required: true,
    placeholder: 'https://hooks.example.com/...',
    className: 'md:col-span-6',
  },
  timeoutField,
]

const smtpFields: ProviderField[] = [
  {
    key: 'smtp_host',
    label: 'SMTP Host',
    location: 'config',
    required: true,
    placeholder: 'smtp.mailgun.org',
  },
  {
    key: 'smtp_port',
    label: 'SMTP Port',
    location: 'config',
    required: true,
    type: 'number',
    defaultValue: '587',
  },
  {
    key: 'from_email',
    label: 'From Email',
    location: 'config',
    required: true,
    placeholder: 'alerts@example.com',
  },
  {
    key: 'to_emails',
    label: 'To Emails (comma-separated)',
    location: 'config',
    required: true,
    placeholder: 'ops@example.com, dev@example.com',
    className: 'md:col-span-6',
  },
  {
    key: 'subject_prefix',
    label: 'Subject Prefix (optional)',
    location: 'config',
    placeholder: '[Uptime Gopher]',
  },
  {
    key: 'smtp_username',
    label: 'SMTP Username (optional)',
    location: 'secret',
  },
  {
    key: 'smtp_password',
    label: 'SMTP Password (optional)',
    location: 'secret',
    type: 'password',
  },
  timeoutField,
]

const providerFieldMap: Record<string, ProviderField[]> = {
  log: [],
  email: smtpFields,
  smtp: smtpFields,
  webhook: genericWebhookFields,
  slack: simpleWebhookFields,
  discord: simpleWebhookFields,
  teams: [
    {
      key: 'webhook_url',
      label: 'Teams Webhook URL',
      location: 'config',
      required: true,
      placeholder: 'https://outlook.office.com/webhook/...',
      className: 'md:col-span-6',
    },
    timeoutField,
  ],
  telegram: [
    {
      key: 'telegram_chat_id',
      label: 'Telegram Chat ID',
      location: 'config',
      required: true,
      placeholder: '123456789',
    },
    {
      key: 'telegram_server_url',
      label: 'Telegram API URL',
      location: 'config',
      defaultValue: 'https://api.telegram.org',
    },
    {
      key: 'telegram_message_thread_id',
      label: 'Thread ID (optional)',
      location: 'config',
    },
    {
      key: 'telegram_send_silently',
      label: 'Send Silently',
      location: 'config',
      type: 'select',
      defaultValue: 'false',
      options: [
        { value: 'false', label: 'No' },
        { value: 'true', label: 'Yes' },
      ],
    },
    {
      key: 'telegram_protect_content',
      label: 'Protect Content',
      location: 'config',
      type: 'select',
      defaultValue: 'false',
      options: [
        { value: 'false', label: 'No' },
        { value: 'true', label: 'Yes' },
      ],
    },
    {
      key: 'telegram_bot_token',
      label: 'Bot Token',
      location: 'secret',
      required: true,
      type: 'password',
      className: 'md:col-span-6',
    },
    timeoutField,
  ],
  pagerduty: [
    {
      key: 'pagerduty_integration_key',
      label: 'Integration Key (Routing Key)',
      location: 'secret',
      required: true,
      type: 'password',
      className: 'md:col-span-6',
    },
    {
      key: 'pagerduty_integration_url',
      label: 'Events API URL',
      location: 'config',
      defaultValue: 'https://events.pagerduty.com/v2/enqueue',
      className: 'md:col-span-6',
    },
    {
      key: 'pagerduty_priority',
      label: 'Severity',
      location: 'config',
      defaultValue: 'warning',
    },
    {
      key: 'pagerduty_auto_resolve',
      label: 'Auto Resolve Mode',
      location: 'config',
      type: 'select',
      defaultValue: 'resolve',
      options: [
        { value: 'resolve', label: 'Resolve' },
        { value: 'acknowledge', label: 'Acknowledge' },
        { value: 'none', label: 'Disabled' },
      ],
    },
    timeoutField,
  ],
  jiraservicemanagement: [
    {
      key: 'jsm_cloud_id',
      label: 'Cloud ID',
      location: 'config',
      required: true,
    },
    {
      key: 'jsm_email',
      label: 'Email',
      location: 'config',
      required: true,
      placeholder: 'ops@example.com',
    },
    {
      key: 'jsm_api_token',
      label: 'API Token',
      location: 'secret',
      required: true,
      type: 'password',
    },
    {
      key: 'jsm_priority',
      label: 'Priority',
      location: 'config',
      type: 'select',
      defaultValue: 'P3',
      options: [
        { value: 'P1', label: 'P1' },
        { value: 'P2', label: 'P2' },
        { value: 'P3', label: 'P3' },
        { value: 'P4', label: 'P4' },
        { value: 'P5', label: 'P5' },
      ],
    },
    {
      key: 'jsm_base_url',
      label: 'Base URL Override (optional)',
      location: 'config',
      className: 'md:col-span-6',
    },
    timeoutField,
  ],
  googlechat: [
    {
      key: 'google_chat_webhook_url',
      label: 'Google Chat Webhook URL',
      location: 'config',
      required: true,
      className: 'md:col-span-6',
      placeholder: 'https://chat.googleapis.com/v1/spaces/...',
    },
    timeoutField,
  ],
  twilio: [
    {
      key: 'twilio_account_sid',
      label: 'Account SID',
      location: 'config',
      required: true,
    },
    {
      key: 'twilio_to_number',
      label: 'To Number',
      location: 'config',
      required: true,
      placeholder: '+15555550123',
    },
    {
      key: 'twilio_from_number',
      label: 'From Number',
      location: 'config',
      placeholder: '+15555550999',
    },
    {
      key: 'twilio_messaging_service_sid',
      label: 'Messaging Service SID',
      location: 'config',
    },
    {
      key: 'twilio_auth_token',
      label: 'Auth Token',
      location: 'secret',
      required: true,
      type: 'password',
    },
    {
      key: 'twilio_api_key',
      label: 'API Key (optional)',
      location: 'secret',
      type: 'password',
    },
    timeoutField,
  ],
  opsgenie: [
    {
      key: 'opsgenie_api_key',
      label: 'API Key',
      location: 'secret',
      required: true,
      type: 'password',
      className: 'md:col-span-6',
    },
    {
      key: 'opsgenie_region',
      label: 'Region',
      location: 'config',
      type: 'select',
      defaultValue: 'us',
      options: [
        { value: 'us', label: 'US' },
        { value: 'eu', label: 'EU' },
      ],
    },
    {
      key: 'opsgenie_priority',
      label: 'Priority',
      location: 'config',
      type: 'select',
      defaultValue: 'P3',
      options: [
        { value: 'P1', label: 'P1' },
        { value: 'P2', label: 'P2' },
        { value: 'P3', label: 'P3' },
        { value: 'P4', label: 'P4' },
        { value: 'P5', label: 'P5' },
      ],
    },
    timeoutField,
  ],
  ntfy: [
    {
      key: 'ntfy_server_url',
      label: 'Server URL',
      location: 'config',
      required: true,
      placeholder: 'https://ntfy.sh',
      className: 'md:col-span-6',
    },
    {
      key: 'ntfy_topic',
      label: 'Topic',
      location: 'config',
      required: true,
    },
    {
      key: 'ntfy_authentication_method',
      label: 'Authentication Method',
      location: 'config',
      type: 'select',
      defaultValue: 'accesstoken',
      options: [
        { value: 'accesstoken', label: 'Access Token' },
        { value: 'usernamepassword', label: 'Username + Password' },
      ],
    },
    {
      key: 'ntfy_username',
      label: 'Username (optional)',
      location: 'config',
    },
    {
      key: 'ntfy_password',
      label: 'Password (optional)',
      location: 'secret',
      type: 'password',
    },
    {
      key: 'ntfy_access_token',
      label: 'Access Token (optional)',
      location: 'secret',
      type: 'password',
    },
    {
      key: 'ntfy_priority',
      label: 'Priority',
      location: 'config',
      defaultValue: '4',
    },
    {
      key: 'ntfy_call',
      label: 'Call Header (optional)',
      location: 'config',
    },
    {
      key: 'ntfy_icon',
      label: 'Icon URL (optional)',
      location: 'config',
      className: 'md:col-span-6',
    },
    timeoutField,
  ],
  gotify: [
    {
      key: 'gotify_server_url',
      label: 'Server URL',
      location: 'config',
      required: true,
      className: 'md:col-span-6',
      placeholder: 'https://gotify.example.com',
    },
    {
      key: 'gotify_application_token',
      label: 'Application Token',
      location: 'secret',
      required: true,
      type: 'password',
    },
    {
      key: 'gotify_priority',
      label: 'Priority',
      location: 'config',
      defaultValue: '8',
    },
    timeoutField,
  ],
  wecom: [
    {
      key: 'wecom_bot_key',
      label: 'Bot Key',
      location: 'secret',
      required: true,
      type: 'password',
      className: 'md:col-span-6',
    },
    {
      key: 'wecom_mentioned_mobile_list',
      label: 'Mentioned Mobile List (comma-separated, optional)',
      location: 'config',
      className: 'md:col-span-6',
    },
    timeoutField,
  ],
  dingding: [
    {
      key: 'dingding_webhook_url',
      label: 'DingDing Webhook URL',
      location: 'config',
      required: true,
      className: 'md:col-span-6',
    },
    {
      key: 'dingding_secret_key',
      label: 'Secret Key (optional)',
      location: 'secret',
      type: 'password',
    },
    timeoutField,
  ],
  feishu: [
    {
      key: 'feishu_webhook_url',
      label: 'Feishu Webhook URL',
      location: 'config',
      required: true,
      className: 'md:col-span-6',
    },
    timeoutField,
  ],
  line: [
    {
      key: 'line_user_id',
      label: 'User ID',
      location: 'config',
      required: true,
    },
    {
      key: 'line_channel_access_token',
      label: 'Channel Access Token',
      location: 'secret',
      required: true,
      type: 'password',
      className: 'md:col-span-6',
    },
    {
      key: 'line_api_url',
      label: 'LINE API URL',
      location: 'config',
      defaultValue: 'https://api.line.me',
      className: 'md:col-span-6',
    },
    timeoutField,
  ],
  waha: [
    {
      key: 'waha_api_url',
      label: 'WAHA API URL',
      location: 'config',
      required: true,
      className: 'md:col-span-6',
      placeholder: 'https://waha.example.com',
    },
    {
      key: 'waha_session',
      label: 'Session',
      location: 'config',
      required: true,
    },
    {
      key: 'waha_chat_id',
      label: 'Chat ID',
      location: 'config',
      required: true,
      placeholder: '123456789@c.us',
    },
    {
      key: 'waha_api_key',
      label: 'API Key',
      location: 'secret',
      required: true,
      type: 'password',
    },
    timeoutField,
  ],
  whapi: [
    {
      key: 'whapi_recipient',
      label: 'Recipient',
      location: 'config',
      required: true,
      placeholder: '1234567890',
    },
    {
      key: 'whapi_auth_token',
      label: 'Auth Token',
      location: 'secret',
      required: true,
      type: 'password',
      className: 'md:col-span-6',
    },
    {
      key: 'whapi_api_url',
      label: 'WHAPI API URL',
      location: 'config',
      defaultValue: 'https://gate.whapi.cloud',
      className: 'md:col-span-6',
    },
    timeoutField,
  ],
  grafanaoncall: genericWebhookFields,
  googlesheets: genericWebhookFields,
  'rocket.chat': genericWebhookFields,
  sendgrid: genericWebhookFields,
  splunk: genericWebhookFields,
}

function mapValue(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function normalizeStringMap(input: Record<string, string> | undefined): Record<string, string> {
  const out: Record<string, string> = {}
  const source = input ?? {}
  for (const [rawKey, rawValue] of Object.entries(source)) {
    const key = String(rawKey || '').trim()
    const value = mapValue(rawValue).trim()
    if (!key || !value) continue
    out[key] = value
  }
  return out
}

function applyFieldDefaults(provider: NotificationProvider, config: Record<string, string>, secrets: Record<string, string>) {
  const fields = providerFields(provider)
  for (const field of fields) {
    if (!field.defaultValue) continue
    if (field.location === 'config' && !config[field.key]) {
      config[field.key] = field.defaultValue
    }
    if (field.location === 'secret' && !secrets[field.key]) {
      secrets[field.key] = field.defaultValue
    }
  }
}

type ChannelDraft = {
  id?: number
  name: string
  provider: NotificationProvider
  is_enabled: boolean
  config: Record<string, string>
  secrets: Record<string, string>
}

function newDraft(provider: NotificationProvider = 'log'): ChannelDraft {
  const draft: ChannelDraft = {
    name: '',
    provider,
    is_enabled: true,
    config: {},
    secrets: {},
  }
  applyFieldDefaults(provider, draft.config, draft.secrets)
  return draft
}

const faviconCacheTTLSeconds = ref(604800)
const loading = ref(true)
const saving = ref(false)
const clearingCache = ref(false)
const channelsLoading = ref(false)
const settingsError = ref('')
const settingsNotice = ref('')
const channelError = ref('')
const channelNotice = ref('')
const channels = ref<NotificationChannel[]>([])
const channelSaving = ref<Record<number, boolean>>({})
const channelDeleting = ref<Record<number, boolean>>({})
const channelEditorOpen = ref<Record<number, boolean>>({})
const channelSecretInputs = ref<Record<number, Record<string, string>>>({})
const draft = ref<ChannelDraft>(newDraft())

function clearSettingsMessages() {
  settingsError.value = ''
  settingsNotice.value = ''
}

function clearChannelMessages() {
  channelError.value = ''
  channelNotice.value = ''
}

function normalizeProvider(provider: NotificationProvider | string): NotificationProvider {
  const value = normalizeProviderID(provider)
  if (isSupportedNotificationProvider(value)) return value
  return value || 'log'
}

function providerLabel(provider: NotificationProvider) {
  return notificationProviderLabel(provider)
}

function providerFields(provider: NotificationProvider | string): ProviderField[] {
  return providerFieldMap[normalizeProvider(provider)] ?? []
}

function fieldClassName(field: ProviderField): string {
  return field.className || 'md:col-span-2'
}

function fieldInputType(field: ProviderField): string {
  if (field.type === 'password') return 'password'
  if (field.type === 'number') return 'number'
  return 'text'
}

function updateDraftProvider(value: string) {
  draft.value.provider = normalizeProvider(value)
  applyFieldDefaults(draft.value.provider, draft.value.config, draft.value.secrets)
}

function updateChannelProvider(channel: NotificationChannel, value: string) {
  channel.provider = normalizeProvider(value)
  channel.config = { ...(channel.config ?? {}) }
  applyFieldDefaults(channel.provider, channel.config, {})
}

function setDraftConfigField(field: ProviderField, value: string) {
  draft.value.config[field.key] = String(value ?? '')
}

function setDraftSecretField(field: ProviderField, value: string) {
  draft.value.secrets[field.key] = String(value ?? '')
}

function draftFieldValue(field: ProviderField): string {
  if (field.location === 'config') return draft.value.config[field.key] ?? ''
  return draft.value.secrets[field.key] ?? ''
}

function channelSecretInput(id: number): Record<string, string> {
  if (!channelSecretInputs.value[id]) {
    channelSecretInputs.value[id] = {}
  }
  return channelSecretInputs.value[id]
}

function channelFieldValue(channel: NotificationChannel, field: ProviderField): string {
  if (field.location === 'config') {
    return (channel.config ?? {})[field.key] ?? ''
  }
  return channelSecretInput(Number(channel.id))[field.key] ?? ''
}

function setChannelConfigField(channel: NotificationChannel, field: ProviderField, value: string) {
  if (!channel.config) channel.config = {}
  channel.config[field.key] = String(value ?? '')
}

function setChannelSecretField(channel: NotificationChannel, field: ProviderField, value: string) {
  channelSecretInput(Number(channel.id))[field.key] = String(value ?? '')
}

function fieldRequiredLabel(field: ProviderField): string {
  return field.required ? `${field.label} *` : field.label
}

function secretFieldsPresent(channel: NotificationChannel) {
  const present = Array.isArray(channel.secrets_present) ? channel.secrets_present : []
  return present.join(', ')
}

function existingSecretSet(channel: NotificationChannel): Set<string> {
  const present = Array.isArray(channel.secrets_present) ? channel.secrets_present : []
  return new Set(present.map((key) => String(key || '').trim()).filter(Boolean))
}

function missingRequiredFields(
  provider: NotificationProvider,
  config: Record<string, string>,
  secrets: Record<string, string>,
  existingSecrets: Set<string>,
): string[] {
  const missing: string[] = []
  for (const field of providerFields(provider)) {
    if (!field.required) continue
    if (field.location === 'config') {
      if (!mapValue(config[field.key]).trim()) {
        missing.push(field.label)
      }
      continue
    }
    const newValue = mapValue(secrets[field.key]).trim()
    if (!newValue && !existingSecrets.has(field.key)) {
      missing.push(field.label)
    }
  }
  if (normalizeProvider(provider) === 'twilio') {
    const fromNumber = mapValue(config.twilio_from_number).trim()
    const messagingService = mapValue(config.twilio_messaging_service_sid).trim()
    if (!fromNumber && !messagingService) {
      missing.push('Twilio From Number or Messaging Service SID')
    }
  }
  return missing
}

function canCreateDraft() {
  const name = draft.value.name.trim()
  if (!name) return false
  const missing = missingRequiredFields(
    draft.value.provider,
    draft.value.config,
    draft.value.secrets,
    new Set<string>(),
  )
  return missing.length === 0
}

function providerIcon(provider: NotificationProvider) {
  if (provider === 'log') return FileText
  if (provider === 'email' || provider === 'smtp') return Mail
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

function providerBrandIcon(provider: NotificationProvider) {
  return providerBrandIcons[normalizeProviderID(provider)] ?? null
}

const sortedChannels = computed(() =>
  [...channels.value].sort((a, b) => Number(b.is_enabled) - Number(a.is_enabled) || a.name.localeCompare(b.name)),
)

watch(
  () => draft.value.provider,
  (provider) => {
    applyFieldDefaults(provider, draft.value.config, draft.value.secrets)
  },
)

async function loadSettings() {
  loading.value = true
  clearSettingsMessages()
  try {
    const payload = await fetchMonitoringSettings()
    const raw = Number(payload?.settings?.favicon_cache_ttl_seconds ?? 604800)
    faviconCacheTTLSeconds.value = Number.isFinite(raw) && raw > 0 ? Math.floor(raw) : 604800
  } catch {
    settingsError.value = 'Failed to load settings.'
  } finally {
    loading.value = false
  }
}

async function saveSettings() {
  clearSettingsMessages()
  const ttl = Math.floor(Number(faviconCacheTTLSeconds.value))
  if (!Number.isFinite(ttl) || ttl < 60 || ttl > 2592000) {
    settingsError.value = 'Favicon cache TTL must be between 60 and 2592000 seconds.'
    return
  }
  saving.value = true
  try {
    await updateMonitoringSettings({ favicon_cache_ttl_seconds: ttl })
    settingsNotice.value = 'Settings saved.'
  } catch (err: any) {
    settingsError.value = typeof err?.message === 'string' ? err.message : 'Failed to save settings.'
  } finally {
    saving.value = false
  }
}

async function clearFaviconCache() {
  clearSettingsMessages()
  clearingCache.value = true
  try {
    const payload = await clearMonitoringFaviconCache()
    const removed = Number(payload?.removed_files ?? 0)
    settingsNotice.value = `Favicon cache cleared (${removed} files removed).`
  } catch (err: any) {
    settingsError.value = typeof err?.message === 'string' ? err.message : 'Failed to clear cache.'
  } finally {
    clearingCache.value = false
  }
}

async function loadChannels() {
  channelsLoading.value = true
  clearChannelMessages()
  try {
    const payload = await fetchNotificationChannels()
    const raw = Array.isArray(payload?.channels) ? payload.channels : []
    channels.value = (raw as NotificationChannel[]).map((channel) => {
      const provider = normalizeProvider(channel.provider)
      const config = { ...(channel?.config ?? {}) }
      applyFieldDefaults(provider, config, {})
      return {
        ...channel,
        provider,
        config,
      }
    })
    channelEditorOpen.value = channels.value.reduce<Record<number, boolean>>((acc, channel) => {
      acc[Number(channel.id)] = false
      return acc
    }, {})
  } catch (err: any) {
    channelError.value = typeof err?.message === 'string' ? err.message : 'Failed to load notification channels.'
  } finally {
    channelsLoading.value = false
  }
}

function isChannelEditorOpen(id: number) {
  return Boolean(channelEditorOpen.value[id])
}

function toggleChannelEditor(id: number) {
  channelEditorOpen.value[id] = !isChannelEditorOpen(id)
}

async function createChannel() {
  clearChannelMessages()
  const missing = missingRequiredFields(
    draft.value.provider,
    draft.value.config,
    draft.value.secrets,
    new Set<string>(),
  )
  if (!draft.value.name.trim() || missing.length > 0) {
    channelError.value = missing.length > 0
      ? `Missing required fields: ${missing.join(', ')}`
      : 'Name is required.'
    return
  }

  try {
    const provider = normalizeProvider(draft.value.provider)
    const config = normalizeStringMap(draft.value.config)
    const secrets = normalizeStringMap(draft.value.secrets)
    const payload: {
      name: string
      provider: NotificationProvider
      is_enabled: boolean
      config: Record<string, string>
      secrets?: Record<string, string>
    } = {
      name: draft.value.name.trim(),
      provider,
      is_enabled: draft.value.is_enabled,
      config,
    }
    if (Object.keys(secrets).length > 0) {
      payload.secrets = secrets
    }
    await createNotificationChannel(payload)
    draft.value = newDraft()
    await loadChannels()
    channelNotice.value = 'Notification channel added.'
  } catch (err: any) {
    channelError.value = typeof err?.message === 'string' ? err.message : 'Failed to create notification channel.'
  }
}

async function saveChannel(channel: NotificationChannel) {
  const id = Number(channel.id)
  channelSaving.value[id] = true
  clearChannelMessages()
  try {
    const provider = normalizeProvider(channel.provider)
    const config = normalizeStringMap(channel.config ?? {})
    const secretPatch = normalizeStringMap(channelSecretInput(id))
    const missing = missingRequiredFields(provider, config, secretPatch, existingSecretSet(channel))
    if (!String(channel.name || '').trim() || missing.length > 0) {
      channelError.value = missing.length > 0
        ? `Missing required fields for ${channel.name}: ${missing.join(', ')}`
        : 'Name is required.'
      return
    }

    const payload: {
      name: string
      provider: NotificationProvider
      is_enabled: boolean
      config: Record<string, string>
      secrets?: Record<string, string>
    } = {
      name: channel.name,
      provider,
      is_enabled: Boolean(channel.is_enabled),
      config,
    }
    if (Object.keys(secretPatch).length > 0) {
      payload.secrets = secretPatch
    }

    await updateNotificationChannel(id, payload)
    channelSecretInputs.value[id] = {}
    channelNotice.value = `Saved ${channel.name}.`
    await loadChannels()
  } catch (err: any) {
    channelError.value = typeof err?.message === 'string' ? err.message : 'Failed to save notification channel.'
  } finally {
    channelSaving.value[id] = false
  }
}

async function removeChannel(channel: NotificationChannel) {
  if (!confirm(`Delete notification channel "${channel.name}"?`)) return
  const id = Number(channel.id)
  channelDeleting.value[id] = true
  clearChannelMessages()
  try {
    await deleteNotificationChannel(id)
    channels.value = channels.value.filter((c) => Number(c.id) !== id)
    channelNotice.value = `Deleted ${channel.name}.`
  } catch (err: any) {
    channelError.value = typeof err?.message === 'string' ? err.message : 'Failed to delete notification channel.'
  } finally {
    channelDeleting.value[id] = false
  }
}

onMounted(() => {
  void loadSettings()
  void loadChannels()
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

          <div v-if="settingsError" class="text-sm text-rose-400">{{ settingsError }}</div>
          <div v-else-if="settingsNotice" class="text-sm text-emerald-400">{{ settingsNotice }}</div>

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

    <div class="px-4 lg:px-6">
      <Card>
        <CardHeader>
          <CardTitle class="flex items-center gap-2">
            <BellRing class="size-4" />
            Notification channels
          </CardTitle>
        </CardHeader>
        <CardContent class="space-y-4">
          <div class="grid gap-x-3 gap-y-4 rounded-md border p-4 md:grid-cols-6">
            <div class="mt-1 space-y-2 md:col-span-2">
              <Label for="channel-name">Name</Label>
              <Input id="channel-name" v-model="draft.name" placeholder="PagerDuty Webhook" />
            </div>
            <div class="mt-1 space-y-2 md:col-span-2">
              <Label>Provider</Label>
              <Select :model-value="draft.provider" @update:model-value="updateDraftProvider(String($event ?? ''))">
                <SelectTrigger class="gap-2">
                  <svg
                    v-if="providerBrandIcon(draft.provider)"
                    viewBox="0 0 24 24"
                    class="size-4 text-muted-foreground"
                    fill="currentColor"
                    aria-hidden="true"
                  >
                    <path :d="providerBrandIcon(draft.provider)!.path" />
                  </svg>
                  <component v-else :is="providerIcon(draft.provider)" class="size-4 text-muted-foreground" />
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem v-for="provider in NOTIFICATION_PROVIDER_OPTIONS" :key="provider.id" :value="provider.id">
                    <div class="flex items-center gap-2">
                      <svg
                        v-if="providerBrandIcon(provider.id)"
                        viewBox="0 0 24 24"
                        class="size-4 text-muted-foreground"
                        fill="currentColor"
                        aria-hidden="true"
                      >
                        <path :d="providerBrandIcon(provider.id)!.path" />
                      </svg>
                      <component v-else :is="providerIcon(provider.id)" class="size-4 text-muted-foreground" />
                      <span>{{ provider.label }}</span>
                    </div>
                  </SelectItem>
                </SelectContent>
              </Select>
            </div>
            <div class="mt-1 space-y-2 md:col-span-1">
              <Label>Enabled</Label>
              <div class="mt-2 flex h-10 items-center">
                <Switch :model-value="draft.is_enabled" @update:model-value="draft.is_enabled = Boolean($event)" />
              </div>
            </div>

            <template v-for="field in providerFields(draft.provider)" :key="`create-${field.location}-${field.key}`">
              <div class="space-y-2" :class="fieldClassName(field)">
                <Label>{{ fieldRequiredLabel(field) }}</Label>
                <Select
                  v-if="field.type === 'select' && field.options"
                  :model-value="draftFieldValue(field)"
                  @update:model-value="field.location === 'config' ? setDraftConfigField(field, String($event ?? '')) : setDraftSecretField(field, String($event ?? ''))"
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem v-for="option in field.options" :key="option.value" :value="option.value">
                      {{ option.label }}
                    </SelectItem>
                  </SelectContent>
                </Select>
                <Input
                  v-else
                  :type="fieldInputType(field)"
                  :placeholder="field.placeholder || ''"
                  :model-value="draftFieldValue(field)"
                  @update:model-value="field.location === 'config' ? setDraftConfigField(field, String($event ?? '')) : setDraftSecretField(field, String($event ?? ''))"
                />
              </div>
            </template>

            <div class="flex items-end justify-start md:col-span-6">
              <Button type="button" variant="outline" class="gap-2" :disabled="!canCreateDraft()" @click="createChannel">
                <Plus class="size-4" />
                Add Channel
              </Button>
            </div>
          </div>

          <div v-if="channelError" class="text-sm text-rose-400">{{ channelError }}</div>
          <div v-else-if="channelNotice" class="text-sm text-emerald-400">{{ channelNotice }}</div>

          <Separator />

          <div v-if="channelsLoading" class="text-sm text-muted-foreground">Loading channels...</div>
          <div v-else-if="sortedChannels.length === 0" class="text-sm text-muted-foreground">
            No channels yet. Add one to route alerts.
          </div>
          <div v-else class="space-y-3">
            <Card v-for="channel in sortedChannels" :key="channel.id">
              <CardContent class="space-y-3 pt-0">
                <div class="flex flex-wrap items-center justify-between gap-2">
                  <div class="flex items-center gap-2">
                    <h3 class="text-sm font-semibold">{{ channel.name }}</h3>
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
                      {{ channel.is_enabled ? 'Enabled' : 'Disabled' }}
                    </Badge>
                  </div>
                  <div class="flex items-center gap-2">
                    <Button
                      type="button"
                      size="sm"
                      variant="outline"
                      class="gap-2"
                      @click="toggleChannelEditor(Number(channel.id))"
                    >
                      <Pencil class="size-4" />
                      {{ isChannelEditorOpen(Number(channel.id)) ? 'Close' : 'Edit' }}
                    </Button>
                    <Button
                      v-if="isChannelEditorOpen(Number(channel.id))"
                      type="button"
                      size="sm"
                      variant="outline"
                      class="gap-2"
                      :disabled="channelSaving[channel.id]"
                      @click="saveChannel(channel)"
                    >
                      <Loader2 v-if="channelSaving[channel.id]" class="size-4 animate-spin" />
                      <Save v-else class="size-4" />
                      Save
                    </Button>
                    <Button
                      type="button"
                      size="sm"
                      variant="destructive"
                      class="gap-2"
                      :disabled="channelDeleting[channel.id]"
                      @click="removeChannel(channel)"
                    >
                      <Loader2 v-if="channelDeleting[channel.id]" class="size-4 animate-spin" />
                      <Trash2 v-else class="size-4" />
                      Delete
                    </Button>
                  </div>
                </div>

                <div v-if="isChannelEditorOpen(Number(channel.id))" class="grid gap-x-3 gap-y-4 md:grid-cols-6">
                  <div class="space-y-2 md:col-span-2">
                    <Label>Name</Label>
                    <Input v-model="channel.name" />
                  </div>
                  <div class="space-y-2">
                    <Label>Provider</Label>
                    <Select :model-value="channel.provider" @update:model-value="updateChannelProvider(channel, String($event ?? ''))">
                      <SelectTrigger class="gap-2">
                        <svg
                          v-if="providerBrandIcon(channel.provider)"
                          viewBox="0 0 24 24"
                          class="size-4 text-muted-foreground"
                          fill="currentColor"
                          aria-hidden="true"
                        >
                          <path :d="providerBrandIcon(channel.provider)!.path" />
                        </svg>
                        <component v-else :is="providerIcon(channel.provider)" class="size-4 text-muted-foreground" />
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem
                          v-for="provider in NOTIFICATION_PROVIDER_OPTIONS"
                          :key="provider.id"
                          :value="provider.id"
                        >
                          <div class="flex items-center gap-2">
                            <svg
                              v-if="providerBrandIcon(provider.id)"
                              viewBox="0 0 24 24"
                              class="size-4 text-muted-foreground"
                              fill="currentColor"
                              aria-hidden="true"
                            >
                              <path :d="providerBrandIcon(provider.id)!.path" />
                            </svg>
                            <component v-else :is="providerIcon(provider.id)" class="size-4 text-muted-foreground" />
                            <span>{{ provider.label }}</span>
                          </div>
                        </SelectItem>
                      </SelectContent>
                    </Select>
                  </div>
                  <div class="space-y-2">
                    <Label>Enabled</Label>
                    <div class="mt-2 flex h-10 items-center">
                      <Switch
                        :model-value="channel.is_enabled"
                        @update:model-value="channel.is_enabled = Boolean($event)"
                      />
                    </div>
                  </div>

                  <template v-for="field in providerFields(channel.provider)" :key="`edit-${channel.id}-${field.location}-${field.key}`">
                    <div class="space-y-2" :class="fieldClassName(field)">
                      <Label>{{ fieldRequiredLabel(field) }}</Label>
                      <Select
                        v-if="field.type === 'select' && field.options"
                        :model-value="channelFieldValue(channel, field)"
                        @update:model-value="field.location === 'config' ? setChannelConfigField(channel, field, String($event ?? '')) : setChannelSecretField(channel, field, String($event ?? ''))"
                      >
                        <SelectTrigger>
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem v-for="option in field.options" :key="option.value" :value="option.value">
                            {{ option.label }}
                          </SelectItem>
                        </SelectContent>
                      </Select>
                      <Input
                        v-else
                        :type="fieldInputType(field)"
                        :placeholder="field.placeholder || (field.location === 'secret' ? 'Leave blank to keep existing value' : '')"
                        :model-value="channelFieldValue(channel, field)"
                        @update:model-value="field.location === 'config' ? setChannelConfigField(channel, field, String($event ?? '')) : setChannelSecretField(channel, field, String($event ?? ''))"
                      />
                    </div>
                  </template>

                  <p class="text-xs text-muted-foreground md:col-span-6">
                    Secret fields in edit mode are patch-only. Leave blank to keep existing values.
                  </p>
                </div>
                <p v-if="channel.has_secrets" class="text-xs text-muted-foreground">
                  Stored secrets: {{ secretFieldsPresent(channel) || 'present' }}.
                </p>
              </CardContent>
            </Card>
          </div>
        </CardContent>
      </Card>
    </div>
  </div>
</template>

export type NotificationProvider = string

type NotificationProviderOption = {
  id: NotificationProvider
  label: string
}

const webhookCompatProviderIDs: NotificationProvider[] = [
  'dingding',
  'discord',
  'feishu',
  'grafanaoncall',
  'googlechat',
  'googlesheets',
  'gotify',
  'jiraservicemanagement',
  'line',
  'ntfy',
  'opsgenie',
  'pagerduty',
  'rocket.chat',
  'sendgrid',
  'slack',
  'smtp',
  'splunk',
  'teams',
  'telegram',
  'twilio',
  'waha',
  'webhook',
  'wecom',
  'whapi',
]

const labelOverrides: Record<string, string> = {
  log: 'Log',
  email: 'Email',
  smtp: 'SMTP',
  waha: 'WAHA',
  whapi: 'Whapi',
  wecom: 'WeCom',
  grafanaoncall: 'Grafana OnCall',
  jiraservicemanagement: 'Jira Service Management',
  'rocket.chat': 'Rocket.Chat',
  googlesheets: 'Google Sheets',
  googlechat: 'Google Chat',
  sendgrid: 'SendGrid',
  splunk: 'Splunk',
}

function toTitleCase(input: string): string {
  return input
    .split(/[^a-z0-9]+/i)
    .filter(Boolean)
    .map((part) => part[0].toUpperCase() + part.slice(1))
    .join(' ')
}

function optionLabel(provider: string): string {
  const normalized = normalizeProviderID(provider)
  if (labelOverrides[normalized]) {
    return labelOverrides[normalized]
  }
  return toTitleCase(normalized)
}

export function normalizeProviderID(provider: string): NotificationProvider {
  return String(provider || '')
    .trim()
    .toLowerCase()
}

export const NOTIFICATION_PROVIDER_OPTIONS: NotificationProviderOption[] = [
  { id: 'log', label: optionLabel('log') },
  { id: 'email', label: optionLabel('email') },
  ...webhookCompatProviderIDs.map((id) => ({
    id,
    label: optionLabel(id),
  })),
]

export const NOTIFICATION_PROVIDER_SET = new Set<string>(
  NOTIFICATION_PROVIDER_OPTIONS.map((provider) => provider.id),
)

export function isSupportedNotificationProvider(provider: string): boolean {
  return NOTIFICATION_PROVIDER_SET.has(normalizeProviderID(provider))
}

export function notificationProviderLabel(provider: string): string {
  return optionLabel(provider)
}

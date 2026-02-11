export type NotificationProvider = string

type NotificationProviderOption = {
  id: NotificationProvider
  label: string
}

const uptimeKumaProviderIDs: NotificationProvider[] = [
  'alerta',
  'alertnow',
  'aliyunsms',
  'apprise',
  'bale',
  'bark',
  'bitrix24',
  'brevo',
  'callmebot',
  'cellsynt',
  'clicksendsms',
  'dingding',
  'discord',
  'elks',
  'evolution',
  'feishu',
  'flashduty',
  'freemobile',
  'goalert',
  'googlechat',
  'googlesheets',
  'gorush',
  'gotify',
  'grafanaoncall',
  'gtxmessaging',
  'halopsa',
  'heiioncall',
  'homeassistant',
  'jiraservicemanagement',
  'keep',
  'kook',
  'line',
  'lunasea',
  'matrix',
  'mattermost',
  'nextcloudtalk',
  'nostr',
  'notifery',
  'ntfy',
  'octopush',
  'onebot',
  'onechat',
  'onesender',
  'opsgenie',
  'pagerduty',
  'pagertree',
  'promosms',
  'pumble',
  'pushbullet',
  'pushbytechulus',
  'pushdeer',
  'pushover',
  'pushplus',
  'pushy',
  'resend',
  'rocket.chat',
  'sendgrid',
  'serverchan',
  'serwersms',
  'sevenio',
  'signal',
  'signl4',
  'slack',
  'smsc',
  'smseagle',
  'smsir',
  'smsmanager',
  'smspartner',
  'smsplanet',
  'smtp',
  'splunk',
  'spugpush',
  'squadcast',
  'stackfield',
  'teams',
  'telegram',
  'threema',
  'twilio',
  'waha',
  'webhook',
  'webpush',
  'wecom',
  'whapi',
  'wpush',
  'yzj',
  'zohocliq',
]

const labelOverrides: Record<string, string> = {
  log: 'Log',
  email: 'Email',
  smtp: 'SMTP',
  smsc: 'SMSC',
  smsir: 'SMSIR',
  signl4: 'SIGNL4',
  yzj: 'YZJ',
  waha: 'WAHA',
  wpush: 'WPush',
  whapi: 'Whapi',
  wecom: 'WeCom',
  webpush: 'Webpush',
  halopsa: 'HaloPSA',
  heiioncall: 'Heii OnCall',
  grafanaoncall: 'Grafana OnCall',
  jiraservicemanagement: 'Jira Service Management',
  gtxmessaging: 'GTX Messaging',
  pushbytechulus: 'Push by Techulus',
  'rocket.chat': 'Rocket.Chat',
  googlesheets: 'Google Sheets',
  googlechat: 'Google Chat',
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
  ...uptimeKumaProviderIDs.map((id) => ({
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

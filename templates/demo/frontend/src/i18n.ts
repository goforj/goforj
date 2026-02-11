import { createI18n } from 'vue-i18n'
import en from '@/locales/en'
import es from '@/locales/es'
import fr from '@/locales/fr'
import de from '@/locales/de'
import pt from '@/locales/pt'
import ja from '@/locales/ja'
import zhCN from '@/locales/zh-CN'
import zhTW from '@/locales/zh-TW'
import koKR from '@/locales/ko-KR'
import itIT from '@/locales/it-IT'
import nlNL from '@/locales/nl-NL'
import ruRU from '@/locales/ru-RU'
import plPL from '@/locales/pl-PL'
import trTR from '@/locales/tr-TR'
import idID from '@/locales/id-ID'
import viVN from '@/locales/vi-VN'
import thTH from '@/locales/th-TH'

export const LOCALES = ['en', 'es', 'fr', 'de', 'pt', 'ja', 'zh-CN', 'zh-TW', 'ko-KR', 'it-IT', 'nl-NL', 'ru-RU', 'pl-PL', 'tr-TR', 'id-ID', 'vi-VN', 'th-TH'] as const
export type AppLocale = (typeof LOCALES)[number]
export const LOCALE_OPTIONS: ReadonlyArray<{ code: AppLocale; labelKey: string }> = [
  { code: 'en', labelKey: 'language.english' },
  { code: 'es', labelKey: 'language.spanish' },
  { code: 'fr', labelKey: 'language.french' },
  { code: 'de', labelKey: 'language.german' },
  { code: 'pt', labelKey: 'language.portuguese' },
  { code: 'ja', labelKey: 'language.japanese' },
  { code: 'zh-CN', labelKey: 'language.chineseSimplified' },
  { code: 'zh-TW', labelKey: 'language.chineseTraditional' },
  { code: 'ko-KR', labelKey: 'language.korean' },
  { code: 'it-IT', labelKey: 'language.italian' },
  { code: 'nl-NL', labelKey: 'language.dutch' },
  { code: 'ru-RU', labelKey: 'language.russian' },
  { code: 'pl-PL', labelKey: 'language.polish' },
  { code: 'tr-TR', labelKey: 'language.turkish' },
  { code: 'id-ID', labelKey: 'language.indonesian' },
  { code: 'vi-VN', labelKey: 'language.vietnamese' },
  { code: 'th-TH', labelKey: 'language.thai' },
]

const STORAGE_KEY = 'demo.locale'

function isLocale(value: string): value is AppLocale {
  return (LOCALES as readonly string[]).includes(value)
}

function detectLocale(): AppLocale {
  const saved = localStorage.getItem(STORAGE_KEY)
  const savedNormalized = String(saved || '').toLowerCase()
  if (savedNormalized === 'zh' || savedNormalized === 'zh-cn') return 'zh-CN'
  if (savedNormalized === 'zh-tw' || savedNormalized === 'zh-hk' || savedNormalized === 'zh-mo') return 'zh-TW'
  if (savedNormalized === 'ko' || savedNormalized === 'ko-kr') return 'ko-KR'
  if (savedNormalized === 'it' || savedNormalized === 'it-it') return 'it-IT'
  if (savedNormalized === 'nl' || savedNormalized === 'nl-nl') return 'nl-NL'
  if (savedNormalized === 'ru' || savedNormalized === 'ru-ru') return 'ru-RU'
  if (savedNormalized === 'pl' || savedNormalized === 'pl-pl') return 'pl-PL'
  if (savedNormalized === 'tr' || savedNormalized === 'tr-tr') return 'tr-TR'
  if (savedNormalized === 'id' || savedNormalized === 'id-id') return 'id-ID'
  if (savedNormalized === 'vi' || savedNormalized === 'vi-vn') return 'vi-VN'
  if (savedNormalized === 'th' || savedNormalized === 'th-th') return 'th-TH'
  if (saved && isLocale(saved)) return saved

  const browser = navigator.language.toLowerCase()
  if (browser.startsWith('es')) return 'es'
  if (browser.startsWith('fr')) return 'fr'
  if (browser.startsWith('de')) return 'de'
  if (browser.startsWith('pt')) return 'pt'
  if (browser.startsWith('ja')) return 'ja'
  if (browser.startsWith('ko')) return 'ko-KR'
  if (browser.startsWith('it')) return 'it-IT'
  if (browser.startsWith('nl')) return 'nl-NL'
  if (browser.startsWith('ru')) return 'ru-RU'
  if (browser.startsWith('pl')) return 'pl-PL'
  if (browser.startsWith('tr')) return 'tr-TR'
  if (browser.startsWith('id')) return 'id-ID'
  if (browser.startsWith('vi')) return 'vi-VN'
  if (browser.startsWith('th')) return 'th-TH'
  if (browser.startsWith('zh-tw') || browser.startsWith('zh-hk') || browser.startsWith('zh-mo')) return 'zh-TW'
  if (browser.startsWith('zh')) return 'zh-CN'
  return 'en'
}

function applyHtmlLang(locale: AppLocale) {
  document.documentElement.setAttribute('lang', locale)
  document.documentElement.setAttribute('translate', 'no')
  document.documentElement.classList.add('notranslate')
}

const initialLocale = detectLocale()
applyHtmlLang(initialLocale)

export const i18n = createI18n({
  legacy: false,
  locale: initialLocale,
  fallbackLocale: 'en',
  messages: {
    en,
    es,
    fr,
    de,
    pt,
    ja,
    'zh-CN': zhCN,
    'zh-TW': zhTW,
    'ko-KR': koKR,
    'it-IT': itIT,
    'nl-NL': nlNL,
    'ru-RU': ruRU,
    'pl-PL': plPL,
    'tr-TR': trTR,
    'id-ID': idID,
    'vi-VN': viVN,
    'th-TH': thTH,
  },
})

export function setLocale(locale: AppLocale) {
  i18n.global.locale.value = locale
  localStorage.setItem(STORAGE_KEY, locale)
  applyHtmlLang(locale)
}

import { createI18n } from 'vue-i18n'
import en from '@/locales/en'
import es from '@/locales/es'

export const LOCALES = ['en', 'es'] as const
export type AppLocale = (typeof LOCALES)[number]

const STORAGE_KEY = 'demo.locale'

function isLocale(value: string): value is AppLocale {
  return (LOCALES as readonly string[]).includes(value)
}

function detectLocale(): AppLocale {
  const saved = localStorage.getItem(STORAGE_KEY)
  if (saved && isLocale(saved)) return saved

  const browser = navigator.language.toLowerCase()
  if (browser.startsWith('es')) return 'es'
  return 'en'
}

function applyHtmlLang(locale: AppLocale) {
  document.documentElement.setAttribute('lang', locale)
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
  },
})

export function setLocale(locale: AppLocale) {
  i18n.global.locale.value = locale
  localStorage.setItem(STORAGE_KEY, locale)
  applyHtmlLang(locale)
}

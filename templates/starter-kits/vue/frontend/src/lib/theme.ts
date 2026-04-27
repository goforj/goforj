export type ThemePreference = 'light' | 'dark' | 'system'

const storageKey = 'theme'

function systemPrefersDark() {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function resolveTheme(preference: ThemePreference) {
  if (preference === 'system') {
    return systemPrefersDark()
  }
  return preference === 'dark'
}

export function themePreference(): ThemePreference {
  const stored = localStorage.getItem(storageKey)
  if (stored === 'light' || stored === 'dark' || stored === 'system') {
    return stored
  }
  return 'system'
}

export function applyTheme(preference: ThemePreference = themePreference()) {
  document.documentElement.classList.toggle('dark', resolveTheme(preference))
  document.documentElement.style.colorScheme = resolveTheme(preference) ? 'dark' : 'light'
}

export function setThemePreference(preference: ThemePreference) {
  localStorage.setItem(storageKey, preference)
  applyTheme(preference)
}

export function watchSystemTheme() {
  const media = window.matchMedia('(prefers-color-scheme: dark)')
  const listener = () => {
    if (themePreference() === 'system') {
      applyTheme('system')
    }
  }
  media.addEventListener('change', listener)
  return () => media.removeEventListener('change', listener)
}

import { afterEach, describe, expect, it } from 'vitest'
import { mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import { useI18n } from 'vue-i18n'
import { i18n, setLocale } from '@/i18n'

afterEach(() => {
  setLocale('en')
  localStorage.removeItem('demo.locale')
})

describe('demo translations', () => {
  it('interpolates named values in both languages', () => {
    setLocale('en')
    expect(i18n.global.t('common.lastRange', { range: '24h' })).toBe('Last 24h')
    setLocale('fr')
    expect(i18n.global.t('common.lastRange', { range: '24h' })).toBe('Derniers 24h')
  })

  it('updates mounted translations and persists the selected language', async () => {
    setLocale('en')
    const view = mount(defineComponent({
      setup() {
        const { t } = useI18n()
        return () => h('p', t('common.save'))
      },
    }), { global: { plugins: [i18n] } })
    try {
      expect(view.text()).toBe('Save')
      setLocale('fr')
      await nextTick()
      expect(view.html()).toContain('Enregistrer')
      expect(localStorage.getItem('demo.locale')).toBe('fr')
      expect(document.documentElement.lang).toBe('fr')
    } finally {
      view.unmount()
    }
  })
})

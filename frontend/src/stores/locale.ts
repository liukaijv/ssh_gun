import { ref } from 'vue'
import { defineStore } from 'pinia'
import i18n, { normalizeLocale, type SupportedLocale } from '@/i18n'
import { GetLanguage, SetLanguage } from '../../wailsjs/go/main/App'
import { WindowSetTitle } from '../../wailsjs/runtime/runtime'

function appTitle(locale: SupportedLocale): string {
  const messages = i18n.global.getLocaleMessage(locale) as { app?: { name?: string } }
  return messages?.app?.name || (locale === 'en-US' ? 'Feisuo' : '飞梭')
}

function syncWindowTitle(locale: SupportedLocale) {
  const title = appTitle(locale)
  document.title = title
  if ((window as any).runtime) {
    WindowSetTitle(title)
  }
}

export const useLocaleStore = defineStore('locale', () => {
  const locale = ref<SupportedLocale>('zh-CN')

  function apply(value: SupportedLocale) {
    locale.value = value
    i18n.global.locale.value = value
    document.documentElement.lang = value
    syncWindowTitle(value)
  }

  async function initialize() {
    try {
      apply(normalizeLocale(await GetLanguage()))
    } catch {
      apply('zh-CN')
    }
  }

  async function setLocale(value: SupportedLocale) {
    const next = normalizeLocale(value)
    apply(next)
    try {
      await SetLanguage(next)
    } catch (err) {
      // Keep UI language even if persistence fails; surface to caller.
      throw err
    }
  }

  return { locale, initialize, setLocale }
})

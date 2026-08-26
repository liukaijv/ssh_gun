import { createI18n } from 'vue-i18n'
import enUS from './locales/en-US'
import zhCN from './locales/zh-CN'

export type SupportedLocale = 'zh-CN' | 'en-US'

export function normalizeLocale(value: string): SupportedLocale {
  return value === 'en-US' ? 'en-US' : 'zh-CN'
}

const i18n = createI18n({
  legacy: false,
  locale: 'zh-CN',
  fallbackLocale: 'zh-CN',
  messages: {
    'zh-CN': zhCN,
    'en-US': enUS,
  },
})

export default i18n

import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import i18n from '@/i18n'
import { useLocaleStore } from '@/stores/locale'

const api = vi.hoisted(() => ({
  getLanguage: vi.fn(),
  setLanguage: vi.fn(),
}))

vi.mock('../../wailsjs/go/main/App', () => ({
  GetLanguage: api.getLanguage,
  SetLanguage: api.setLanguage,
}))

vi.mock('../../wailsjs/runtime/runtime', () => ({
  WindowSetTitle: vi.fn(),
}))

describe('locale store', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    api.getLanguage.mockReset()
    api.setLanguage.mockReset()
    i18n.global.locale.value = 'zh-CN'
  })

  it('loads the persisted language', async () => {
    api.getLanguage.mockResolvedValue('en-US')
    const store = useLocaleStore()
    await store.initialize()
    expect(store.locale).toBe('en-US')
    expect(i18n.global.locale.value).toBe('en-US')
  })

  it('switches immediately and persists', async () => {
    api.setLanguage.mockResolvedValue(undefined)
    const store = useLocaleStore()
    await store.setLocale('en-US')
    expect(store.locale).toBe('en-US')
    expect(i18n.global.locale.value).toBe('en-US')
    expect(api.setLanguage).toHaveBeenCalledWith('en-US')
  })
})

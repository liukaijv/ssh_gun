import { describe, expect, it } from 'vitest'
import { normalizeLocale } from '@/i18n'
import enUS from '@/i18n/locales/en-US'
import zhCN from '@/i18n/locales/zh-CN'

function leafKeys(value: Record<string, unknown>, prefix = ''): string[] {
  return Object.entries(value).flatMap(([key, child]) => {
    const path = prefix ? `${prefix}.${key}` : key
    if (child && typeof child === 'object') {
      return leafKeys(child as Record<string, unknown>, path)
    }
    return [path]
  })
}

describe('i18n', () => {
  it('normalizes supported locales and defaults to Chinese', () => {
    expect(normalizeLocale('en-US')).toBe('en-US')
    expect(normalizeLocale('zh-CN')).toBe('zh-CN')
    expect(normalizeLocale('')).toBe('zh-CN')
    expect(normalizeLocale('ja-JP')).toBe('zh-CN')
  })

  it('keeps both locale dictionaries structurally complete', () => {
    expect(leafKeys(enUS).sort()).toEqual(leafKeys(zhCN).sort())
  })
})

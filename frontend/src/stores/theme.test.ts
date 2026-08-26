import { describe, expect, it } from 'vitest'
import { resolveEffectiveTheme } from '@/stores/theme'

describe('resolveEffectiveTheme', () => {
  it('uses the explicit light or dark mode', () => {
    expect(resolveEffectiveTheme('light', true)).toBe('light')
    expect(resolveEffectiveTheme('dark', false)).toBe('dark')
  })

  it('resolves system mode from the media query', () => {
    expect(resolveEffectiveTheme('system', true)).toBe('dark')
    expect(resolveEffectiveTheme('system', false)).toBe('light')
  })
})

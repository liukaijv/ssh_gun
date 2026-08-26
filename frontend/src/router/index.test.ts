import { describe, expect, it } from 'vitest'
import router from '@/router'

describe('router', () => {
  it('provides a settings page', () => {
    expect(router.hasRoute('settings')).toBe(true)
    expect(router.resolve({ name: 'settings' }).path).toBe('/settings')
  })
})

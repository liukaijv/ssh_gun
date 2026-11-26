import { describe, expect, it } from 'vitest'
import router from '@/router'

describe('router', () => {
  it('provides a settings page', () => {
    expect(router.hasRoute('settings')).toBe(true)
    expect(router.resolve({ name: 'settings' }).path).toBe('/settings')
  })

  it('opens a mapping file browser', () => {
    expect(router.hasRoute('mapping-files')).toBe(true)
    expect(router.resolve({ name: 'mapping-files', params: { id: 'map-1' } }).path).toBe('/mappings/map-1/files')
  })
})

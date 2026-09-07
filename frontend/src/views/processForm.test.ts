import { describe, expect, it } from 'vitest'
import { canSaveProcessForm } from './processForm'

describe('canSaveProcessForm', () => {
  it('requires name and command', () => {
    expect(canSaveProcessForm({ name: 'x', command: 'memcached.exe' })).toBe(true)
    expect(canSaveProcessForm({ name: ' ', command: 'memcached.exe' })).toBe(false)
    expect(canSaveProcessForm({ name: 'x', command: '  ' })).toBe(false)
  })
})

import { describe, expect, it } from 'vitest'
import { canSaveServerForm, validateServerForm } from '@/views/serverForm'

describe('serverForm validation', () => {
  const valid = {
    name: 'dev',
    host: '127.0.0.1',
    port: 22,
    user: 'abc',
  }

  it('rejects blank required fields for create and edit payloads', () => {
    expect(validateServerForm({ ...valid, name: '  ' })).toEqual({ name: 'required' })
    expect(validateServerForm({ ...valid, host: '' })).toEqual({ host: 'required' })
    expect(validateServerForm({ ...valid, user: '\t' })).toEqual({ user: 'required' })
    expect(validateServerForm({ ...valid, port: null })).toEqual({ port: 'portRange' })
    expect(validateServerForm({ ...valid, port: 0 })).toEqual({ port: 'portRange' })
    expect(validateServerForm({ ...valid, port: 65536 })).toEqual({ port: 'portRange' })
    expect(canSaveServerForm({ ...valid, name: '' })).toBe(false)
  })

  it('allows saving when all required fields are present', () => {
    expect(validateServerForm(valid)).toEqual({})
    expect(canSaveServerForm(valid)).toBe(true)
    expect(canSaveServerForm({ ...valid, name: '  trimmed  ' })).toBe(true)
  })
})

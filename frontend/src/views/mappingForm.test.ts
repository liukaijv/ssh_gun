import { describe, expect, it } from 'vitest'
import { canSaveMappingForm, validateMappingForm } from '@/views/mappingForm'

describe('mappingForm validation', () => {
  const valid = {
    name: 'proj',
    serverId: 'srv-1',
    localPath: 'D:\\Work\\proj',
    remotePath: '/opt/proj',
  }

  it('rejects blank required fields for create and edit payloads', () => {
    expect(validateMappingForm({ ...valid, name: '  ' })).toEqual({ name: 'required' })
    expect(validateMappingForm({ ...valid, serverId: '' })).toEqual({ serverId: 'required' })
    expect(validateMappingForm({ ...valid, localPath: '\t' })).toEqual({ localPath: 'required' })
    expect(validateMappingForm({ ...valid, remotePath: '' })).toEqual({ remotePath: 'required' })
    expect(canSaveMappingForm({ ...valid, name: '' })).toBe(false)
  })

  it('allows saving when all required fields are present', () => {
    expect(validateMappingForm(valid)).toEqual({})
    expect(canSaveMappingForm(valid)).toBe(true)
    expect(canSaveMappingForm({ ...valid, name: '  trimmed  ' })).toBe(true)
  })
})

import { describe, expect, it } from 'vitest'
import { canSaveForwardForm, validateForwardForm } from '@/views/forwardForm'

describe('forwardForm validation', () => {
  const local = {
    name: 'mysql',
    serverId: 'srv-1',
    type: 'local' as const,
    localAddr: '127.0.0.1',
    localPort: 3316,
    remoteHost: '127.0.0.1',
    remotePort: 3306,
  }

  it('rejects blank shared required fields', () => {
    expect(validateForwardForm({ ...local, name: '  ' })).toEqual({ name: 'required' })
    expect(validateForwardForm({ ...local, serverId: '' })).toEqual({ serverId: 'required' })
    expect(validateForwardForm({ ...local, localPort: null })).toEqual({ localPort: 'portRange' })
    expect(canSaveForwardForm({ ...local, name: '' })).toBe(false)
  })

  it('applies type-specific remote requirements', () => {
    expect(validateForwardForm({ ...local, remoteHost: '' })).toEqual({ remoteHost: 'required' })
    expect(validateForwardForm({ ...local, remotePort: 0 })).toEqual({ remotePort: 'portRange' })
    expect(
      validateForwardForm({
        ...local,
        type: 'remote',
        localAddr: '',
      }),
    ).toEqual({ localAddr: 'required' })
    expect(
      validateForwardForm({
        name: 'socks',
        serverId: 'srv-1',
        type: 'dynamic',
        localAddr: '127.0.0.1',
        localPort: 1080,
        remoteHost: '',
        remotePort: null,
      }),
    ).toEqual({})
    expect(canSaveForwardForm(local)).toBe(true)
  })
})

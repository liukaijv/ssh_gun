import { describe, expect, it } from 'vitest'
import { canSaveProcessForm, formatProcessEnv, parseProcessEnvText } from './processForm'

describe('canSaveProcessForm', () => {
  it('requires name and command', () => {
    expect(canSaveProcessForm({ name: 'x', command: 'memcached.exe' })).toBe(true)
    expect(canSaveProcessForm({ name: ' ', command: 'memcached.exe' })).toBe(false)
    expect(canSaveProcessForm({ name: 'x', command: '  ' })).toBe(false)
  })
})

describe('parseProcessEnvText', () => {
  it('parses KEY=VALUE lines and skips blanks/comments', () => {
    const { env, error } = parseProcessEnvText(`
# comment
OPENAI_TARGET_API_URL=https://tokenhub.tencentmaas.com
FOO=bar

`)
    expect(error).toBeUndefined()
    expect(env).toEqual({
      OPENAI_TARGET_API_URL: 'https://tokenhub.tencentmaas.com',
      FOO: 'bar',
    })
  })

  it('rejects lines without KEY=', () => {
    const { error } = parseProcessEnvText('NOEQUALS')
    expect(error).toBe('line 1')
  })
})

describe('formatProcessEnv', () => {
  it('sorts keys', () => {
    expect(formatProcessEnv({ B: '2', A: '1' })).toBe('A=1\nB=2')
    expect(formatProcessEnv(undefined)).toBe('')
  })
})

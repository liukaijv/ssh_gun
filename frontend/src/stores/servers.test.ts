import { createPinia, setActivePinia } from 'pinia'
import { describe, expect, it } from 'vitest'
import type { ApiClient } from '@/api/client'
import { useServersStore } from '@/stores/servers'

describe('servers store', () => {
  it('loads servers from api client', async () => {
    setActivePinia(createPinia())
    const fake: ApiClient = {
      async listServers() {
        return [{ id: '1', name: 'dev', host: '127.0.0.1', port: 22, user: 'root', authType: 'password' }]
      },
      async testConnection() {
        return { ok: true, message: 'ok' }
      },
    }
    const store = useServersStore()
    store.setClient(fake)
    await store.refresh()
    expect(store.servers).toHaveLength(1)
    expect(store.servers[0].name).toBe('dev')
  })
})

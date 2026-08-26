import { defineStore } from 'pinia'
import { ref } from 'vue'
import type { ApiClient, ServerDTO } from '@/api/client'

export const useServersStore = defineStore('servers', () => {
  const servers = ref<ServerDTO[]>([])
  const loading = ref(false)
  let client: ApiClient | null = null

  function setClient(c: ApiClient) {
    client = c
  }

  async function refresh() {
    if (!client) throw new Error('api client not set')
    loading.value = true
    try {
      servers.value = await client.listServers()
    } finally {
      loading.value = false
    }
  }

  return { servers, loading, setClient, refresh }
})

/** API client abstraction over Wails bindings — tests inject a fake. */
export interface ServerDTO {
  id: string
  name: string
  host: string
  port: number
  user: string
  authType: 'password' | 'key'
}

export interface ApiClient {
  listServers(): Promise<ServerDTO[]>
  testConnection(id: string): Promise<{ ok: boolean; message: string }>
}

export function createWailsClient(): ApiClient {
  // Bound methods are generated later; placeholder returns empty until milestone 3.
  return {
    async listServers() {
      return []
    },
    async testConnection() {
      return { ok: false, message: 'not implemented' }
    },
  }
}

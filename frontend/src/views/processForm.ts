export type ProcessFormFields = {
  name: string
  command: string
}

export function canSaveProcessForm(fields: ProcessFormFields): boolean {
  return Boolean(fields.name.trim() && fields.command.trim())
}

/** Parse KEY=VALUE lines into a map. Blank lines and # comments are ignored. */
export function parseProcessEnvText(text: string): { env: Record<string, string>; error?: string } {
  const env: Record<string, string> = {}
  const lines = (text || '').split(/\r?\n/)
  for (let i = 0; i < lines.length; i++) {
    const raw = lines[i].trim()
    if (!raw || raw.startsWith('#')) {
      continue
    }
    const eq = raw.indexOf('=')
    if (eq <= 0) {
      return { env: {}, error: `line ${i + 1}` }
    }
    const key = raw.slice(0, eq).trim()
    const value = raw.slice(eq + 1).trim()
    if (!key) {
      return { env: {}, error: `line ${i + 1}` }
    }
    env[key] = value
  }
  return { env }
}

/** Format env map as stable KEY=VALUE lines (sorted by key). */
export function formatProcessEnv(env: Record<string, string> | null | undefined): string {
  if (!env) {
    return ''
  }
  return Object.keys(env)
    .sort()
    .map((k) => `${k}=${env[k] ?? ''}`)
    .join('\n')
}

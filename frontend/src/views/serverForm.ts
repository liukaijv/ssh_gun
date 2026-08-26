export type ServerFormFields = {
  name: string
  host: string
  port: number | null
  user: string
}

export type ServerFormErrors = Partial<Record<keyof ServerFormFields, string>>

export function validateServerForm(fields: ServerFormFields): ServerFormErrors {
  const errors: ServerFormErrors = {}
  if (!fields.name.trim()) {
    errors.name = 'required'
  }
  if (!fields.host.trim()) {
    errors.host = 'required'
  }
  if (!fields.user.trim()) {
    errors.user = 'required'
  }
  if (fields.port == null || fields.port < 1 || fields.port > 65535) {
    errors.port = 'portRange'
  }
  return errors
}

export function canSaveServerForm(fields: ServerFormFields): boolean {
  return Object.keys(validateServerForm(fields)).length === 0
}

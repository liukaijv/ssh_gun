export type ForwardType = 'local' | 'remote' | 'dynamic'

export type ForwardFormFields = {
  name: string
  serverId: string
  type: ForwardType
  localAddr: string
  localPort: number | null
  remoteHost: string
  remotePort: number | null
}

export type ForwardFormErrors = Partial<
  Record<'name' | 'serverId' | 'localAddr' | 'localPort' | 'remoteHost' | 'remotePort', string>
>

export function validateForwardForm(fields: ForwardFormFields): ForwardFormErrors {
  const errors: ForwardFormErrors = {}
  if (!fields.name.trim()) {
    errors.name = 'required'
  }
  if (!fields.serverId.trim()) {
    errors.serverId = 'required'
  }
  if (fields.localPort == null || fields.localPort < 1 || fields.localPort > 65535) {
    errors.localPort = 'portRange'
  }

  if (fields.type === 'dynamic') {
    return errors
  }

  if (fields.remotePort == null || fields.remotePort < 1 || fields.remotePort > 65535) {
    errors.remotePort = 'portRange'
  }
  if (fields.type === 'local' && !fields.remoteHost.trim()) {
    errors.remoteHost = 'required'
  }
  if (fields.type === 'remote' && !fields.localAddr.trim()) {
    errors.localAddr = 'required'
  }
  return errors
}

export function canSaveForwardForm(fields: ForwardFormFields): boolean {
  return Object.keys(validateForwardForm(fields)).length === 0
}

export type MappingFormFields = {
  name: string
  serverId: string
  localPath: string
  remotePath: string
}

export type MappingFormErrors = Partial<Record<keyof MappingFormFields, string>>

export function validateMappingForm(fields: MappingFormFields): MappingFormErrors {
  const errors: MappingFormErrors = {}
  if (!fields.name.trim()) {
    errors.name = 'required'
  }
  if (!fields.serverId.trim()) {
    errors.serverId = 'required'
  }
  if (!fields.localPath.trim()) {
    errors.localPath = 'required'
  }
  if (!fields.remotePath.trim()) {
    errors.remotePath = 'required'
  }
  return errors
}

export function canSaveMappingForm(fields: MappingFormFields): boolean {
  return Object.keys(validateMappingForm(fields)).length === 0
}

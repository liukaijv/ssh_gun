export type ProcessFormFields = {
  name: string
  command: string
}

export function canSaveProcessForm(fields: ProcessFormFields): boolean {
  return Boolean(fields.name.trim() && fields.command.trim())
}

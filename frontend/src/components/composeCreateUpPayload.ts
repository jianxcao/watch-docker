export interface ComposeCreateUpPayloadInput {
  projectName: string
  yamlContent: string
  force?: boolean
  composeFile?: string
}

export interface ComposeCreateUpPayload {
  name: string
  yamlContent: string
  force: boolean
  composeFile?: string
}

export function buildComposeCreateUpPayload(
  input: ComposeCreateUpPayloadInput,
): ComposeCreateUpPayload {
  const payload: ComposeCreateUpPayload = {
    name: input.projectName,
    yamlContent: input.yamlContent,
    force: Boolean(input.force),
  }
  const trimmedComposeFile = input.composeFile?.trim()
  if (trimmedComposeFile) {
    payload.composeFile = trimmedComposeFile
  }
  return payload
}

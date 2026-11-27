import { describe, expect, it } from 'vitest'
import { buildComposeCreateUpPayload } from './composeCreateUpPayload'

describe('buildComposeCreateUpPayload', () => {
  it('includes composeFile alongside name, yamlContent and force when redeploying', () => {
    const payload = buildComposeCreateUpPayload({
      projectName: 'demo-stack',
      composeFile: '/data/compose/demo-stack/compose.yaml',
      yamlContent: 'services:\n  web:\n    image: nginx:alpine\n',
      force: true,
    })

    expect(payload).toEqual({
      name: 'demo-stack',
      composeFile: '/data/compose/demo-stack/compose.yaml',
      yamlContent: 'services:\n  web:\n    image: nginx:alpine\n',
      force: true,
    })
  })

  it('omits composeFile when creating a new project', () => {
    const payload = buildComposeCreateUpPayload({
      projectName: 'fresh-stack',
      yamlContent: 'services:\n  app:\n    image: alpine:3.20\n',
    })

    expect(payload).toEqual({
      name: 'fresh-stack',
      yamlContent: 'services:\n  app:\n    image: alpine:3.20\n',
      force: false,
    })
    expect('composeFile' in payload).toBe(false)
  })
})

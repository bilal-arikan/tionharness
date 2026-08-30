import { describe, expect, it } from 'vitest'
import { parseRoute } from './url'

describe('navigation reshuffle compatibility', () => {
  it('routes legacy logs bookmarks to Workspace logs', () => {
    expect(parseRoute('#/w/WS5/logs')).toMatchObject({
      workspaceId: 'WS5',
      view: 'workspace',
      id: 'logs',
    })
  })

  it('routes legacy workspace files bookmarks to Prompts', () => {
    expect(parseRoute('#/w/WS5/workspace/files')).toMatchObject({
      workspaceId: 'WS5',
      view: 'prompts',
      id: null,
    })
  })
})

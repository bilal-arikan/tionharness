import { describe, expect, it } from 'vitest'
import { buildRoute, parseRoute, routeIdForView } from './url'

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

describe('explorer focus deep-link contract', () => {
  it('round-trips encoded refs without changing their bytes', () => {
    const path = buildRoute({
      workspaceId: 'WS 5',
      view: 'explorer',
      id: 'category:col:in progress#özel',
    })

    expect(path).toBe('/w/WS%205/explorer/category%3Acol%3Ain%20progress%23%C3%B6zel')
    expect(parseRoute('#' + path)).toMatchObject({
      workspaceId: 'WS 5',
      view: 'explorer',
      id: 'category:col:in progress#özel',
    })
  })

  it('keeps a root focus out of the URL', () => {
    expect(
      routeIdForView('explorer', {
        sessionId: null,
        agentId: null,
        artifactId: null,
        scheduleId: null,
        settingsCat: null,
        workspaceTab: null,
        insightTab: null,
        flowsTab: null,
        explorerNode: null,
      }),
    ).toBeNull()
  })
})

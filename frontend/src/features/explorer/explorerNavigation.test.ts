import { describe, expect, it } from 'vitest'
import type { ViewRef } from '@/types'
import { screenForRef } from './explorerNavigation'

const ref = (kind: ViewRef['kind'], id: string, sub?: string): ViewRef => ({ kind, id, sub })

describe('screenForRef', () => {
  it('sends entities to their own screen with a pre-selection where the screen supports one', () => {
    expect(screenForRef(ref('session', 'SES1'))).toEqual({
      view: 'chat',
      id: 'SES1',
      label: 'Sohbet',
    })
    expect(screenForRef(ref('agent', 'AGT1'))).toMatchObject({ view: 'agents', id: 'AGT1' })
    expect(screenForRef(ref('artifact', 'ART1'))).toMatchObject({ view: 'artifacts', id: 'ART1' })
    expect(screenForRef(ref('automation', 'AUT1'))).toMatchObject({ view: 'schedules', id: 'AUT1' })
    expect(screenForRef(ref('schedule', 'SCH1'))).toMatchObject({ view: 'schedules', id: 'SCH1' })
    expect(screenForRef(ref('trajectory', 'RTA1'))).toMatchObject({ view: 'rota', id: 'RTA1' })
  })

  it('sends hubs, groups and leaves without a deep link to the screen itself', () => {
    expect(screenForRef(ref('workspace', 'workspace'))).toMatchObject({
      view: 'workspace',
      id: null,
    })
    expect(screenForRef(ref('category', 'sessions'))).toMatchObject({ view: 'chat', id: null })
    expect(screenForRef(ref('category', 'flows'))).toMatchObject({ view: 'flows' })
    expect(screenForRef(ref('category', 'col:done'))).toMatchObject({ view: 'board' })
    expect(screenForRef(ref('category', 'skind:chat'))).toMatchObject({ view: 'chat', id: null })
    expect(screenForRef(ref('board', 'board'))).toMatchObject({ view: 'board', id: null })
    expect(screenForRef(ref('board', 'board', 'T1'))).toMatchObject({ view: 'board', id: 'T1' })
    expect(screenForRef(ref('flowrun', 'RUN1'))).toMatchObject({ view: 'flows', id: null })
    expect(screenForRef(ref('skill', 'x'))).toMatchObject({ view: 'skills', id: null })
    expect(screenForRef(ref('insight', 'FND1'))).toMatchObject({ view: 'insights', id: null })
    expect(screenForRef(ref('budget', 'budget', 'provider:openai'))).toMatchObject({
      view: 'budget',
    })
    expect(screenForRef(ref('tools', 'tools', 'mcp:M1'))).toMatchObject({ view: 'tools', id: null })
    expect(screenForRef(ref('tools', 'tools', 'group:files'))).toMatchObject({
      view: 'tools',
      id: 'files',
    })
    expect(screenForRef(ref('logs', 'logs'))).toEqual({
      view: 'workspace',
      id: 'logs',
      label: 'Workspace',
    })
  })

  it('returns null for an unknown category', () => {
    expect(screenForRef(ref('category', 'nope'))).toBeNull()
  })
})

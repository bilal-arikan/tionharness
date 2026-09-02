import { describe, expect, it } from 'vitest'
import { SIGNAL_SKILLS, signalsForEvent } from './eventToRefreshSignals'
import { viewForEventType } from './eventViews'
import type { AppEvent } from '@/types'

const skillsEvent: AppEvent = {
  type: 'skills',
  level: 'info',
  workspaceId: 'WS1',
  title: 'Beceriler değişti',
  body: '',
  time: 1,
}

describe('skills change event routing', () => {
  it('routes the control event to the Skills unread nav indicator', () => {
    expect(viewForEventType('skills')).toBe('skills')
  })

  it('refreshes only Skills consumers', () => {
    expect(signalsForEvent(skillsEvent)).toEqual([SIGNAL_SKILLS])
  })
})

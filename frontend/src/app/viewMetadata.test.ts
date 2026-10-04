import { describe, expect, it } from 'vitest'
import { NAV, PINNED_NAV } from './navItems'
import { HEADERLESS_VIEWS, VIEW_TITLE } from './viewRegistry'

describe('navigation destinations', () => {
  it('preserves primary rail order and exposes each destination once on mobile', () => {
    expect(NAV.map((item) => item.key)).toEqual([
      'dashboard',
      'chat',
      'agents',
      'rota',
      'explorer',
      'board',
      'schedules',
      'flows',
      'artifacts',
      'skills',
      'tools',
      'market',
      'budget',
      'prompts',
      'insights',
      'notes',
    ])
    expect(PINNED_NAV.map((item) => item.key)).toEqual(['workspace', 'settings'])
    const destinations = [...NAV, ...PINNED_NAV].map((item) => item.key)
    expect(new Set(destinations).size).toBe(destinations.length)
    expect(destinations.every((key) => key in VIEW_TITLE)).toBe(true)
  })

  it('preserves the command palette order and headers for the chat and settings destinations', () => {
    expect(Object.keys(VIEW_TITLE)).toEqual([
      'dashboard',
      'chat',
      'agents',
      'rota',
      'explorer',
      'board',
      'schedules',
      'flows',
      'artifacts',
      'skills',
      'tools',
      'budget',
      'prompts',
      'insights',
      'market',
      'notes',
      'workspace',
      'settings',
    ])
    expect(
      [...NAV, ...PINNED_NAV]
        .filter((item) => !HEADERLESS_VIEWS.has(item.key))
        .map((item) => item.key),
    ).toEqual(['chat', 'workspace', 'settings'])
  })
})

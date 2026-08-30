import { describe, expect, it } from 'vitest'
import { NAV } from './navItems'

describe('primary navigation', () => {
  it('contains Prompts and no standalone Logs item', () => {
    expect(
      NAV.some((item) => item.key === 'prompts' && item.labelKey === 'navigation.promptsFiles'),
    ).toBe(true)
    expect(NAV.map((item) => String(item.key))).not.toContain('logs')
  })
})

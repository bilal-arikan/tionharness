import { describe, expect, it } from 'vitest'
import type { AppSettings } from '@/types'
import { categoryPatch, dirtyCategories, rebaseSettingsDraft } from './settingsFields'

const original = {
  userName: 'Ada',
  keepAwake: false,
  enableShell: false,
  desktopNotifications: false,
  toolGuardWarnings: true,
  anthropicWebTools: false,
} as AppSettings

describe('settings draft ownership', () => {
  it('marks only the categories with editable changes', () => {
    const draft = { ...original, userName: 'Grace', anthropicWebTools: true }
    expect([...dirtyCategories(draft, original)]).toEqual(['profile', 'providers'])
    expect(categoryPatch('profile', draft, original)).toEqual({ userName: 'Grace' })
    expect(categoryPatch('reference', draft, original)).toEqual({})
  })

  it('does not write settings owned by another screen', () => {
    const draft = { ...original, toolGuardWarnings: false }
    expect(dirtyCategories(draft, original).size).toBe(0)
  })

  it('keeps another category dirty while incorporating fresh server values', () => {
    const draft = { ...original, userName: 'Grace', keepAwake: true }
    const submitted = { userName: 'Grace' }
    const updated = { ...original, ...submitted, toolGuardWarnings: false }
    const next = rebaseSettingsDraft(draft, original, submitted, updated)
    expect(next).toMatchObject({ userName: 'Grace', keepAwake: true, toolGuardWarnings: false })
    expect([...dirtyCategories(next, updated)]).toEqual(['general'])
  })

  it('preserves typing and reverting to the original value during a save', () => {
    const submitted = { userName: 'Grace' }
    const updated = { ...original, ...submitted }
    expect(
      rebaseSettingsDraft({ ...original, userName: 'Lin' }, original, submitted, updated).userName,
    ).toBe('Lin')
    expect(rebaseSettingsDraft(original, original, submitted, updated).userName).toBe('Ada')
  })

  it('updates immediate notifications without committing or losing a form draft', () => {
    const current = { ...original, enableShell: true }
    const updated = { ...original, desktopNotifications: true }
    const next = rebaseSettingsDraft(current, original, { desktopNotifications: true }, updated)
    expect(next.desktopNotifications).toBe(true)
    expect([...dirtyCategories(next, updated)]).toEqual(['tools'])
  })
})

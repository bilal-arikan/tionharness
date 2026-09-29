import { afterEach, describe, expect, it } from 'vitest'
import { i18next } from '@/i18n'
import { STEP_KINDS } from '@/shared/stepKinds'
import { DELETED_AGENT_LABEL, deletedAgentLabel } from './agentLookup'
import { coordinationLabel } from './coordination'
import { NOTIFY_TYPES } from './notifyTypes'
import { phaseLook } from './searchIndexState'
import { THEME_COLORS } from './themePresets'

afterEach(async () => {
  await i18next.changeLanguage('tr')
})

describe('shared UI localization', () => {
  it('resolves module-level catalog entries in the active language at access time', async () => {
    await i18next.changeLanguage('tr')
    expect(NOTIFY_TYPES[0].label).toBe('Sohbet yanıtı')
    expect(THEME_COLORS[0].label).toBe('Mor')
    expect(STEP_KINDS[0].label).toBe('Metin')
    expect(phaseLook('ready').label).toBe('hazır')
    expect(coordinationLabel({ coordinatorMode: true })).toBe('Koordinatör')
    expect(deletedAgentLabel()).toBe('Silinmiş ajan')
    expect(DELETED_AGENT_LABEL).toBe('Silinmiş ajan')

    await i18next.changeLanguage('en')
    expect(NOTIFY_TYPES[0].label).toBe('Chat reply')
    expect(THEME_COLORS[0].label).toBe('Violet')
    expect(STEP_KINDS[0].label).toBe('Text')
    expect(phaseLook('ready').label).toBe('ready')
    expect(coordinationLabel({ coordinatorMode: true })).toBe('Coordinator')
    expect(deletedAgentLabel()).toBe('Deleted agent')
    expect(DELETED_AGENT_LABEL).toBe('Deleted agent')
  })
})

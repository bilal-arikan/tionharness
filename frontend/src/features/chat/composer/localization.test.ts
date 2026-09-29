import { afterAll, describe, expect, it } from 'vitest'
import { i18next } from '@/i18n'
import { PERMISSION_OPTIONS, THINKING_OPTIONS } from './pickerOptions'
import { STT_LANGUAGES } from './sttLanguages'

const initialLanguage = i18next.language

describe.sequential('chat control localization', () => {
  afterAll(async () => {
    await i18next.changeLanguage(initialLanguage)
  })

  it('updates module-level picker copy without changing machine values', async () => {
    const thinking = THINKING_OPTIONS.find((option) => option.value === 'off')!
    const permission = PERMISSION_OPTIONS.find((option) => option.value === 'read-only')!

    await i18next.changeLanguage('en')
    expect([thinking.value, thinking.label, permission.value, permission.label]).toEqual([
      'off',
      'Off',
      'read-only',
      'Read-only',
    ])

    await i18next.changeLanguage('tr')
    expect([thinking.value, thinking.label, permission.value, permission.label]).toEqual([
      'off',
      'Kapalı',
      'read-only',
      'Salt-okunur',
    ])
  })

  it('keeps speech language codes stable while translating their hints', async () => {
    const german = STT_LANGUAGES.find((option) => option.value === 'de-DE')!

    await i18next.changeLanguage('en')
    expect([german.value, german.label, german.hint]).toEqual(['de-DE', 'Deutsch', 'German'])

    await i18next.changeLanguage('tr')
    expect([german.value, german.label, german.hint]).toEqual(['de-DE', 'Deutsch', 'Almanca'])
  })
})

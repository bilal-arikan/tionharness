import { afterEach, describe, expect, it } from 'vitest'
import { i18next } from '@/i18n'
import { APP_CATS } from './settingsCats'

afterEach(async () => {
  await i18next.changeLanguage('en')
})

describe('settings category localization', () => {
  it('resolves static category metadata against the active language', async () => {
    const profile = APP_CATS.find((category) => category.key === 'profile')

    await i18next.changeLanguage('en')
    expect(profile?.label).toBe('Profile')

    await i18next.changeLanguage('tr')
    expect(profile?.label).toBe('Profil')
  })
})

import { afterEach, describe, expect, it } from 'vitest'
import { setLocale } from '@/i18n'
import { laneLiveLabel } from './rotaLabels'
import { formatSegments } from './rotaSegments'

describe('Rota localization', () => {
  afterEach(async () => {
    await setLocale('tr')
  })

  it('updates helper labels with the active UI language', async () => {
    await setLocale('en')
    expect(laneLiveLabel({ state: 'queued', waiting: 2 })).toBe('queued +2')
    expect(
      formatSegments(
        [
          { start: 0, end: 3600 },
          { start: 7200, end: 10800 },
        ],
        0,
      ),
    ).toBe('2 sittings · 2 hr 0 min working · 1 hr 0 min idle')

    await setLocale('tr')
    expect(laneLiveLabel({ state: 'queued', waiting: 2 })).toBe('kuyrukta +2')
  })
})

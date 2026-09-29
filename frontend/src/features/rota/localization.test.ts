import { afterEach, describe, expect, it } from 'vitest'
import { setLocale } from '@/i18n'
import { laneLiveLabel } from './rotaLabels'
import { formatSegments } from './rotaSegments'
import { fmtDurationSec } from './trajectoryFormat'
import { trajectoryStatusLabel } from './trajectoryStatus'

describe('Rota localization', () => {
  afterEach(async () => {
    await setLocale('tr')
  })

  it('updates helper and status labels with the active UI language', async () => {
    await setLocale('en')
    expect(trajectoryStatusLabel('running')).toBe('running')
    expect(laneLiveLabel({ state: 'queued', waiting: 2 })).toBe('queued +2')
    expect(fmtDurationSec(4500)).toBe('1 hr 15 min')
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
    expect(trajectoryStatusLabel('running')).toBe('sürüyor')
    expect(laneLiveLabel({ state: 'queued', waiting: 2 })).toBe('kuyrukta +2')
    expect(fmtDurationSec(4500)).toBe('1 sa 15 dk')
  })
})

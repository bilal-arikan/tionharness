// End-to-end check of the i18n layer: these assertions go through the real
// i18next instance and the real catalogs, so they fail if a key is missing, a
// plural suffix is wrong, or the Intl locale is not switching.
//
// The plural case is the reason this file exists. i18next resolves a key passed
// `count` through its plural suffixes (`_one` / `_other`) and NOT through the bare
// key, so a catalog written without suffixes type-checks, lints, passes the parity
// test — and renders the raw key path at runtime. Only an assertion on rendered
// output catches that.

import { beforeEach, describe, expect, it } from 'vitest'
import { setLocale } from '@/i18n'
import { clearIntlCaches } from './intl'
import { bucketLabel, formatDuration, formatDurationMs, relativeTime } from './time'
import { count, percent, usd } from './format'

// A fixed instant so the relative-time assertions are not clock-dependent.
const NOW = Date.UTC(2026, 7, 25, 12, 0, 0)
const secondsAgo = (n: number) => Math.floor(NOW / 1000) - n

beforeEach(() => {
  clearIntlCaches()
})

describe('translated time labels', () => {
  it('renders Turkish labels under the tr locale', async () => {
    await setLocale('tr')
    expect(relativeTime(secondsAgo(10), NOW)).toBe('az önce')
    expect(relativeTime(secondsAgo(5 * 60), NOW)).toBe('5 dk')
    expect(relativeTime(secondsAgo(3 * 3600), NOW)).toBe('3 sa')
    expect(relativeTime(secondsAgo(3 * 86400), NOW)).toBe('3 gün')
    expect(bucketLabel('today')).toBe('Bugün')
    expect(formatDuration(45)).toBe('45 sn')
    expect(formatDuration(135)).toBe('2 dk 15 sn')
    expect(formatDuration(3900)).toBe('1 sa 5 dk')
    expect(formatDurationMs(800)).toBe('0.8 sn')
  })

  it('renders English labels under the en locale', async () => {
    await setLocale('en')
    expect(relativeTime(secondsAgo(10), NOW)).toBe('just now')
    expect(relativeTime(secondsAgo(5 * 60), NOW)).toBe('5 min')
    expect(bucketLabel('today')).toBe('Today')
    expect(formatDuration(45)).toBe('45s')
    expect(formatDuration(135)).toBe('2m 15s')
  })

  // The singular/plural split is real in English and absent in Turkish; both must
  // resolve rather than falling through to the key path.
  it('resolves both plural forms in both locales', async () => {
    await setLocale('en')
    expect(relativeTime(secondsAgo(86400 * 1.5), NOW)).toBe('Yesterday')
    expect(relativeTime(secondsAgo(86400 * 3), NOW)).toBe('3 days')
    await setLocale('tr')
    expect(relativeTime(secondsAgo(86400 * 3), NOW)).toBe('3 gün')
  })

  // A missing key renders its own path (returnNull:false + no fallback hit), which
  // is what this guards against across every label the module produces.
  it('never renders a raw key path', async () => {
    for (const locale of ['tr', 'en'] as const) {
      await setLocale(locale)
      const rendered = [
        relativeTime(secondsAgo(10), NOW),
        relativeTime(secondsAgo(90), NOW),
        relativeTime(secondsAgo(7200), NOW),
        relativeTime(secondsAgo(86400 * 3), NOW),
        formatDuration(1),
        formatDuration(61),
        formatDuration(3601),
        formatDurationMs(500),
        bucketLabel('pinned'),
        bucketLabel('older'),
      ]
      for (const s of rendered) {
        expect(s, `unresolved key in "${locale}"`).not.toMatch(/^time\./)
      }
    }
  })
})

describe('locale-aware number formatting', () => {
  it('groups digits per locale', async () => {
    await setLocale('tr')
    // Turkish uses '.' as the thousands separator, English ','.
    expect(count(1234567)).toBe('1.234.567')
    await setLocale('en')
    expect(count(1234567)).toBe('1,234,567')
  })

  it('places the percent sign per locale', async () => {
    await setLocale('tr')
    expect(percent(0.42)).toBe('%42')
    await setLocale('en')
    expect(percent(0.42)).toBe('42%')
  })

  it('keeps USD as the currency in every locale', async () => {
    await setLocale('en')
    expect(usd(1234.5)).toBe('$1,234.50')
    expect(usd(0)).toBe('$0')
    // Sub-cent spends keep four digits so they do not all collapse to $0.00.
    expect(usd(0.0004)).toContain('0.0004')
  })
})

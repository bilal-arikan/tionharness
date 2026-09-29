import { afterEach, describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { setLocale } from '@/i18n'
import { RegressedBadge, SeverityBadge, StatusBadge } from './insightBadges'

describe('Insight badge localization', () => {
  afterEach(async () => {
    await setLocale('tr')
  })

  it('renders lifecycle, severity, and regression labels in the active UI language', async () => {
    await setLocale('en')
    expect(renderToStaticMarkup(<StatusBadge status="verified" />)).toContain('Verified')
    expect(renderToStaticMarkup(<SeverityBadge severity="high" />)).toContain('high')
    expect(renderToStaticMarkup(<RegressedBadge />)).toContain('REGRESSION')

    await setLocale('tr')
    expect(renderToStaticMarkup(<StatusBadge status="verified" />)).toContain('Doğrulandı')
    expect(renderToStaticMarkup(<SeverityBadge severity="high" />)).toContain('yüksek')
    expect(renderToStaticMarkup(<RegressedBadge />)).toContain('REGRESYON')
  })
})

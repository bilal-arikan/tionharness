// @vitest-environment jsdom

import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import { i18next } from '@/i18n'
import { ArtifactView } from './ArtifactView'

describe('ArtifactView localization', () => {
  it('renders missing-media chrome in the active UI language', async () => {
    await i18next.changeLanguage('en')
    const english = renderToStaticMarkup(<ArtifactView kind="image" content="" sourcePath="" />)
    expect(english).toContain('Media file not found')

    await i18next.changeLanguage('tr')
    const turkish = renderToStaticMarkup(<ArtifactView kind="image" content="" sourcePath="" />)
    expect(turkish).toContain('Medya dosyası bulunamadı')
  })
})

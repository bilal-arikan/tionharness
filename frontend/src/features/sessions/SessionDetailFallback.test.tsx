// @vitest-environment jsdom

import { afterEach, describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { SessionDetailFallback } from './SessionDetailFallback'
import { SESSION_DETAIL_DEFAULT_WIDTH, SESSION_DETAIL_WIDTH_KEY } from './sessionDetailWidth'

afterEach(() => {
  localStorage.clear()
})

describe('SessionDetailFallback', () => {
  it('draws the panel frame at the default width around the loading message', () => {
    const html = renderToStaticMarkup(<SessionDetailFallback label="Loading session info…" />)
    expect(html).toContain(`width:${SESSION_DETAIL_DEFAULT_WIDTH}px`)
    expect(html).toContain('Loading session info…')
    expect(html).toContain('data-testid="loading-state"')
  })

  it('uses the width the user dragged the panel to', () => {
    localStorage.setItem(SESSION_DETAIL_WIDTH_KEY, '420')
    const html = renderToStaticMarkup(<SessionDetailFallback label="Loading" />)
    expect(html).toContain('width:420px')
  })

  it('ignores a stored width outside the allowed range', () => {
    localStorage.setItem(SESSION_DETAIL_WIDTH_KEY, '9000')
    const html = renderToStaticMarkup(<SessionDetailFallback label="Loading" />)
    expect(html).toContain(`width:${SESSION_DETAIL_DEFAULT_WIDTH}px`)
  })
})

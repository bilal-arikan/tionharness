// @vitest-environment jsdom

import { afterEach, describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { setLocale } from '@/i18n'
import type { Note } from '@/types'
import { NoteList } from './NoteList'

function note(over: Partial<Note>): Note {
  return {
    id: 'NOTE1',
    kind: 'lesson',
    title: 'Retry the build once',
    scope: 'workspace',
    confidence: 'inferred',
    created: 1,
    updated: Math.floor(Date.now() / 1000),
    body: 'body',
    ...over,
  }
}

describe('NoteList', () => {
  afterEach(async () => {
    await setLocale('tr')
  })

  it('renders kind, confidence, reach, recurrence and lifecycle badges per row', async () => {
    await setLocale('en')
    const html = renderToStaticMarkup(
      <NoteList
        notes={[
          note({
            id: 'A',
            kind: 'gotcha',
            confidence: 'verified',
            scope: 'agent',
            agents: ['AGT1', 'AGT2'],
            occurrences: 3,
            source: 'lesson-extractor',
          }),
          note({ id: 'B', title: 'Old rule', supersededBy: 'C', archived: true, private: true }),
        ]}
        activeId="A"
        onSelect={() => {}}
        loading={false}
        filtered={false}
      />,
    )
    expect(html).toContain('Gotcha')
    expect(html).toContain('Verified')
    expect(html).toContain('Agent · 2')
    expect(html).toContain('seen 3×')
    expect(html).toContain('lesson extractor')
    expect(html).toContain('superseded')
    expect(html).toContain('archived')
    expect(html).toContain('private')
    expect(html).toContain('aria-current="true"')
    expect(html.match(/data-testid="note-row"/g)).toHaveLength(2)
  })

  it('shows the search snippet under a hit and the right empty copy', async () => {
    await setLocale('en')
    const withSnippet = renderToStaticMarkup(
      <NoteList
        notes={[note({ id: 'A' })]}
        snippets={{ A: '…the build failed once…' }}
        activeId={null}
        onSelect={() => {}}
        loading={false}
        filtered
      />,
    )
    expect(withSnippet).toContain('the build failed once')

    const empty = renderToStaticMarkup(
      <NoteList notes={[]} activeId={null} onSelect={() => {}} loading={false} filtered={false} />,
    )
    expect(empty).toContain('No notes yet.')
    const noMatch = renderToStaticMarkup(
      <NoteList notes={[]} activeId={null} onSelect={() => {}} loading={false} filtered />,
    )
    expect(noMatch).toContain('No notes match the filter.')
  })

  it('localizes the badges in Turkish', async () => {
    await setLocale('tr')
    const html = renderToStaticMarkup(
      <NoteList
        notes={[note({ kind: 'decision', confidence: 'unverified' })]}
        activeId={null}
        onSelect={() => {}}
        loading={false}
        filtered={false}
      />,
    )
    expect(html).toContain('Karar')
    expect(html).toContain('Doğrulanmamış')
  })
})

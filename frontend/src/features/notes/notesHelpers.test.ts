import { describe, expect, it } from 'vitest'
import type { Note } from '@/types'
import {
  EMPTY_NOTE_FILTER,
  EMPTY_NOTE_WRITE,
  applyNoteFilter,
  budgetPercent,
  digestFlags,
  hasActiveNoteFilters,
  listParamsFor,
  normalizeNoteWrite,
  noteToWrite,
  parseList,
  reachOf,
  sortNotesNewest,
  toggleKind,
  validateNoteWrite,
} from './notesHelpers'

function note(over: Partial<Note>): Note {
  return {
    id: 'NOTE1',
    kind: 'lesson',
    title: 'A',
    scope: 'workspace',
    confidence: 'inferred',
    created: 1,
    updated: 1,
    body: 'body',
    ...over,
  }
}

describe('note filter → list params', () => {
  it('sends only the facets the server understands', () => {
    expect(listParamsFor(EMPTY_NOTE_FILTER)).toEqual({})
    expect(
      listParamsFor({
        ...EMPTY_NOTE_FILTER,
        kinds: ['lesson', 'gotcha'],
        scope: 'agent',
        confidence: 'verified',
        showArchived: true,
        showSuperseded: true,
      }),
    ).toEqual({ kinds: ['lesson', 'gotcha'], scope: 'agent', archived: true, retired: true })
  })

  it('reports an active filter for any facet, including the search text', () => {
    expect(hasActiveNoteFilters(EMPTY_NOTE_FILTER)).toBe(false)
    expect(hasActiveNoteFilters({ ...EMPTY_NOTE_FILTER, query: '  x' })).toBe(true)
    expect(hasActiveNoteFilters({ ...EMPTY_NOTE_FILTER, showArchived: true })).toBe(true)
  })

  it('toggles kind chips while keeping the canonical order', () => {
    expect(toggleKind([], 'work')).toEqual(['work'])
    expect(toggleKind(['work'], 'lesson')).toEqual(['lesson', 'work'])
    expect(toggleKind(['lesson', 'work'], 'work')).toEqual(['lesson'])
  })
})

describe('applyNoteFilter / sortNotesNewest', () => {
  const notes = [
    note({ id: 'A', updated: 10, confidence: 'verified' }),
    note({ id: 'B', updated: 30, archived: true }),
    note({ id: 'C', updated: 20, supersededBy: 'D' }),
    note({ id: 'D', updated: 20, title: 'Alpha', confidence: 'verified' }),
  ]

  it('hides archived and superseded notes unless asked and applies confidence client-side', () => {
    expect(applyNoteFilter(notes, EMPTY_NOTE_FILTER).map((n) => n.id)).toEqual(['A', 'D'])
    expect(
      applyNoteFilter(notes, {
        ...EMPTY_NOTE_FILTER,
        showArchived: true,
        showSuperseded: true,
      }).map((n) => n.id),
    ).toEqual(['A', 'B', 'C', 'D'])
    expect(
      applyNoteFilter(notes, { ...EMPTY_NOTE_FILTER, confidence: 'inferred' }).map((n) => n.id),
    ).toEqual([])
  })

  it('orders newest first with a stable title tie-break', () => {
    expect(sortNotesNewest(notes).map((n) => n.id)).toEqual(['B', 'C', 'D', 'A'])
  })
})

describe('reach and lists', () => {
  it('lists the agents or projects a scoped note reaches', () => {
    expect(reachOf(note({ scope: 'agent', agents: ['AGT1'], projects: ['/p'] }))).toEqual(['AGT1'])
    expect(reachOf(note({ scope: 'project', projects: ['/p'] }))).toEqual(['/p'])
    expect(reachOf(note({ scope: 'workspace', agents: ['AGT1'] }))).toEqual([])
  })

  it('splits comma/newline lists and drops blanks and duplicates', () => {
    expect(parseList(' a, b\nc,,a ')).toEqual(['a', 'b', 'c'])
    expect(parseList('')).toEqual([])
  })
})

describe('note form validation', () => {
  it('requires title and body', () => {
    expect(validateNoteWrite(EMPTY_NOTE_WRITE)).toEqual(['title', 'body'])
    expect(validateNoteWrite({ ...EMPTY_NOTE_WRITE, title: 'T', body: 'B' })).toEqual([])
  })

  it('requires the reach list its scope needs and a verification for verified notes', () => {
    const base = { ...EMPTY_NOTE_WRITE, title: 'T', body: 'B' }
    expect(validateNoteWrite({ ...base, scope: 'agent' })).toEqual(['agents'])
    expect(validateNoteWrite({ ...base, scope: 'project' })).toEqual(['projects'])
    expect(validateNoteWrite({ ...base, confidence: 'verified' })).toEqual(['verification'])
    expect(
      validateNoteWrite({ ...base, scope: 'agent', agents: ['AGT1'], confidence: 'verified' }),
    ).toEqual(['verification'])
  })

  it('normalizes the payload to what the scope and confidence keep', () => {
    expect(
      normalizeNoteWrite({
        ...EMPTY_NOTE_WRITE,
        title: ' T ',
        body: 'B',
        scope: 'workspace',
        agents: ['AGT1'],
        projects: ['/p'],
        confidence: 'inferred',
        verification: 'ran tests',
      }),
    ).toMatchObject({ title: 'T', agents: [], projects: [], verification: '' })
  })

  it('round-trips a stored note into the form shape', () => {
    const n = note({
      kind: 'decision',
      scope: 'agent',
      agents: ['AGT1'],
      confidence: 'verified',
      verification: 'checked',
      tags: ['x'],
      private: true,
    })
    expect(noteToWrite(n)).toEqual({
      kind: 'decision',
      title: 'A',
      body: 'body',
      scope: 'agent',
      agents: ['AGT1'],
      projects: [],
      confidence: 'verified',
      verification: 'checked',
      tags: ['x'],
      private: true,
    })
  })
})

describe('awareness helpers', () => {
  it('reports budget use as a clamped percentage and 0 for an unlimited budget', () => {
    expect(budgetPercent(512, 1024)).toBe(50)
    expect(budgetPercent(2048, 1024)).toBe(100)
    expect(budgetPercent(100, 0)).toBe(0)
  })

  it('flags digests with errors, open loops and a note suggestion', () => {
    const base = { sessionId: 'S', title: 't', at: 1, line: 'l', hash: 'h' }
    expect(digestFlags(base)).toEqual([])
    expect(digestFlags({ ...base, errors: 2, openLoops: 1, suggestNote: true })).toEqual([
      'errors',
      'openLoops',
      'suggestNote',
    ])
  })
})

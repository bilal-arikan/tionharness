// @vitest-environment jsdom
//
// Render smoke test for the Notes screen: every sub-page mounts against a mocked
// API, the list → detail → expand chain fires the right calls, and the Digests /
// Context tabs fetch and show their verbatim text blocks.

import { act } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AwarenessSeen, Digest, DigestIndexEntry, Note, NoteExpansion } from '@/types'
import { setLocale } from '@/i18n'
import { render, query } from '@/test/render'

const calls = vi.hoisted(() => ({
  noteStats: vi.fn(),
  listAgents: vi.fn(),
  listNotes: vi.fn(),
  searchNotes: vi.fn(),
  getNote: vi.fn(),
  expandNote: vi.fn(),
  listDigests: vi.fn(),
  sessionDigest: vi.fn(),
  listSessions: vi.fn(),
  sessionAwareness: vi.fn(),
}))

vi.mock('@/api', () => ({ api: calls }))

import { NotesPanel } from './NotesPanel'

const now = Math.floor(Date.now() / 1000)

const lesson: Note = {
  id: 'NOTE1',
  kind: 'lesson',
  title: 'Warm the cache first',
  scope: 'workspace',
  confidence: 'verified',
  verification: 'reproduced twice',
  created: now - 100,
  updated: now - 50,
  source: 'lesson-extractor',
  occurrences: 4,
  body: 'Run `make warm` before the suite. See [[Build flakiness]].',
  links: ['Build flakiness'],
}
const decision: Note = {
  id: 'NOTE2',
  kind: 'decision',
  title: 'Build flakiness',
  scope: 'agent',
  agents: ['AGT1'],
  confidence: 'inferred',
  created: now - 10,
  updated: now - 5,
  body: 'Retries are capped at one.',
}
const expansion: NoteExpansion = {
  note: lesson,
  links: [decision],
  backlinks: [],
}
const digestEntry: DigestIndexEntry = {
  sessionId: 'SES1',
  agentId: 'AGT1',
  title: 'Fix login',
  at: now - 30,
  line: 'Fix login — 12 msgs, 3 tools, 1 error',
  hash: 'h1',
  errors: 1,
  openLoops: 2,
  suggestNote: true,
}
const digest: Digest = {
  sessionId: 'SES1',
  agentName: 'Ada',
  title: 'Fix login',
  at: now - 30,
  messages: 12,
  toolCalls: 3,
  toolErrors: 1,
  todo: { total: 3, done: 1, inProgress: 1, open: ['write tests'] },
  openLoops: ['write tests', 'answer the question'],
  suggestNote: true,
  hash: 'h1',
  text: 'DIGEST TEXT VERBATIM',
}
const seen: AwarenessSeen = {
  sessionId: 'SES1',
  turns: 3,
  lastPulse: 'pulse: 2 running',
  brief: {
    moment: 'brief',
    text: 'brief body\n[meter] brief 2/3 full',
    meter: '[meter] brief 2/3 full',
    bytes: 40,
    budget: 100,
    sections: [
      { key: 'notes', bytes: 20, state: 'full', priority: 1 },
      { key: 'recent', bytes: 20, state: 'pointer', priority: 3 },
    ],
    degraded: ['recent'],
    hash: 'b',
  },
}

beforeEach(async () => {
  vi.clearAllMocks()
  await setLocale('en')
  calls.noteStats.mockResolvedValue({
    total: 2,
    active: 2,
    retired: 0,
    archived: 0,
    private: 0,
    byKind: { lesson: 1, decision: 1 },
    byScope: {},
    newest: now,
    sources: {},
  })
  calls.listAgents.mockResolvedValue([{ id: 'AGT1', name: 'Ada' }])
  calls.listNotes.mockResolvedValue([decision, lesson])
  calls.searchNotes.mockResolvedValue([])
  calls.getNote.mockResolvedValue(lesson)
  calls.expandNote.mockResolvedValue(expansion)
  calls.listDigests.mockResolvedValue([digestEntry])
  calls.sessionDigest.mockResolvedValue(digest)
  calls.listSessions.mockResolvedValue({
    items: [{ id: 'SES1', title: 'Fix login', updatedAt: now, createdAt: now }],
    total: 1,
    offset: 0,
    limit: 50,
    hasMore: false,
  })
  calls.sessionAwareness.mockResolvedValue(seen)
})

afterEach(async () => {
  await setLocale('tr')
})

async function renderPanel(selectedId: string | null = null) {
  const onError = vi.fn()
  const onSelectNote = vi.fn()
  const onOpenSession = vi.fn()
  const rendered = render(
    <NotesPanel
      onError={onError}
      selectedId={selectedId}
      onSelectNote={onSelectNote}
      onOpenSession={onOpenSession}
    />,
  )
  await act(async () => {})
  return { ...rendered, onError, onSelectNote, onOpenSession }
}

describe('NotesPanel', () => {
  it('lists notes newest first with their badges and opens the selected note with its links', async () => {
    const { container, rerender, onError, onSelectNote } = await renderPanel()
    expect(calls.listNotes).toHaveBeenCalledWith(
      expect.objectContaining({ limit: 500 }),
      expect.anything(),
    )
    const rows = [...container.querySelectorAll<HTMLButtonElement>('[data-testid="note-row"]')]
    expect(rows.map((r) => r.dataset.noteId)).toEqual(['NOTE2', 'NOTE1'])
    expect(rows[1].textContent).toContain('Lesson')
    expect(rows[1].textContent).toContain('Verified')
    expect(rows[1].textContent).toContain('seen 4×')
    expect(container.textContent).toContain('Active')

    act(() => rows[1].click())
    expect(onSelectNote).toHaveBeenCalledWith('NOTE1')

    rerender(<NotesPanel onError={onError} selectedId="NOTE1" onSelectNote={onSelectNote} />)
    await act(async () => {})
    expect(calls.expandNote).toHaveBeenCalledWith('NOTE1', expect.anything())
    const detail = query<HTMLElement>(container, '[data-testid="note-detail"]')
    expect(detail.textContent).toContain('Warm the cache first')
    expect(detail.textContent).toContain('reproduced twice')
    expect(detail.textContent).toContain('Build flakiness')
    expect(onError).not.toHaveBeenCalled()
  })

  it('switches the list to search when a query is typed', async () => {
    calls.searchNotes.mockResolvedValue([{ note: lesson, score: 2, snippet: 'make warm' }])
    const { container } = await renderPanel()
    const input = query<HTMLInputElement>(container, '[data-testid="notes-search"]')
    act(() => {
      Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value')?.set?.call(
        input,
        'warm',
      )
      input.dispatchEvent(new Event('input', { bubbles: true }))
    })
    // The search box is debounced; wait it out.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 300))
    })
    expect(calls.searchNotes).toHaveBeenCalledWith(
      'warm',
      expect.objectContaining({ limit: 100 }),
      expect.anything(),
    )
    const rows = [...container.querySelectorAll<HTMLButtonElement>('[data-testid="note-row"]')]
    expect(rows.map((r) => r.dataset.noteId)).toEqual(['NOTE1'])
    expect(rows[0].textContent).toContain('make warm')
  })

  it('shows a digest verbatim and offers to open its session', async () => {
    const { container, onOpenSession } = await renderPanel()
    act(() => query<HTMLButtonElement>(container, '[data-testid="notes-tab-digests"]').click())
    await act(async () => {})
    expect(calls.listDigests).toHaveBeenCalled()
    const row = query<HTMLButtonElement>(container, '[data-testid="digest-row"]')
    expect(row.textContent).toContain('Ada')
    expect(row.textContent).toContain('1 error')
    expect(row.textContent).toContain('2 open loops')
    expect(row.textContent).toContain('note suggested')

    act(() => row.click())
    await act(async () => {})
    expect(calls.sessionDigest).toHaveBeenCalledWith('SES1', expect.anything())
    expect(query<HTMLElement>(container, '[data-testid="digest-text"]').textContent).toBe(
      'DIGEST TEXT VERBATIM',
    )
    act(() => query<HTMLButtonElement>(container, '[data-testid="digest-open-session"]').click())
    expect(onOpenSession).toHaveBeenCalledWith('SES1')
  })

  it('shows what a session was told: meter, sections and pulse', async () => {
    const { container } = await renderPanel()
    act(() => query<HTMLButtonElement>(container, '[data-testid="notes-tab-context"]').click())
    await act(async () => {})
    expect(calls.listSessions).toHaveBeenCalled()
    const select = query<HTMLSelectElement>(container, '[data-testid="context-session"]')
    act(() => {
      Object.getOwnPropertyDescriptor(window.HTMLSelectElement.prototype, 'value')?.set?.call(
        select,
        'SES1',
      )
      select.dispatchEvent(new Event('change', { bubbles: true }))
    })
    await act(async () => {})
    expect(calls.sessionAwareness).toHaveBeenCalledWith('SES1', expect.anything())
    expect(query<HTMLElement>(container, '[data-testid="context-pulse"]').textContent).toBe(
      'pulse: 2 running',
    )
    expect(query<HTMLElement>(container, '[data-testid="composition-meter"]').textContent).toBe(
      '[meter] brief 2/3 full',
    )
    expect(container.textContent).toContain('pointer')
    expect(container.textContent).toContain('Degraded to a pointer: recent')
    expect(container.textContent).toContain('No per-turn block has been served yet.')
  })
})

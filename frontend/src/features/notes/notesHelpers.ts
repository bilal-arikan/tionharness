// Pure helpers for the Notes screen: filter state ↔ list params, client-side
// refinement, badge tones, form validation and small label mappings. Kept free
// of React so the vitest suite can cover them without a DOM.
import type { ListNotesParams } from '@/api/notes'
import type { BadgeTone } from '@/shared/components'
import type {
  CompositionSectionState,
  DigestIndexEntry,
  Note,
  NoteConfidence,
  NoteKind,
  NoteScope,
  NoteWrite,
} from '@/types'

// Display order mirrors notes.Kinds / Scopes / Confidences on the backend.
export const NOTE_KINDS: NoteKind[] = [
  'lesson',
  'decision',
  'work',
  'gotcha',
  'pattern',
  'profile',
  'reference',
]
export const NOTE_SCOPES: NoteScope[] = ['agent', 'project', 'workspace']
export const NOTE_CONFIDENCES: NoteConfidence[] = ['verified', 'inferred', 'unverified']

export interface NoteFilter {
  // Empty = every kind.
  kinds: NoteKind[]
  scope: NoteScope | ''
  confidence: NoteConfidence | ''
  showArchived: boolean
  showSuperseded: boolean
  // Free text; non-empty switches the list to /api/notes/search.
  query: string
}

export const EMPTY_NOTE_FILTER: NoteFilter = {
  kinds: [],
  scope: '',
  confidence: '',
  showArchived: false,
  showSuperseded: false,
  query: '',
}

export function hasActiveNoteFilters(f: NoteFilter): boolean {
  return (
    f.kinds.length > 0 ||
    f.scope !== '' ||
    f.confidence !== '' ||
    f.showArchived ||
    f.showSuperseded ||
    f.query.trim() !== ''
  )
}

// toggleKind adds or removes one kind chip, keeping the canonical display order.
export function toggleKind(kinds: NoteKind[], kind: NoteKind): NoteKind[] {
  const next = kinds.includes(kind) ? kinds.filter((k) => k !== kind) : [...kinds, kind]
  return NOTE_KINDS.filter((k) => next.includes(k))
}

// listParamsFor maps the filter onto the server's query. Confidence has no
// server parameter and is applied client-side by applyNoteFilter.
export function listParamsFor(f: NoteFilter): ListNotesParams {
  const p: ListNotesParams = {}
  if (f.kinds.length) p.kinds = f.kinds
  if (f.scope) p.scope = f.scope
  if (f.showArchived) p.archived = true
  if (f.showSuperseded) p.retired = true
  return p
}

export function isRetired(n: Note): boolean {
  return !!n.supersededBy
}

// applyNoteFilter refines a server page: the confidence facet (client-only) and
// a defensive archived/retired check so a stale page never shows what the
// toggles hide.
export function applyNoteFilter(notes: Note[], f: NoteFilter): Note[] {
  return notes.filter((n) => {
    if (f.confidence && n.confidence !== f.confidence) return false
    if (!f.showArchived && n.archived) return false
    if (!f.showSuperseded && isRetired(n)) return false
    return true
  })
}

// sortNotesNewest orders by last update (newest first), then by title so two
// notes written in the same second have a stable order.
export function sortNotesNewest(notes: Note[]): Note[] {
  return [...notes].sort((a, b) => b.updated - a.updated || a.title.localeCompare(b.title))
}

export function kindTone(kind: NoteKind): BadgeTone {
  switch (kind) {
    case 'lesson':
    case 'gotcha':
      return 'warning'
    case 'decision':
      return 'accent'
    case 'work':
      return 'success'
    default:
      return 'muted'
  }
}

export function confidenceTone(c: NoteConfidence): BadgeTone {
  switch (c) {
    case 'verified':
      return 'success'
    case 'inferred':
      return 'muted'
    case 'unverified':
      return 'warning'
  }
}

// reachOf lists who a note reaches beyond its scope word: the agents or project
// roots it names. A workspace note reaches everyone, so it lists nothing.
export function reachOf(n: Note): string[] {
  if (n.scope === 'agent') return n.agents ?? []
  if (n.scope === 'project') return n.projects ?? []
  return []
}

// parseList splits a comma / newline separated text field into distinct,
// trimmed entries (agents, project roots).
export function parseList(text: string): string[] {
  const out: string[] = []
  for (const part of text.split(/[,\n]/)) {
    const v = part.trim()
    if (v && !out.includes(v)) out.push(v)
  }
  return out
}

export const EMPTY_NOTE_WRITE: NoteWrite = {
  kind: 'lesson',
  title: '',
  body: '',
  scope: 'workspace',
  agents: [],
  projects: [],
  confidence: 'inferred',
  verification: '',
  tags: [],
  private: false,
}

// noteToWrite prefills the form from an existing note (edit / correct).
export function noteToWrite(n: Note): NoteWrite {
  return {
    kind: n.kind,
    title: n.title,
    body: n.body,
    scope: n.scope,
    agents: n.agents ?? [],
    projects: n.projects ?? [],
    confidence: n.confidence,
    verification: n.verification ?? '',
    tags: n.tags ?? [],
    private: !!n.private,
  }
}

export type NoteFormError = 'title' | 'body' | 'agents' | 'projects' | 'verification'

// validateNoteWrite mirrors the backend validator's required-field rules so a
// refused write is caught before the round trip: title + body always, the reach
// list its scope needs, and how a verified note was checked.
export function validateNoteWrite(w: NoteWrite): NoteFormError[] {
  const errors: NoteFormError[] = []
  if (!w.title.trim()) errors.push('title')
  if (!w.body.trim()) errors.push('body')
  if (w.scope === 'agent' && !(w.agents?.length ?? 0)) errors.push('agents')
  if (w.scope === 'project' && !(w.projects?.length ?? 0)) errors.push('projects')
  if (w.confidence === 'verified' && !(w.verification ?? '').trim()) errors.push('verification')
  return errors
}

// normalizeNoteWrite drops the reach list the scope does not use and the
// verification of a non-verified note, so the wire payload matches what the
// backend will keep.
export function normalizeNoteWrite(w: NoteWrite): NoteWrite {
  return {
    ...w,
    title: w.title.trim(),
    agents: w.scope === 'agent' ? (w.agents ?? []) : [],
    projects: w.scope === 'project' ? (w.projects ?? []) : [],
    verification: w.confidence === 'verified' ? (w.verification ?? '').trim() : '',
    tags: w.tags ?? [],
    private: !!w.private,
  }
}

export function sectionStateTone(state: CompositionSectionState): BadgeTone {
  switch (state) {
    case 'full':
      return 'success'
    case 'pointer':
      return 'warning'
    case 'cut':
    case 'dropped':
      return 'danger'
  }
}

// budgetPercent is how much of a composition's budget its text used; 0 for an
// unlimited (<= 0) budget so the bar reads "no ceiling" rather than "empty".
export function budgetPercent(bytes: number, budget: number): number {
  if (budget <= 0 || bytes <= 0) return 0
  return Math.min(100, Math.round((bytes / budget) * 100))
}

export type DigestFlag = 'errors' | 'openLoops' | 'suggestNote'

// digestFlags lists the attention markers a digest row shows, most urgent first.
export function digestFlags(e: DigestIndexEntry): DigestFlag[] {
  const flags: DigestFlag[] = []
  if ((e.errors ?? 0) > 0) flags.push('errors')
  if ((e.openLoops ?? 0) > 0) flags.push('openLoops')
  if (e.suggestNote) flags.push('suggestNote')
  return flags
}

import type { SessionInfo } from '@/types'
import { pctOf } from './sessionDetailFormat'

type Filler = SessionInfo['fillers'][number]

// The context bar uses three tones rather than a hue per bucket: what the human
// and the model said carries the accent, the fixed per-call overhead is the
// strong neutral, and tool traffic is the soft neutral.
export type ContextTone = 'overhead' | 'conversation' | 'traffic'

const TONE_ORDER: ContextTone[] = ['overhead', 'conversation', 'traffic']

const TONE_COLOR: Record<ContextTone, string> = {
  overhead: 'var(--color-text-dim)',
  conversation: 'var(--color-accent)',
  traffic: 'color-mix(in srgb, var(--color-text-dim) 45%, var(--color-bg))',
}

// Buckets sent on every call regardless of the conversation.
const OVERHEAD_ROLES = new Set(['cli-harness', 'system', 'tools', 'skills', 'lazy-tools'])
// Buckets that are the conversation itself. Machine-injected prompts
// (`auto-prompt`: wake / schedule resumes) are deliberately left out: they ride
// the user role on the wire but nobody typed them, so they must not take the
// accent and make an autonomous session read as if the user wrote it.
const CONVERSATION_ROLES = new Set(['user', 'assistant', 'summary'])

// OTHER_ROW_KEY identifies the row that folds every sub-1% bucket.
export const OTHER_ROW_KEY = 'other'

// A share below this whole-number percent of the window is too thin to read in
// the bar, so such buckets are folded together.
const MINOR_PERCENT = 1

export interface ContextRow {
  key: string
  tone: ContextTone
  tokens: number
  // Items in the bucket; for the folded row, how many buckets it holds.
  count: number
  // True when every figure behind the row is an exact or measured count.
  calibrated: boolean
  // The backend bucket this row shows; absent on the folded row.
  filler?: Filler
  // The buckets folded into this row; only set on the folded row.
  folded?: Filler[]
}

export function contextToneOf(role: string): ContextTone {
  if (OVERHEAD_ROLES.has(role)) return 'overhead'
  if (CONVERSATION_ROLES.has(role)) return 'conversation'
  return 'traffic'
}

// contextToneColor is the one colour source for a row: the bar segment and its
// legend swatch both read it, so they cannot drift apart.
export function contextToneColor(tone: ContextTone): string {
  return TONE_COLOR[tone]
}

// buildContextRows turns the backend buckets into the rows the bar and legend
// draw: grouped by tone (backend order kept within a tone), with every bucket
// under 1% of the window folded into one trailing row. A lone thin bucket keeps
// its own name: folding it would save no row and hide what it is.
export function buildContextRows(fillers: Filler[], ctxWindow: number): ContextRow[] {
  const minor = fillers.filter((f) => pctOf(f.tokens, ctxWindow) < MINOR_PERCENT)
  const fold = minor.length >= 2
  const kept = fold ? fillers.filter((f) => !minor.includes(f)) : fillers
  const rows: ContextRow[] = kept.map((f) => ({
    key: f.role,
    tone: contextToneOf(f.role),
    tokens: f.tokens,
    count: f.count,
    calibrated: f.calibrated ?? false,
    filler: f,
  }))
  rows.sort((a, b) => TONE_ORDER.indexOf(a.tone) - TONE_ORDER.indexOf(b.tone))
  if (fold) {
    rows.push({
      key: OTHER_ROW_KEY,
      tone: 'traffic',
      tokens: minor.reduce((sum, f) => sum + f.tokens, 0),
      count: minor.length,
      calibrated: minor.every((f) => f.calibrated ?? false),
      folded: minor,
    })
  }
  return rows
}

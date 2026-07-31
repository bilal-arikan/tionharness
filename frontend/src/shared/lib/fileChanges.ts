// Lifts file mutations out of an assistant turn's activity trace so they can be
// listed on their own — the bulk "all changes" popup — instead of only inline as
// individual diff cards.
//
// The extraction rules MUST match what TurnSteps renders as a DiffCard, or the
// popup's file count would disagree with the visible cards:
//   - a `diff` step: the native tool loop recorded a server-side FileDiff, so
//     the patch and the +/- counts come straight off the step;
//   - a successful `tool` step of an edit tool: the claude-cli path, where the
//     CLI applied the change itself and no FileDiff exists — the patch is
//     synthesized from the tool input (synthDiffData);
//   - errored steps are skipped: nothing reached disk.

import type { TurnStep } from '@/types'
import { isEditToolBase, synthDiffData } from './diff'

/** Lower-cased base tool name with any MCP namespace prefix removed. */
function toolBase(name: string): string {
  const i = name.lastIndexOf('__')
  return (i >= 0 ? name.slice(i + 2) : name).toLowerCase()
}

export interface FileChange {
  /** Absolute path of the mutated file, as the tool reported it. */
  path: string
  /** The mutating tool's recorded name (`Edit`, `Write`, `apply_patch`, …). */
  tool: string
  patch: string
  added: number
  removed: number
  /** The file did not exist before this change. */
  created: boolean
  /**
   * The patch was synthesized client-side from the tool input rather than
   * recorded server-side. It shows the intended replacement, with no
   * surrounding file context — worth flagging so the view is not read as a
   * full-fidelity `git diff`.
   */
  synthesized: boolean
  /**
   * The SERVER cut this step to its read-path cap, so the change is only
   * partially known until the untrimmed trace is refetched. For a recorded
   * patch that means the body is clipped while the +/- counts stay exact; for a
   * SYNTHESIZED one the cut input also makes the counts an undercount, since
   * they are derived from that same input.
   */
  truncated: boolean
  /** Turn this change belongs to. Empty when extracting a single turn's steps. */
  msgId: string
  /** Unix seconds of the turn. 0 when unknown. */
  at: number
  /** Made by a subagent nested inside the turn, not by the turn's own agent. */
  nested: boolean
}

/** Origin metadata attached to every change found in one turn's trace. */
export interface ChangeOrigin {
  msgId?: string
  at?: number
  nested?: boolean
}

/**
 * Convert one trace step to a FileChange, or null when it is not a file
 * mutation. Mirrors DiffCard's own resolution of step-vs-synthesized fields.
 */
export function stepToFileChange(step: TurnStep, origin: ChangeOrigin = {}): FileChange | null {
  if (step.isError) return null
  const base = toolBase(step.tool || '')
  if (step.kind !== 'diff' && !(step.kind === 'tool' && isEditToolBase(base))) return null
  const hasOwnPatch = !!step.patch?.trim()
  const synth = hasOwnPatch ? null : synthDiffData(base, step.input)
  const path = step.path || synth?.path || ''
  const patch = hasOwnPatch ? step.patch! : synth?.patch || ''
  // A `tool` step that yielded neither a path nor a patch is not a usable
  // change record — listing it would produce a blank row.
  if (!path && !patch) return null
  return {
    path,
    tool: step.tool || '',
    patch,
    added: step.added || synth?.added || 0,
    removed: step.removed || synth?.removed || 0,
    created: !!step.created,
    synthesized: !hasOwnPatch,
    truncated: hasOwnPatch ? !!step.patchTruncated : !!step.inputTruncated,
    msgId: origin.msgId || '',
    at: origin.at || 0,
    nested: !!origin.nested,
  }
}

/**
 * All file mutations in one turn's trace, in call order, descending into a
 * subagent step's nested trace (a subagent's edits hit the same disk).
 */
export function extractFileChanges(steps: TurnStep[], origin: ChangeOrigin = {}): FileChange[] {
  const out: FileChange[] = []
  const walk = (list: TurnStep[], nested: boolean) => {
    for (const st of list) {
      if (st.subSteps?.length) walk(st.subSteps, true)
      const change = stepToFileChange(st, { ...origin, nested: nested || origin.nested })
      if (change) out.push(change)
    }
  }
  walk(steps, false)
  return out
}

/**
 * Cheap "did this turn touch any file?" probe. Stops at the first mutation
 * instead of building the change list, so a caller that only needs to decide
 * whether to render an affordance does not pay for patch synthesis.
 */
export function hasFileChanges(steps: TurnStep[]): boolean {
  for (const st of steps) {
    // Nested steps are checked BEFORE the parent's own error flag: a subagent
    // run that ended badly can still have applied real edits along the way.
    if (st.subSteps?.length && hasFileChanges(st.subSteps)) return true
    if (st.isError) continue
    if (st.kind === 'diff') return true
    if (st.kind === 'tool' && isEditToolBase(toolBase(st.tool || ''))) return true
  }
  return false
}

/** One file with every change made to it, in chronological order. */
export interface FileGroup {
  path: string
  changes: FileChange[]
  /** Sum of the individual changes' line deltas. */
  added: number
  removed: number
  /** The file was created by the first of these changes. */
  created: boolean
  /** Any change in the group carries a server-trimmed patch. */
  truncated: boolean
}

/**
 * Group changes by target file, preserving first-touch order.
 *
 * The per-file totals are a SUM of the individual edits, not a merged patch:
 * TionSwarm stores each edit's own diff, never the file's before/after content,
 * so a real union cannot be computed. Six edits to one file therefore read as
 * "6 sequential changes", which is what the UI must say.
 */
export function groupByPath(changes: FileChange[]): FileGroup[] {
  const byPath = new Map<string, FileGroup>()
  const order: string[] = []
  for (const c of changes) {
    let g = byPath.get(c.path)
    if (!g) {
      g = { path: c.path, changes: [], added: 0, removed: 0, created: false, truncated: false }
      byPath.set(c.path, g)
      order.push(c.path)
    }
    g.changes.push(c)
    g.added += c.added
    g.removed += c.removed
    g.created = g.created || c.created
    g.truncated = g.truncated || c.truncated
  }
  return order.map((p) => byPath.get(p)!)
}

/** Roll a change list up to the headline counts shown on the chat chip. */
export function changeTotals(changes: FileChange[]): {
  files: number
  added: number
  removed: number
} {
  const files = new Set<string>()
  let added = 0
  let removed = 0
  for (const c of changes) {
    files.add(c.path)
    added += c.added
    removed += c.removed
  }
  return { files: files.size, added, removed }
}

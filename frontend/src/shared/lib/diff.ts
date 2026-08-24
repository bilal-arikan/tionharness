// Minimal unified-diff parser for inline diff rendering. Handles standard
// `diff`/```diff code blocks and the +/- line form produced by edit tools.

export type DiffLineKind = 'add' | 'del' | 'ctx' | 'meta' | 'hunk'

export interface DiffLine {
  kind: DiffLineKind
  text: string
}

export interface DiffStats {
  added: number
  removed: number
}

export interface ParsedDiff {
  lines: DiffLine[]
  stats: DiffStats
}

function classify(line: string): DiffLineKind {
  if (line.startsWith('@@')) return 'hunk'
  if (
    line.startsWith('+++') ||
    line.startsWith('---') ||
    line.startsWith('diff ') ||
    line.startsWith('index ')
  ) {
    return 'meta'
  }
  if (line.startsWith('+')) return 'add'
  if (line.startsWith('-')) return 'del'
  return 'ctx'
}

export function parseDiff(text: string): ParsedDiff {
  const lines: DiffLine[] = []
  let added = 0
  let removed = 0
  for (const raw of text.replace(/\n$/, '').split('\n')) {
    const kind = classify(raw)
    if (kind === 'add') added++
    if (kind === 'del') removed++
    lines.push({ kind, text: raw })
  }
  return { lines, stats: { added, removed } }
}

// ── Context folding ────────────────────────────────────────────────────────
//
// A patch's bulk is usually unchanged context. Collapsing long runs of it is the
// single cheapest way to shrink a large diff before any rendering trick is
// needed, so the panel view folds them behind a "… N satır" expander.

/** A half-open [start, end) run of line indices that can be hidden. */
export interface FoldRange {
  start: number
  end: number
}

// Context lines KEPT on each side of a fold, so a hunk never loses the lines
// that explain it.
const FOLD_CONTEXT = 3
// Minimum number of lines a fold must actually hide to be worth an expander —
// below this the "… 2 satır" row costs more than it saves.
const FOLD_MIN_HIDDEN = 6

/**
 * Foldable runs of unchanged context in a parsed diff. Runs at the very start /
 * end of the patch keep context only on their inner side: there is no hunk above
 * the first line nor below the last, so the outer margin is pure noise.
 */
export function foldableRanges(lines: DiffLine[]): FoldRange[] {
  const out: FoldRange[] = []
  let run = -1
  const flush = (end: number) => {
    if (run < 0) return
    const start = run === 0 ? 0 : run + FOLD_CONTEXT
    const stop = end === lines.length ? end : end - FOLD_CONTEXT
    if (stop - start >= FOLD_MIN_HIDDEN) out.push({ start, end: stop })
    run = -1
  }
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].kind === 'ctx') {
      if (run < 0) run = i
      continue
    }
    flush(i)
  }
  flush(lines.length)
  return out
}

/** One rendered row: either a diff line, or a collapsed run standing in for many. */
export type DiffRow = { kind: 'line'; index: number } | { kind: 'fold'; id: number; count: number }

/**
 * Flatten `total` lines into the rows to render, replacing each still-collapsed
 * fold range with a single marker row. `ranges` must be sorted and disjoint,
 * which foldableRanges guarantees. `expanded` holds the indices (into `ranges`)
 * the user has opened.
 */
export function diffRows(
  total: number,
  ranges: FoldRange[],
  expanded: ReadonlySet<number>,
): DiffRow[] {
  const rows: DiffRow[] = []
  let i = 0
  let r = 0
  while (i < total) {
    if (r < ranges.length && ranges[r].start === i && !expanded.has(r)) {
      rows.push({ kind: 'fold', id: r, count: ranges[r].end - ranges[r].start })
      i = ranges[r].end
      r++
      continue
    }
    if (r < ranges.length && ranges[r].start === i) r++
    rows.push({ kind: 'line', index: i })
    i++
  }
  return rows
}

/** Heuristic: does this text look like a unified diff worth special rendering? */
export function looksLikeDiff(text: string): boolean {
  return /^(@@ |diff --git |--- |\+\+\+ )/m.test(text) || /^[+-].*\n[+-]/m.test(text)
}

// synthDiff builds a unified-diff-style text from an edit/write tool's INPUT,
// for tools whose OUTPUT is only a confirmation message (as claude-cli's Edit /
// Write return — "The file … has been updated", not a diff). Edit → old_string
// as removed lines + new_string as added lines; Write → the whole content as
// added lines (a new file). Returns null when the input lacks those fields.
export function synthDiff(toolBase: string, input: unknown): string | null {
  if (!input || typeof input !== 'object') return null
  const o = input as Record<string, unknown>
  if (toolBase === 'write' || toolBase === 'write_file') {
    if (typeof o.content === 'string' && o.content.length) {
      return o.content
        .replace(/\n$/, '')
        .split('\n')
        .map((l) => '+' + l)
        .join('\n')
    }
    return null
  }
  // MultiEdit carries an `edits` array of {old_string,new_string}; synthesize one
  // patch per edit and stack them so every replacement shows in the card.
  if (toolBase === 'multiedit') {
    const edits = Array.isArray(o.edits) ? o.edits : null
    if (!edits) return null
    const parts: string[] = []
    for (const e of edits) {
      if (!e || typeof e !== 'object') continue
      const eo = e as Record<string, unknown>
      const oldS = typeof eo.old_string === 'string' ? eo.old_string : ''
      const newS = typeof eo.new_string === 'string' ? eo.new_string : ''
      if (!oldS && !newS) continue
      parts.push(lineDiff(oldS, newS))
    }
    return parts.length ? parts.join('\n') : null
  }
  if (toolBase === 'edit' || toolBase === 'edit_file') {
    const oldS = typeof o.old_string === 'string' ? o.old_string : ''
    const newS = typeof o.new_string === 'string' ? o.new_string : ''
    if (!oldS && !newS) return null
    return lineDiff(oldS, newS)
  }
  // apply_patch carries the unified diff verbatim in its `patch` field; the CLI
  // output is just a confirmation line like "patched file.go (+3/-1)".
  if (toolBase === 'apply_patch') {
    if (typeof o.patch === 'string' && o.patch.trim().length) return o.patch
    return null
  }
  return null
}

// EDIT_TOOL_BASES are the file-mutating tool names whose chat step should render
// as a prominent diff card (rather than a generic activity row).
const EDIT_TOOL_BASES = ['edit', 'edit_file', 'write', 'write_file', 'multiedit', 'apply_patch']

export function isEditToolBase(base: string): boolean {
  return EDIT_TOOL_BASES.includes(base)
}

export interface SynthDiff {
  patch: string
  added: number
  removed: number
  path: string
}

// synthDiffData builds the full diff-card payload for a file-edit tool STEP that
// arrived without a precomputed patch (the claude-cli path: the CLI applies the
// edit itself, so TionHarness never recorded a server-side FileDiff). It synthesizes
// the unified patch from the tool input (old_string/new_string, content or a
// multiedit edits[]), counts the +/- lines and extracts the target path. Returns
// null when the input is not a recognizable edit/write shape.
export function synthDiffData(toolBase: string, input: unknown): SynthDiff | null {
  const patch = synthDiff(toolBase, input)
  if (!patch) return null
  const { stats } = parseDiff(patch)
  let path = ''
  if (input && typeof input === 'object') {
    const o = input as Record<string, unknown>
    if (typeof o.file_path === 'string') path = o.file_path
    else if (typeof o.path === 'string') path = o.path
    // apply_patch: extract the first file's path from the unified diff headers.
    else if (toolBase === 'apply_patch' && typeof o.patch === 'string') {
      path = firstPatchPath(o.patch)
    }
  }
  return { patch, added: stats.added, removed: stats.removed, path }
}

// firstPatchPath extracts the target path from the first `---`/`+++` header pair
// in a unified diff (used for apply_patch's multi-file patches).
function firstPatchPath(patch: string): string {
  const m = patch.match(/^\+\+\+ [ab]\/(.+?)(?:\t|$)/m)
  if (m) return m[1]
  return ''
}

// lineDiff produces a unified-diff-style string from two blocks of text using an
// LCS of their lines: lines common to both are emitted as context (" "), lines
// only in old as removed ("-"), only in new as added ("+"). This avoids the
// naive "all old removed + all new added" that double-counts unchanged context
// lines an edit happens to include (e.g. deleting rows in the middle of a block).
export function lineDiff(oldText: string, newText: string): string {
  const a = oldText.replace(/\n$/, '').split('\n')
  const b = newText.replace(/\n$/, '').split('\n')
  const n = a.length
  const m = b.length
  // Guard against a pathologically large DP table; fall back to a block replace.
  if (n * m > 400_000) {
    return [...a.map((l) => '-' + l), ...b.map((l) => '+' + l)].join('\n')
  }
  // LCS length table (suffix DP).
  const dp: number[][] = Array.from({ length: n + 1 }, () => new Array<number>(m + 1).fill(0))
  for (let i = n - 1; i >= 0; i--) {
    for (let j = m - 1; j >= 0; j--) {
      dp[i][j] = a[i] === b[j] ? dp[i + 1][j + 1] + 1 : Math.max(dp[i + 1][j], dp[i][j + 1])
    }
  }
  const out: string[] = []
  let i = 0
  let j = 0
  while (i < n && j < m) {
    if (a[i] === b[j]) {
      out.push(' ' + a[i])
      i++
      j++
    } else if (dp[i + 1][j] >= dp[i][j + 1]) {
      out.push('-' + a[i])
      i++
    } else {
      out.push('+' + b[j])
      j++
    }
  }
  while (i < n) out.push('-' + a[i++])
  while (j < m) out.push('+' + b[j++])
  return out.join('\n')
}

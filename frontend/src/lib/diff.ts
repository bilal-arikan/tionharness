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

/** Heuristic: does this text look like a unified diff worth special rendering? */
export function looksLikeDiff(text: string): boolean {
  return /^(@@ |diff --git |--- |\+\+\+ )/m.test(text) ||
    /^[+-].*\n[+-]/m.test(text)
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
  return null
}

// EDIT_TOOL_BASES are the file-mutating tool names whose chat step should render
// as a prominent diff card (rather than a generic activity row).
const EDIT_TOOL_BASES = ['edit', 'edit_file', 'write', 'write_file', 'multiedit']

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
// edit itself, so SwarmGo never recorded a server-side FileDiff). It synthesizes
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
  }
  return { patch, added: stats.added, removed: stats.removed, path }
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

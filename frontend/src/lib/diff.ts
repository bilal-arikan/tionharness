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
  const out: string[] = []
  const push = (text: string, sign: '+' | '-') => {
    for (const line of text.replace(/\n$/, '').split('\n')) out.push(sign + line)
  }
  if (toolBase === 'write' || toolBase === 'write_file') {
    if (typeof o.content === 'string' && o.content.length) push(o.content, '+')
  } else if (toolBase === 'edit' || toolBase === 'edit_file' || toolBase === 'multiedit') {
    if (typeof o.old_string === 'string' && o.old_string.length) push(o.old_string, '-')
    if (typeof o.new_string === 'string' && o.new_string.length) push(o.new_string, '+')
  }
  return out.length ? out.join('\n') : null
}

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

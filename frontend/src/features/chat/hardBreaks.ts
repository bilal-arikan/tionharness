// Helpers that prepare a PLAIN-TEXT message (an agent-authored brief) for the
// markdown renderer without changing what it says. Such text uses markdown on
// purpose (lists, code spans, fences) but is not written with markdown's
// whitespace and escape rules in mind:
//   - single newlines separate lines, which CommonMark would join into one
//     paragraph → keepLineBreaks;
//   - backslashes are literal (Windows paths), which CommonMark would consume
//     as escapes before punctuation → keepBackslashes.
// Fenced code blocks are left untouched by both.
const FENCE = /^(\s*)(`{3,}|~{3,})/
// Lines that already open a block (list item, heading, quote, table row):
// markdown separates those on its own.
const BLOCK_START = /^\s*([-*+]\s|\d+[.)]\s|#{1,6}\s|>|\|)/
// A backslash escapes the next character only when that is ASCII punctuation.
const ESCAPING_BACKSLASH = /\\(?=[!-/:-@[-`{-~])/g
// An inline code span: a backtick run, its content, the closing run.
const CODE_SPAN = /(`+[^`\n]*`+)/

// mapOutsideFences runs `fn` over every line that is not part of a fenced code
// block, passing the following line for look-ahead.
function mapOutsideFences(text: string, fn: (line: string, next?: string) => string): string {
  const lines = text.split('\n')
  let fence: string | null = null
  return lines
    .map((line, i) => {
      const m = FENCE.exec(line)
      if (m) {
        if (!fence) fence = m[2][0]
        else if (m[2][0] === fence) fence = null
        return line
      }
      return fence ? line : fn(line, lines[i + 1])
    })
    .join('\n')
}

// keepLineBreaks turns a single newline between two plain lines into a
// markdown hard break (two trailing spaces).
export function keepLineBreaks(text: string): string {
  return mapOutsideFences(text, (line, next) => {
    const joinable =
      line.trim() !== '' &&
      next !== undefined &&
      next.trim() !== '' &&
      !FENCE.test(next) &&
      !BLOCK_START.test(next)
    return joinable ? `${line.replace(/\s+$/, '')}  ` : line
  })
}

// keepBackslashes doubles every backslash markdown would read as an escape, so
// it survives as a literal backslash. Code spans already keep theirs.
export function keepBackslashes(text: string): string {
  return mapOutsideFences(text, (line) =>
    line
      .split(CODE_SPAN)
      // Odd segments are the captured code spans.
      .map((part, i) => (i % 2 ? part : part.replace(ESCAPING_BACKSLASH, '\\\\')))
      .join(''),
  )
}

// plainTextToMarkdown applies both; they do not interact.
export function plainTextToMarkdown(text: string): string {
  return keepLineBreaks(keepBackslashes(text))
}

import { describe, expect, it } from 'vitest'
import { keepBackslashes, keepLineBreaks, plainTextToMarkdown } from './hardBreaks'

describe('keepLineBreaks', () => {
  it('adds a hard break between consecutive plain lines', () => {
    expect(keepLineBreaks('Context: a\nEnvironment: b')).toBe('Context: a  \nEnvironment: b')
  })

  it('leaves paragraph gaps and block starts alone', () => {
    const text = 'Todo:\n1. first\n2. second\n\nDone.'
    expect(keepLineBreaks(text)).toBe(text)
  })

  it('does not touch fenced code', () => {
    const text = 'Run:\n```sh\nline one\nline two\n```\nafter'
    expect(keepLineBreaks(text)).toBe(text)
  })
})

describe('keepBackslashes', () => {
  it('doubles only the backslashes markdown would consume', () => {
    // "\U" and "\B" are literal already; "\." would lose its backslash.
    expect(keepBackslashes(String.raw`C:\Users\Bilal\.config`)).toBe(
      String.raw`C:\Users\Bilal\\.config`,
    )
  })

  it('leaves code spans and fences alone', () => {
    const text = ['run `dir C:\\.cfg` then', '```', 'cd C:\\.cfg', '```'].join('\n')
    expect(keepBackslashes(text)).toBe(text)
  })
})

describe('plainTextToMarkdown', () => {
  it('keeps both the line structure and the path', () => {
    expect(plainTextToMarkdown(String.raw`Workspace: C:\a\.b` + '\nNext line')).toBe(
      String.raw`Workspace: C:\a\\.b` + '  \nNext line',
    )
  })
})

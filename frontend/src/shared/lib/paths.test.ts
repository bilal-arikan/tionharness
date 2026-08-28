import { describe, expect, it } from 'vitest'
import { displayPath, isUrl, mediaUrl, shortPath, splitPaths, urlHref } from './paths'

describe('displayPath', () => {
  it('collapses a Windows home path to ~\\', () => {
    expect(displayPath('C:\\Users\\bilal\\Desktop\\Projects\\TionHarness')).toBe(
      '~\\Desktop\\Projects\\TionHarness',
    )
  })

  it('collapses a POSIX home path to ~/', () => {
    expect(displayPath('/home/bilal/projects/app.go')).toBe('~/projects/app.go')
  })

  it('collapses a Git Bash mounted drive home path to ~/', () => {
    expect(displayPath('/c/Users/bilal/Desktop/Projects/TionHarness')).toBe(
      '~/Desktop/Projects/TionHarness',
    )
  })

  it('leaves a path outside the home directory untouched', () => {
    expect(displayPath('/c/Program Files/Git/bin/bash.exe')).toBe(
      '/c/Program Files/Git/bin/bash.exe',
    )
  })

  it('leaves a relative path untouched', () => {
    expect(displayPath('internal/agent/runtime.go')).toBe('internal/agent/runtime.go')
  })
})

describe('shortPath', () => {
  it('tail-truncates a long Git Bash home path', () => {
    expect(shortPath('/c/Users/bilal/Desktop/Projects/TionHarness/internal/agent/runtime.go')).toBe(
      '…/internal/agent/runtime.go',
    )
  })
})

describe('splitPaths', () => {
  it('detects a Git Bash path embedded in a shell command', () => {
    const segments = splitPaths('cd /c/Users/bilal/Desktop/Projects/TionHarness && go build ./...')
    const pathSeg = segments.find((s) => s.kind === 'path')
    expect(pathSeg?.text).toBe('/c/Users/bilal/Desktop/Projects/TionHarness')
  })

  it('keeps a whole URL in one url segment (scheme not cut off)', () => {
    const segments = splitPaths('https://docs.anthropic.com/en/docs/build.md')
    expect(segments).toEqual([{ text: 'https://docs.anthropic.com/en/docs/build.md', kind: 'url' }])
  })

  it('does not mistake a URL for a file path', () => {
    const segments = splitPaths('WebSearch → https://go.dev/doc/tutorial/web-service-gin')
    expect(segments.some((s) => s.kind === 'path')).toBe(false)
  })

  it('links a URL embedded in prose and leaves the surrounding text alone', () => {
    const segments = splitPaths('kaynak: https://example.com/a?b=1&c=2 — bak')
    expect(segments).toEqual([
      { text: 'kaynak: ', kind: 'text' },
      { text: 'https://example.com/a?b=1&c=2', kind: 'url' },
      { text: ' — bak', kind: 'text' },
    ])
  })

  it('drops trailing sentence punctuation from the link', () => {
    expect(splitPaths('see https://example.com/a.')).toEqual([
      { text: 'see ', kind: 'text' },
      { text: 'https://example.com/a', kind: 'url' },
      { text: '.', kind: 'text' },
    ])
  })

  it('drops an unbalanced closing paren but keeps a balanced one', () => {
    expect(splitPaths('(https://en.wikipedia.org/wiki/Go)').map((s) => s.text)).toEqual([
      '(',
      'https://en.wikipedia.org/wiki/Go',
      ')',
    ])
    expect(splitPaths('https://en.wikipedia.org/wiki/Go_(language)')[0]).toEqual({
      text: 'https://en.wikipedia.org/wiki/Go_(language)',
      kind: 'url',
    })
  })

  it('treats a scheme-less www. host as a URL', () => {
    const segments = splitPaths('www.example.com/docs')
    expect(segments[0].kind).toBe('url')
    expect(urlHref(segments[0].text)).toBe('https://www.example.com/docs')
  })

  it('still detects paths that appear after a URL', () => {
    const segments = splitPaths('https://example.com/x then internal/agent/titler.go')
    expect(segments.map((s) => s.kind)).toEqual(['url', 'text', 'path'])
  })
})

describe('isUrl', () => {
  it('accepts absolute and www. links, rejects paths', () => {
    expect(isUrl('https://example.com')).toBe(true)
    expect(isUrl('www.example.com')).toBe(true)
    expect(isUrl('/c/Users/bilal/a.go')).toBe(false)
    expect(isUrl('internal/agent/titler.go')).toBe(false)
  })
})

describe('mediaUrl', () => {
  it('routes a local image through the file server', () => {
    expect(mediaUrl('C:\\tmp\\a.png')).toBe('/api/files?path=C%3A%5Ctmp%5Ca.png')
  })

  it('passes a remote image URL through untouched', () => {
    expect(mediaUrl('https://example.com/a.png')).toBe('https://example.com/a.png')
  })
})

import { describe, expect, it } from 'vitest'
import {
  displayPath,
  isUrl,
  mediaUrl,
  normalizeMarkdownPaths,
  pathTarget,
  shortPath,
  splitPaths,
  urlHref,
} from './paths'

describe('displayPath', () => {
  it('collapses a Windows home path to ~\\', () => {
    expect(displayPath('C:\\Users\\alex\\Desktop\\Projects\\TionHarness')).toBe(
      '~\\Desktop\\Projects\\TionHarness',
    )
  })

  it('collapses a POSIX home path to ~/', () => {
    expect(displayPath('/home/alex/projects/app.go')).toBe('~/projects/app.go')
  })

  it('collapses a Git Bash mounted drive home path to ~/', () => {
    expect(displayPath('/c/Users/alex/Desktop/Projects/TionHarness')).toBe(
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
    expect(shortPath('/c/Users/alex/Desktop/Projects/TionHarness/internal/agent/runtime.go')).toBe(
      '…/internal/agent/runtime.go',
    )
  })
})

describe('splitPaths', () => {
  it('detects a Git Bash path embedded in a shell command', () => {
    const segments = splitPaths('cd /c/Users/alex/Desktop/Projects/TionHarness && go build ./...')
    const pathSeg = segments.find((s) => s.kind === 'path')
    expect(pathSeg?.text).toBe('/c/Users/alex/Desktop/Projects/TionHarness')
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

  it.each([
    ['Windows', 'C:\\work\\src\\app.ts:12:4', 'C:\\work\\src\\app.ts'],
    ['Windows slash', 'C:/work/src/app.ts:12', 'C:/work/src/app.ts'],
    ['POSIX', '/opt/app/src/main.go:9', '/opt/app/src/main.go'],
    ['Git Bash', '/c/Users/alex/project/main.go:7:2', '/c/Users/alex/project/main.go'],
    ['relative', 'frontend/src/App.tsx:20:3', 'frontend/src/App.tsx'],
  ])('detects %s path references with locations', (_label, shown, target) => {
    expect(splitPaths(shown)).toEqual([{ text: shown, kind: 'path', target }])
  })

  it('keeps an unquoted Windows path with spaces and a location in one segment', () => {
    const shown = 'C:\\Users\\user\\Desktop\\My Project\\file.ts:12:4'
    expect(splitPaths(shown)).toEqual([
      { text: shown, kind: 'path', target: 'C:\\Users\\user\\Desktop\\My Project\\file.ts' },
    ])
  })

  it.each([
    ['"C:\\Program Files\\Git\\bin\\bash.exe"', 'C:\\Program Files\\Git\\bin\\bash.exe'],
    ['`/tmp/project files/app.ts:4`', '/tmp/project files/app.ts'],
    ["'./folder name/data.json:8:2'", './folder name/data.json'],
  ])('links a quoted path with spaces while preserving delimiters', (source, target) => {
    const segments = splitPaths(source)
    expect(segments.map((segment) => segment.text).join('')).toBe(source)
    expect(segments.find((segment) => segment.kind === 'path')?.target).toBe(target)
  })

  it.each([
    [
      'https://example.com/search?q=a%20b&lang=tr#result',
      'https://example.com/search?q=a%20b&lang=tr#result',
    ],
    ['https://example.com/wiki/Foo_(bar)', 'https://example.com/wiki/Foo_(bar)'],
    ['(https://example.com/a).', 'https://example.com/a'],
  ])('keeps URL query/hash and trims only prose punctuation: %s', (source, linked) => {
    expect(splitPaths(source).find((segment) => segment.kind === 'url')?.text).toBe(linked)
  })
})

describe('path normalization', () => {
  it.each([
    ['C:\\src\\app.ts:10:2', 'C:\\src\\app.ts'],
    ['/src/app.ts:10', '/src/app.ts'],
    ['C:\\src\\app.ts', 'C:\\src\\app.ts'],
  ])('removes only a trailing source location from %s', (source, expected) => {
    expect(pathTarget(source)).toBe(expected)
  })

  it('normalizes Windows Markdown destinations without touching URLs or prose', () => {
    expect(
      normalizeMarkdownPaths(
        '[file](C:\\Program Files\\app.ts) ![img](C:\\tmp\\a.png) https://x.test/a\\b',
      ),
    ).toBe('[file](C:/Program Files/app.ts) ![img](C:/tmp/a.png) https://x.test/a\\b')
  })
})

describe('isUrl', () => {
  it('accepts absolute and www. links, rejects paths', () => {
    expect(isUrl('https://example.com')).toBe(true)
    expect(isUrl('www.example.com')).toBe(true)
    expect(isUrl('/c/Users/alex/a.go')).toBe(false)
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

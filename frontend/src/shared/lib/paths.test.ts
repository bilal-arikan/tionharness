import { describe, expect, it } from 'vitest'
import { displayPath, shortPath, splitPaths } from './paths'

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
    const pathSeg = segments.find((s) => s.isPath)
    expect(pathSeg?.text).toBe('/c/Users/bilal/Desktop/Projects/TionHarness')
  })
})

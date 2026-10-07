import { afterEach, describe, expect, it, vi } from 'vitest'
import { examplePath, isApplePlatform, modKeyLabel, normalizeServerOS, pickByOS } from './platform'

describe('normalizeServerOS', () => {
  it('maps Go GOOS values to OS families', () => {
    expect(normalizeServerOS('windows')).toBe('windows')
    expect(normalizeServerOS('darwin')).toBe('darwin')
    expect(normalizeServerOS('linux')).toBe('linux')
    expect(normalizeServerOS('freebsd')).toBe('linux')
  })

  it('returns undefined for missing or unknown values', () => {
    expect(normalizeServerOS(undefined)).toBeUndefined()
    expect(normalizeServerOS('')).toBeUndefined()
    expect(normalizeServerOS('plan9')).toBeUndefined()
  })
})

describe('pickByOS', () => {
  const choices = { windows: 'w', darwin: 'd', linux: 'l', other: 'o' }
  it('picks the matching entry and falls back to other', () => {
    expect(pickByOS('windows', choices)).toBe('w')
    expect(pickByOS('darwin', choices)).toBe('d')
    expect(pickByOS('linux', choices)).toBe('l')
    expect(pickByOS(undefined, choices)).toBe('o')
  })
})

describe('modKeyLabel', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('shows ⌘ on Apple platforms', () => {
    vi.stubGlobal('navigator', { platform: 'MacIntel' })
    expect(isApplePlatform()).toBe(true)
    expect(modKeyLabel()).toBe('⌘')
  })

  it('prefers userAgentData.platform when present', () => {
    vi.stubGlobal('navigator', { userAgentData: { platform: 'macOS' }, platform: 'Win32' })
    expect(modKeyLabel()).toBe('⌘')
  })

  it('shows Ctrl elsewhere', () => {
    vi.stubGlobal('navigator', { platform: 'Win32' })
    expect(isApplePlatform()).toBe(false)
    expect(modKeyLabel()).toBe('Ctrl')
  })
})

describe('examplePath', () => {
  it('keeps the Windows example and adds native POSIX ones', () => {
    expect(examplePath('windows', 'project')).toBe('C:\\Users\\...\\Desktop\\Projects\\my-app')
    expect(examplePath('darwin', 'project')).toBe('/Users/<you>/Projects/my-app')
    expect(examplePath('linux', 'project')).toBe('/home/<you>/projects/my-app')
    expect(examplePath(undefined, 'project')).toBe('~/Projects/my-app')
    expect(examplePath('darwin', 'repo')).toBe('/path/to/repo')
  })
})

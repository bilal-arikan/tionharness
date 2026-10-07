import { describe, expect, it } from 'vitest'
import { detectTrigger, isSlashCommandText } from './trigger'

describe('isSlashCommandText', () => {
  it('accepts a slash command with or without arguments', () => {
    expect(isSlashCommandText('/compact')).toBe(true)
    expect(isSlashCommandText('/review src/app.ts')).toBe(true)
    expect(isSlashCommandText('/plugin:cmd arg')).toBe(true)
  })

  it('treats absolute POSIX paths as prose', () => {
    expect(isSlashCommandText('/Users/me/app/main.go fails')).toBe(false)
    expect(isSlashCommandText('/tmp/x')).toBe(false)
    expect(isSlashCommandText('/home/me/project')).toBe(false)
    expect(isSlashCommandText('/')).toBe(false)
    expect(isSlashCommandText('/ hello')).toBe(false)
  })
})

describe('detectTrigger command palette', () => {
  it('opens for a single /word', () => {
    expect(detectTrigger('/comp', 5)).toEqual({ mode: 'command', query: 'comp' })
    expect(detectTrigger('/opt', 4)).toEqual({ mode: 'command', query: 'opt' })
  })

  it('stays closed once the token becomes a POSIX path', () => {
    expect(detectTrigger('/opt/', 5)).toBeNull()
    expect(detectTrigger('/tmp/build.log', 14)).toBeNull()
    expect(detectTrigger('/Users/me', 9)).toBeNull()
  })

  it('stays closed after a space', () => {
    expect(detectTrigger('/compact now', 12)).toBeNull()
  })
})

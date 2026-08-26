import { describe, expect, it } from 'vitest'
import { toolMeta } from './tools'

describe('toolMeta', () => {
  it('strips a shell host from string input', () => {
    const input =
      '"C:\\\\Windows\\\\System32\\\\WindowsPowerShell\\\\v1.0\\\\powershell.exe" -NoProfile -Command \'Get-Content -LiteralPath CLAUDE.md\''

    expect(toolMeta('powershell', input).summary).toBe('Get-Content -LiteralPath CLAUDE.md')
  })

  it('preserves unwrapped string input', () => {
    expect(toolMeta('bash', 'git status').summary).toBe('git status')
  })
})

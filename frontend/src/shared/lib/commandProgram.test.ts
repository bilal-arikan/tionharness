import { describe, expect, it } from 'vitest'
import { stripShellHost } from './commandProgram'

describe('stripShellHost', () => {
  it.each([
    ['"C:\\Windows\\system32\\bash.exe" -lc "git status --short"', 'git status --short'],
    ["bash -lc 'npm run build'", 'npm run build'],
    ['cmd.exe /c "git status"', 'git status'],
    ['"C:\\Windows\\system32\\cmd.exe" /c "npm test"', 'npm test'],
    ['cmd /K "pnpm build"', 'pnpm build'],
    ['cmd /k npm run dev', 'npm run dev'],
    ['cmd.exe /c npm run build --silent', 'npm run build --silent'],
    ['bash -lc "echo \\"hi there\\""', 'echo "hi there"'],
    ['powershell -Command "Write-Host \\"a b\\""', 'Write-Host "a b"'],
    [
      '"C:\\\\Windows\\\\System32\\\\WindowsPowerShell\\\\v1.0\\\\powershell.exe" -NoProfile -Command \'Get-Content -LiteralPath CLAUDE.md\'',
      'Get-Content -LiteralPath CLAUDE.md',
    ],
    ['git status', 'git status'],
    ['bash', 'bash'],
  ])('strips shell host from %s', (command, expected) => {
    expect(stripShellHost(command)).toBe(expected)
  })
})

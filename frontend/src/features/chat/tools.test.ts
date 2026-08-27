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

  it('summarizes entity-addressing tools by their identity', () => {
    expect(toolMeta('get_view', { kind: 'board', id: 'TSK379', level: 'full' }).summary).toBe(
      'board · TSK379',
    )
    expect(toolMeta('expand', { kind: 'session', id: 'SES1787', sub: 'steps' }).summary).toBe(
      'session · SES1787/steps',
    )
  })

  it('summarizes filter-only list inputs by their values', () => {
    expect(
      toolMeta('list_agents', { provider: 'claude', state: 'idle', sort: 'name', limit: 20 })
        .summary,
    ).toBe('provider=claude, state=idle')
    expect(toolMeta('list_workers', { scope: 'subtree', limit: 50, offset: 0 }).summary).toBe(
      'scope=subtree',
    )
  })

  it('summarizes report_to_coordinator and boolean switches', () => {
    expect(
      toolMeta('report_to_coordinator', { summary: 'done', status: 'completed' }).summary,
    ).toBe('done')
    expect(toolMeta('set_coordinator_mode', { enabled: true }).summary).toBe('enabled=true')
  })

  it('stays blank when the input carries only paging fields', () => {
    expect(toolMeta('list_tasks', { limit: 20, offset: 0 }).summary).toBe('')
  })
})

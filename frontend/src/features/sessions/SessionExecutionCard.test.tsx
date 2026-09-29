// @vitest-environment jsdom

import { beforeEach, describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import type { SessionInfo } from '@/types'
import { formatDurationMs, SessionExecutionCard } from './SessionExecutionCard'
import { i18next } from '@/i18n'

const info = {
  executionType: 'subagent',
  category: 'subagent',
  contextMode: 'isolated',
  targetProfile: 'coder',
  runState: 'completed',
  terminal: true,
  durationMs: 1250,
  inputTokens: 1200,
  outputTokens: 340,
  toolCallCount: 3,
  persistedSteps: 7,
  stopReason: 'end_turn',
  errorSummary: 'permission_denied',
} as unknown as SessionInfo

describe('SessionExecutionCard', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('tr')
  })

  it('renders persisted child execution facts without raw step payloads', () => {
    const html = renderToStaticMarkup(<SessionExecutionCard info={info} />)
    expect(html).toContain('Profil: coder')
    expect(html).toContain('Tamamlandı')
    expect(html).toContain('permission_denied')
    expect(html).toContain('1,2k giriş · 340 çıkış')
  })

  it('formats unknown and measured durations', () => {
    expect(formatDurationMs(0)).toBe('—')
    expect(formatDurationMs(1250)).toBe('1.3 sn')
  })

  it('renders the same execution facts in English', async () => {
    await i18next.changeLanguage('en')
    const html = renderToStaticMarkup(<SessionExecutionCard info={info} />)
    expect(html).toContain('Execution')
    expect(html).toContain('Profile: coder')
    expect(html).toContain('Completed')
    expect(html).toContain('1.2k input · 340 output')
  })
})

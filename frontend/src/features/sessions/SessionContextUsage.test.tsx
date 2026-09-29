// @vitest-environment jsdom

import { beforeEach, describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { i18next } from '@/i18n'
import type { SessionInfo } from '@/types'
import { SessionContextUsage } from './SessionContextUsage'

const info = {
  fillers: [
    { role: 'system', label: 'Sistem promptu', tokens: 180, count: 1 },
    { role: 'tools', label: 'Araçlar', tokens: 140, count: 4 },
    {
      role: 'lazy-tools',
      label: 'Araç kataloğu (talep üzerine)',
      tokens: 90,
      count: 12,
    },
    { role: 'skills', label: 'Skill kataloğu', tokens: 70, count: 2 },
    {
      role: 'cli-harness',
      label: 'CLI ek yükü (claude-cli, referans)',
      tokens: 120,
      count: 1,
    },
    { role: 'custom-role', label: 'Workspace-specific bucket', tokens: 20, count: 1 },
  ],
  running: { provider: 'claude-cli' },
  hasSummary: false,
} as unknown as SessionInfo

describe('SessionContextUsage localization', () => {
  beforeEach(async () => {
    await i18next.changeLanguage('en')
  })

  it('translates structured backend filler roles and preserves unknown labels', () => {
    const html = renderToStaticMarkup(
      <SessionContextUsage info={info} ctxWindow={1_000} ctxUsed={620} ctxFree={380} ctxPct={62} />,
    )

    expect(html).toContain('System prompt')
    expect(html).toContain('Tools')
    expect(html).toContain('Tool catalog (on demand)')
    expect(html).toContain('Skill catalog')
    expect(html).toContain('CLI overhead (claude-cli, reference)')
    expect(html).toContain('Workspace-specific bucket')
    expect(html).not.toContain('Sistem promptu')
    expect(html).not.toContain('Araç kataloğu')
    expect(html).not.toContain('CLI ek yükü')
  })

  it('uses the calibrated flag for a measured CLI overhead label', () => {
    const measured = {
      ...info,
      fillers: info.fillers.map((f) =>
        f.role === 'cli-harness'
          ? { ...f, label: 'CLI ek yükü (claude-cli, ölçülmüş · 4 tur)', calibrated: true }
          : f,
      ),
    }
    const html = renderToStaticMarkup(
      <SessionContextUsage
        info={measured}
        ctxWindow={1_000}
        ctxUsed={620}
        ctxFree={380}
        ctxPct={62}
      />,
    )
    expect(html).toContain('CLI overhead (claude-cli, measured)')
    expect(html).not.toContain('4 tur')
  })
})

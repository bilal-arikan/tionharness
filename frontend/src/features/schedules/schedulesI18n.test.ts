import { afterEach, describe, expect, it } from 'vitest'
import { setLocale } from '@/i18n'
import { BOARD_OPS, DEFAULT_PROMPT, STUCK_TEMPLATE, TRAJ_PROMPT_VARS } from './automationMeta'
import { PRESET_GROUPS } from './cronPresets'
import { SKIP_REASON_LABEL } from './fireMeta'

afterEach(async () => {
  await setLocale('en')
})

describe('schedule localization metadata', () => {
  it('updates labels when the UI locale changes without changing machine values', async () => {
    await setLocale('en')
    expect(BOARD_OPS.find((item) => item.value === 'move')?.label).toBe('Moved (column changed)')
    expect(PRESET_GROUPS[0].items[1].expr).toBe('*/5 * * * *')
    expect(TRAJ_PROMPT_VARS[0].name).toBe('{{trajectoryId}}')
    expect(STUCK_TEMPLATE.name).toBe('Stuck session repairer')

    await setLocale('tr')
    expect(BOARD_OPS.find((item) => item.value === 'move')?.label).toBe('Taşındı (sütun değişti)')
    expect(PRESET_GROUPS[0].items[1].label).toBe('Her 5 dakika')
    expect(SKIP_REASON_LABEL.max_iterations).toBe('iterasyon tavanı')
    expect(STUCK_TEMPLATE.name).toBe('Stuck oturum onarıcısı')
  })

  it('keeps saved prompt templates out of UI translation', () => {
    expect(DEFAULT_PROMPT.tag).toContain('{{result}}')
    expect(DEFAULT_PROMPT.board).toContain('{{toLabel}}')
  })
})

import { describe, expect, it } from 'vitest'
import { parseJudgeLabel } from './judgeLabel'

describe('parseJudgeLabel', () => {
  it('splits the picked arm from the judgement note', () => {
    expect(parseJudgeLabel('A feature request (judge 0.93)')).toEqual({
      arm: 'A feature request',
      note: 'judge 0.93',
    })
  })

  it('reads default-arm fallbacks', () => {
    expect(parseJudgeLabel('default (judge unsure, 0.41)')).toEqual({
      arm: 'default',
      note: 'judge unsure, 0.41',
    })
    expect(parseJudgeLabel('default (judge error: decider is disabled)')).toEqual({
      arm: 'default',
      note: 'judge error: decider is disabled',
    })
    expect(parseJudgeLabel('default (no options)')).toEqual({ arm: 'default', note: 'no options' })
  })

  it('keeps an arm description that itself contains parentheses', () => {
    expect(parseJudgeLabel('Bug (crash or data loss) (judge 0.8)')).toEqual({
      arm: 'Bug (crash or data loss)',
      note: 'judge 0.8',
    })
  })

  it('returns plain labels whole', () => {
    expect(parseJudgeLabel('default')).toEqual({ arm: 'default', note: '' })
    expect(parseJudgeLabel('Bug (crash)')).toEqual({ arm: 'Bug (crash)', note: '' })
  })
})

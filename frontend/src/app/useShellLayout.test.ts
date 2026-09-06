import { describe, expect, it } from 'vitest'
import { computeShellLayout } from './useShellLayout'

describe('computeShellLayout', () => {
  it('narrow: bottom nav, both side panels are drawers', () => {
    expect(computeShellLayout('narrow', 'portrait')).toEqual({
      rail: 'hidden',
      list: 'drawer',
      detail: 'drawer',
      measure: 'fluid',
    })
  })
  it('square: compact rail, docked list, detail still a drawer', () => {
    expect(computeShellLayout('square', 'landscape')).toEqual({
      rail: 'compact',
      list: 'docked',
      detail: 'drawer',
      measure: 'fluid',
    })
  })
  it('wide: everything docks', () => {
    expect(computeShellLayout('wide', 'landscape')).toEqual({
      rail: 'full',
      list: 'docked',
      detail: 'docked',
      measure: 'fluid',
    })
  })
  it('wide but portrait-oriented: the detail panel stays a drawer', () => {
    expect(computeShellLayout('wide', 'portrait').detail).toBe('drawer')
    expect(computeShellLayout('wide', 'square').detail).toBe('docked')
  })
  it('ultra: docked columns plus a centred reading measure', () => {
    expect(computeShellLayout('ultra', 'landscape')).toEqual({
      rail: 'full',
      list: 'docked',
      detail: 'docked',
      measure: 'centered',
    })
  })
})

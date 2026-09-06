import { describe, expect, it } from 'vitest'
import { capListColumnWidth, classifyAspect, classifyViewport, classifyWidth } from './viewport'

describe('classifyWidth', () => {
  it('maps widths onto the four tiers at the shared breakpoints', () => {
    expect(classifyWidth(320)).toBe('narrow')
    expect(classifyWidth(767)).toBe('narrow')
    expect(classifyWidth(768)).toBe('square')
    expect(classifyWidth(1279)).toBe('square')
    expect(classifyWidth(1280)).toBe('wide')
    expect(classifyWidth(1919)).toBe('wide')
    expect(classifyWidth(1920)).toBe('ultra')
    expect(classifyWidth(3840)).toBe('ultra')
  })
})

describe('classifyAspect', () => {
  it('splits portrait / square / landscape by width:height ratio', () => {
    expect(classifyAspect(390, 844)).toBe('portrait')
    expect(classifyAspect(1200, 1600)).toBe('portrait')
    expect(classifyAspect(1024, 1024)).toBe('square')
    expect(classifyAspect(1280, 1024)).toBe('square')
    expect(classifyAspect(1920, 1080)).toBe('landscape')
    expect(classifyAspect(3440, 1440)).toBe('landscape')
  })
  it('treats a zero height as landscape instead of dividing by zero', () => {
    expect(classifyAspect(800, 0)).toBe('landscape')
  })
})

describe('classifyViewport', () => {
  it('combines both axes', () => {
    expect(classifyViewport(1200, 1600)).toEqual({ tier: 'square', aspect: 'portrait' })
    expect(classifyViewport(2560, 1440)).toEqual({ tier: 'ultra', aspect: 'landscape' })
  })
})

describe('capListColumnWidth', () => {
  it('caps only the square tier and leaves the stored width alone elsewhere', () => {
    expect(capListColumnWidth(400, 'square')).toBe(256)
    expect(capListColumnWidth(200, 'square')).toBe(200)
    expect(capListColumnWidth(400, 'wide')).toBe(400)
    expect(capListColumnWidth(640, 'ultra')).toBe(640)
    expect(capListColumnWidth(400, 'narrow')).toBe(400)
  })
})

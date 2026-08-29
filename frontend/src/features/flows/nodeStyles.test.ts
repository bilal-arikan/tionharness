import { describe, expect, it } from 'vitest'
import { chromeFor, nodeHeaderForeground } from './nodeStyles'

describe('nodeHeaderForeground', () => {
  it('uses dark text on light node headers and inspector icons', () => {
    expect(nodeHeaderForeground(chromeFor('await-input').accent)).toBe('#000')
    expect(nodeHeaderForeground(chromeFor('start').accent)).toBe('#000')
  })

  it('uses light text on dark node headers', () => {
    expect(nodeHeaderForeground(chromeFor('parallel').accent)).toBe('#fff')
  })

  it('keeps the semantic foreground for theme accent colors', () => {
    expect(nodeHeaderForeground(chromeFor('agent').accent)).toBe('var(--color-on-accent)')
  })
})

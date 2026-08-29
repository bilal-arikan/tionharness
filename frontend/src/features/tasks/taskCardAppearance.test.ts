import { describe, expect, it } from 'vitest'
import { taskCardShadowClass } from './taskCardAppearance'

describe('taskCardShadowClass', () => {
  it('uses only the glow shadow for a recently changed card', () => {
    const className = taskCardShadowClass(true, false)

    expect(className).toBe(
      'shadow-[0_0_0_1px_var(--color-accent),0_0_14px_2px_var(--color-accent)]',
    )
    expect(className).not.toContain('shadow-[var(--shadow-sm)]')
  })

  it('uses the regular shadow for unchanged and pending cards', () => {
    expect(taskCardShadowClass(false, false)).toBe('shadow-[var(--shadow-sm)]')
    expect(taskCardShadowClass(true, true)).toBe('shadow-[var(--shadow-sm)]')
  })
})

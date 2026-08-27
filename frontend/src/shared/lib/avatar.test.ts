import { describe, expect, it } from 'vitest'
import { avatarForeground } from './avatar'

describe('avatarForeground', () => {
  it('uses a light foreground on dark backgrounds', () => {
    expect(avatarForeground('#17171a')).toBe('#fff')
  })

  it('uses a dark foreground on light backgrounds', () => {
    expect(avatarForeground('#f59e0b')).toBe('#000')
  })

  it('rejects invalid persisted colors', () => {
    expect(() => avatarForeground('transparent')).toThrow('Invalid avatar color: transparent')
  })
})

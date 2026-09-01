import { describe, expect, it, vi } from 'vitest'
import {
  browserSessionChipStorage,
  readSessionChipsOff,
  writeSessionChipsOff,
} from './useSessionChips'

describe('session chip persistence', () => {
  it('falls back to all chips enabled when localStorage reads throw', () => {
    const storage = {
      getItem: vi.fn(() => {
        throw new DOMException('blocked', 'SecurityError')
      }),
      setItem: vi.fn(),
    }
    expect(readSessionChipsOff(storage)).toEqual([])
  })

  it('does not throw when localStorage writes are blocked', () => {
    const storage = {
      getItem: vi.fn(),
      setItem: vi.fn(() => {
        throw new DOMException('blocked', 'SecurityError')
      }),
    }
    expect(() => writeSessionChipsOff(storage, ['chat'])).not.toThrow()
  })

  it('handles a blocked localStorage property getter', () => {
    const scope = Object.defineProperty({}, 'localStorage', {
      get() {
        throw new DOMException('blocked', 'SecurityError')
      },
    }) as { readonly localStorage: Storage }

    expect(browserSessionChipStorage(scope)).toBeNull()
  })
})

import { describe, expect, it } from 'vitest'
import { ALL_SESSION_CHIPS, nextChipsOff } from './sessionKindMeta'

const KEY = ALL_SESSION_CHIPS[0]
const OTHER = ALL_SESSION_CHIPS[1]

describe('nextChipsOff', () => {
  it('toggles a single chip off and back on', () => {
    const off = nextChipsOff([], KEY, 'toggle')
    expect(off).toEqual([KEY])
    expect(nextChipsOff(off, KEY, 'toggle')).toEqual([])
  })

  it('solo leaves only the clicked chip on', () => {
    const off = nextChipsOff([], KEY, 'solo')
    expect(off).toEqual(ALL_SESSION_CHIPS.filter((k) => k !== KEY))
  })

  it('solo turns the clicked chip on even when it was off', () => {
    expect(nextChipsOff([KEY], KEY, 'solo')).not.toContain(KEY)
  })

  it('invert flips every other chip and keeps the clicked one', () => {
    // KEY off, OTHER on -> KEY stays off, OTHER becomes off, rest become on
    const off = nextChipsOff([KEY], KEY, 'invert')
    expect(off).toContain(KEY)
    expect(off).toContain(OTHER)
    expect(off).toHaveLength(ALL_SESSION_CHIPS.length)
  })

  it('invert on an all-on state turns every other chip off', () => {
    const off = nextChipsOff([], KEY, 'invert')
    expect(off).toEqual(ALL_SESSION_CHIPS.filter((k) => k !== KEY))
  })
})

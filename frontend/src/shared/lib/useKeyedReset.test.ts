// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { act } from 'react'
import { createRoot } from 'react-dom/client'
import { createElement, useEffect, useState } from 'react'
import { useKeyedReset } from './useKeyedReset'

// Renders a probe that mirrors `key` into local state through useKeyedReset and
// records every committed value (in an effect), so the test can see that a key
// change never commits the stale state first.
function mount() {
  const commits: string[] = []
  let setKey: (k: string) => void = () => {}
  let resets = 0
  function Probe() {
    const [key, set] = useState('a')
    const [mirror, setMirror] = useState('init')
    setKey = set
    useKeyedReset(key, (k) => {
      resets++
      setMirror(`reset:${k}`)
    })
    // Recorded after commit: a render pass discarded by the in-render reset
    // never reaches this effect.
    useEffect(() => {
      commits.push(`${key}/${mirror}`)
    })
    return null
  }
  const host = document.createElement('div')
  const root = createRoot(host)
  act(() => root.render(createElement(Probe)))
  return { commits, setKey: (k: string) => act(() => setKey(k)), resets: () => resets, root }
}

describe('useKeyedReset', () => {
  it('does not run on the first render', () => {
    const p = mount()
    expect(p.resets()).toBe(0)
    expect(p.commits).toEqual(['a/init'])
    act(() => p.root.unmount())
  })

  it('resets during render when the key changes, without painting the stale state', () => {
    const p = mount()
    p.setKey('b')
    expect(p.resets()).toBe(1)
    // The render with key=b and the stale mirror is discarded; only the reset
    // value reaches the committed sequence.
    expect(p.commits.filter((c) => c.startsWith('b/'))).toEqual(['b/reset:b'])
    act(() => p.root.unmount())
  })

  it('ignores re-renders with the same key', () => {
    const p = mount()
    p.setKey('a')
    expect(p.resets()).toBe(0)
    act(() => p.root.unmount())
  })
})

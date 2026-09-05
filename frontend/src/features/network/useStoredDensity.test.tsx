// @vitest-environment jsdom

import { act, useEffect } from 'react'
import { createRoot, type Root } from 'react-dom/client'
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import { parseDensity, useStoredDensity } from './useStoredDensity'

;(
  globalThis as typeof globalThis & { IS_REACT_ACT_ENVIRONMENT: boolean }
).IS_REACT_ACT_ENVIRONMENT = true

const roots = new Set<Root>()
let latest: [number, (value: number) => void] | null = null

function Probe({ storageKey, report }: { storageKey: string; report: (s: typeof latest) => void }) {
  const state = useStoredDensity(storageKey)
  useEffect(() => report(state))
  return null
}

async function mount(storageKey: string) {
  const host = document.createElement('div')
  document.body.appendChild(host)
  const root = createRoot(host)
  roots.add(root)
  const report = (s: typeof latest) => {
    latest = s
  }
  await act(async () => root.render(<Probe storageKey={storageKey} report={report} />))
  return root
}

beforeEach(() => {
  localStorage.clear()
  latest = null
})

afterEach(async () => {
  for (const root of [...roots]) {
    await act(async () => root.unmount())
    roots.delete(root)
  }
})

describe('parseDensity', () => {
  it('accepts the slider range and falls back otherwise', () => {
    expect(parseDensity('1.6')).toBe(1.6)
    expect(parseDensity('0.4')).toBe(0.4)
    expect(parseDensity(null)).toBe(1)
    expect(parseDensity('abc')).toBe(1)
    expect(parseDensity('9')).toBe(1)
    expect(parseDensity('0')).toBe(1)
  })
})

describe('useStoredDensity', () => {
  it('persists per key and restores on the next mount', async () => {
    const root = await mount('k1')
    expect(latest![0]).toBe(1)
    await act(async () => latest![1](1.4))
    expect(latest![0]).toBe(1.4)
    expect(localStorage.getItem('k1')).toBe('1.4')
    await act(async () => root.unmount())
    roots.delete(root)

    await mount('k1')
    expect(latest![0]).toBe(1.4)
    await mount('k2')
    expect(latest![0]).toBe(1)
  })
})

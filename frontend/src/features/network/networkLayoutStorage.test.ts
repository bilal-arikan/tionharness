import { describe, expect, it } from 'vitest'
import {
  NETWORK_LAYOUT_VERSION,
  networkLayoutKey,
  pruneNetworkPositions,
  readNetworkPositions,
  writeNetworkPositions,
} from './networkLayoutStorage'

function memoryStorage(): Storage {
  const values = new Map<string, string>()
  return {
    get length() {
      return values.size
    },
    clear: () => values.clear(),
    getItem: (key) => values.get(key) ?? null,
    key: (index) => [...values.keys()][index] ?? null,
    removeItem: (key) => values.delete(key),
    setItem: (key, value) => values.set(key, value),
  }
}

describe('network layout storage', () => {
  it('isolates positions by workspace and layout version', () => {
    const storage = memoryStorage()
    writeNetworkPositions('workspace-a', { node: { x: 1, y: 2 } }, storage)

    expect(readNetworkPositions('workspace-a', storage)).toEqual({ node: { x: 1, y: 2 } })
    expect(readNetworkPositions('workspace-b', storage)).toEqual({})
    expect(networkLayoutKey('workspace-a')).toContain(`.v${NETWORK_LAYOUT_VERSION}.`)
  })

  it('rejects corrupt, mismatched, and non-finite values safely', () => {
    const storage = memoryStorage()
    storage.setItem(networkLayoutKey('broken'), '{')
    storage.setItem(
      networkLayoutKey('old'),
      JSON.stringify({ v: NETWORK_LAYOUT_VERSION + 1, positions: { node: { x: 1, y: 2 } } }),
    )
    storage.setItem(
      networkLayoutKey('mixed'),
      JSON.stringify({
        v: NETWORK_LAYOUT_VERSION,
        positions: { good: { x: 3, y: 4 }, infinite: { x: null, y: 1 }, text: { x: '3', y: 4 } },
      }),
    )

    expect(readNetworkPositions('broken', storage)).toEqual({})
    expect(readNetworkPositions('old', storage)).toEqual({})
    expect(readNetworkPositions('mixed', storage)).toEqual({ good: { x: 3, y: 4 } })
  })

  it('prunes stale node ids without mutating input', () => {
    const positions = { kept: { x: 1, y: 2 }, stale: { x: 3, y: 4 } }
    expect(pruneNetworkPositions(positions, ['kept'])).toEqual({ kept: { x: 1, y: 2 } })
    expect(positions).toHaveProperty('stale')
  })

  it('swallows storage quota errors', () => {
    const storage = memoryStorage()
    storage.setItem = () => {
      throw new DOMException('quota', 'QuotaExceededError')
    }
    expect(() =>
      writeNetworkPositions('workspace', { node: { x: 1, y: 2 } }, storage),
    ).not.toThrow()
  })
})

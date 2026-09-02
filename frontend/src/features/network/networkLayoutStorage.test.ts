import { describe, expect, it } from 'vitest'
import {
  NETWORK_LAYOUT_VERSION,
  networkLayoutKey,
  pruneNetworkPositions,
  readNetworkLayout,
  readNetworkPositions,
  writeNetworkLayout,
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

  it('round-trips active physics state and finite node velocities', () => {
    const storage = memoryStorage()

    writeNetworkLayout(
      'workspace',
      {
        physicsActive: true,
        positions: {
          moving: { x: 1, y: 2, vx: 3, vy: 4 },
          invalidVelocity: { x: 5, y: 6, vx: Number.POSITIVE_INFINITY, vy: 7 },
        },
      },
      storage,
    )

    expect(readNetworkLayout('workspace', storage)).toEqual({
      physicsActive: true,
      positions: {
        moving: { x: 1, y: 2, vx: 3, vy: 4 },
        invalidVelocity: { x: 5, y: 6 },
      },
    })
  })

  it('prunes stale node ids without mutating input', () => {
    const positions = { kept: { x: 1, y: 2 }, stale: { x: 3, y: 4 } }
    expect(pruneNetworkPositions(positions, ['kept'])).toEqual({ kept: { x: 1, y: 2 } })
    expect(positions).toHaveProperty('stale')
  })

  it('merges a snapshot with latest storage and prunes only canonical deletions', () => {
    const storage = memoryStorage()
    writeNetworkPositions(
      'workspace',
      { visible: { x: 1, y: 2 }, hidden: { x: 3, y: 4 }, deleted: { x: 5, y: 6 } },
      storage,
    )

    expect(
      writeNetworkPositions('workspace', { visible: { x: 10, y: 20 } }, storage, [
        'visible',
        'hidden',
      ]),
    ).toEqual({ visible: { x: 10, y: 20 }, hidden: { x: 3, y: 4 } })
    expect(readNetworkPositions('workspace', storage)).toEqual({
      visible: { x: 10, y: 20 },
      hidden: { x: 3, y: 4 },
    })
  })

  it('keeps entries introduced or updated after a stale writer snapshot', () => {
    const storage = memoryStorage()
    writeNetworkPositions('workspace', { kept: { x: 1, y: 2 }, stale: { x: 3, y: 4 } }, storage)
    const staleWriterSnapshot = readNetworkPositions('workspace', storage)

    writeNetworkPositions(
      'workspace',
      { kept: { x: 10, y: 20 }, concurrent: { x: 30, y: 40 }, stale: { x: 50, y: 60 } },
      storage,
    )

    expect(
      writeNetworkPositions(
        'workspace',
        staleWriterSnapshot,
        storage,
        ['kept'],
        staleWriterSnapshot,
      ),
    ).toEqual({
      kept: { x: 10, y: 20 },
      concurrent: { x: 30, y: 40 },
      stale: { x: 50, y: 60 },
    })
  })

  it('still prunes a stale id unchanged since the authoritative writer read', () => {
    const storage = memoryStorage()
    writeNetworkPositions('workspace', { kept: { x: 1, y: 2 }, stale: { x: 3, y: 4 } }, storage)
    const known = readNetworkPositions('workspace', storage)

    expect(
      writeNetworkPositions('workspace', { kept: { x: 10, y: 20 } }, storage, ['kept'], known),
    ).toEqual({ kept: { x: 10, y: 20 } })
  })

  it('does not resurrect a deletion observed after a stale writer snapshot', () => {
    const storage = memoryStorage()
    writeNetworkPositions('workspace', { kept: { x: 1, y: 2 }, stale: { x: 3, y: 4 } }, storage)
    const staleWriterSnapshot = readNetworkPositions('workspace', storage)

    writeNetworkPositions('workspace', { kept: { x: 10, y: 20 } }, storage, ['kept'])

    expect(
      writeNetworkPositions(
        'workspace',
        staleWriterSnapshot,
        storage,
        ['kept', 'stale'],
        staleWriterSnapshot,
      ),
    ).toEqual({ kept: { x: 10, y: 20 } })
    expect(readNetworkPositions('workspace', storage)).toEqual({ kept: { x: 10, y: 20 } })
  })

  it('keeps the committed baseline after a transient quota failure and retries', () => {
    const storage = memoryStorage()
    writeNetworkPositions('workspace', { node: { x: 1, y: 2 } }, storage)
    const baseline = readNetworkPositions('workspace', storage)
    const setItem = storage.setItem
    storage.setItem = () => {
      throw new DOMException('quota', 'QuotaExceededError')
    }
    expect(
      writeNetworkPositions('workspace', { node: { x: 10, y: 20 } }, storage, ['node'], baseline),
    ).toEqual(baseline)
    expect(readNetworkPositions('workspace', storage)).toEqual(baseline)

    storage.setItem = setItem
    expect(
      writeNetworkPositions('workspace', { node: { x: 10, y: 20 } }, storage, ['node'], baseline),
    ).toEqual({ node: { x: 10, y: 20 } })
    expect(readNetworkPositions('workspace', storage)).toEqual({ node: { x: 10, y: 20 } })
  })
})

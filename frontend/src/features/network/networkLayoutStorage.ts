export const NETWORK_LAYOUT_VERSION = 1

const KEY_PREFIX = 'tionharness.network-layout'

export interface NetworkPosition {
  x: number
  y: number
}

export type NetworkPositions = Record<string, NetworkPosition>

interface StoredNetworkLayout {
  v: number
  positions: NetworkPositions
}

export function networkLayoutKey(workspaceId: string): string {
  return `${KEY_PREFIX}.v${NETWORK_LAYOUT_VERSION}.${encodeURIComponent(workspaceId)}`
}

export function readNetworkPositions(
  workspaceId: string,
  storage: Storage = localStorage,
): NetworkPositions {
  try {
    const raw = storage.getItem(networkLayoutKey(workspaceId))
    if (!raw) return {}
    const parsed = JSON.parse(raw) as Partial<StoredNetworkLayout>
    if (parsed.v !== NETWORK_LAYOUT_VERSION || !isRecord(parsed.positions)) return {}

    const positions: NetworkPositions = {}
    for (const [id, position] of Object.entries(parsed.positions)) {
      if (
        isRecord(position) &&
        typeof position.x === 'number' &&
        Number.isFinite(position.x) &&
        typeof position.y === 'number' &&
        Number.isFinite(position.y)
      ) {
        positions[id] = { x: position.x, y: position.y }
      }
    }
    return positions
  } catch {
    return {}
  }
}

export function writeNetworkPositions(
  workspaceId: string,
  positions: NetworkPositions,
  storage: Storage = localStorage,
  canonicalNodeIds?: Iterable<string>,
  knownPositions?: NetworkPositions,
): NetworkPositions {
  const latest = readNetworkPositions(workspaceId, storage)
  const merged = { ...latest }
  for (const [id, position] of Object.entries(positions)) {
    const known = knownPositions?.[id]
    const current = latest[id]
    // A missing current value that existed in this writer's baseline is an
    // observed deletion from another tab. Do not resurrect it from a stale
    // local snapshot.
    if (known && !current) continue
    // A snapshot unchanged since this writer's read cannot overwrite a value
    // another tab changed in the meantime.
    if (known && current && samePosition(position, known) && !samePosition(current, known)) continue
    merged[id] = position
  }
  const next = canonicalNodeIds
    ? knownPositions
      ? pruneKnownNetworkPositions(merged, canonicalNodeIds, latest, knownPositions)
      : pruneNetworkPositions(merged, canonicalNodeIds)
    : merged
  try {
    storage.setItem(
      networkLayoutKey(workspaceId),
      JSON.stringify({ v: NETWORK_LAYOUT_VERSION, positions: next } satisfies StoredNetworkLayout),
    )
  } catch {
    // Persistence is optional; storage can be unavailable or over quota.
    return latest
  }
  return next
}

function pruneKnownNetworkPositions(
  positions: NetworkPositions,
  nodeIds: Iterable<string>,
  latest: NetworkPositions,
  knownPositions: NetworkPositions,
): NetworkPositions {
  // localStorage has no atomic compare-and-swap. This protects changes visible
  // at read time; an exactly simultaneous setItem can still be last-writer-wins.
  const currentIds = new Set(nodeIds)
  return Object.fromEntries(
    Object.entries(positions).filter(([id]) => {
      if (currentIds.has(id)) return true
      const known = knownPositions[id]
      const current = latest[id]
      return !known || !current || !samePosition(current, known)
    }),
  )
}

function samePosition(a: NetworkPosition, b: NetworkPosition): boolean {
  return a.x === b.x && a.y === b.y
}

export function pruneNetworkPositions(
  positions: NetworkPositions,
  nodeIds: Iterable<string>,
): NetworkPositions {
  const currentIds = new Set(nodeIds)
  return Object.fromEntries(Object.entries(positions).filter(([id]) => currentIds.has(id)))
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

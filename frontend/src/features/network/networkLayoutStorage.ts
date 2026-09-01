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
): NetworkPositions {
  const merged = { ...readNetworkPositions(workspaceId, storage), ...positions }
  const next = canonicalNodeIds ? pruneNetworkPositions(merged, canonicalNodeIds) : merged
  try {
    storage.setItem(
      networkLayoutKey(workspaceId),
      JSON.stringify({ v: NETWORK_LAYOUT_VERSION, positions: next } satisfies StoredNetworkLayout),
    )
  } catch {
    // Persistence is optional; storage can be unavailable or over quota.
  }
  return next
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

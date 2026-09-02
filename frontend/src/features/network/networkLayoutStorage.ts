export const NETWORK_LAYOUT_VERSION = 1

const KEY_PREFIX = 'tionharness.network-layout'

export interface NetworkPosition {
  x: number
  y: number
  vx?: number
  vy?: number
}

export type NetworkPositions = Record<string, NetworkPosition>

export interface NetworkViewport {
  scale: number
  position: { x: number; y: number }
}

export interface NetworkLayout {
  positions: NetworkPositions
  physicsActive: boolean
  viewport?: NetworkViewport
}

interface StoredNetworkLayout extends NetworkLayout {
  v: number
}

export function networkLayoutKey(workspaceId: string): string {
  return `${KEY_PREFIX}.v${NETWORK_LAYOUT_VERSION}.${encodeURIComponent(workspaceId)}`
}

export function readNetworkLayout(
  workspaceId: string,
  storage: Storage = localStorage,
): NetworkLayout {
  try {
    const raw = storage.getItem(networkLayoutKey(workspaceId))
    if (!raw) return { positions: {}, physicsActive: false }
    const parsed = JSON.parse(raw) as Partial<StoredNetworkLayout>
    if (
      parsed.v !== NETWORK_LAYOUT_VERSION ||
      !isRecord(parsed.positions) ||
      (parsed.physicsActive !== undefined && typeof parsed.physicsActive !== 'boolean')
    ) {
      return { positions: {}, physicsActive: false }
    }

    const positions: NetworkPositions = {}
    for (const [id, position] of Object.entries(parsed.positions)) {
      if (
        isRecord(position) &&
        typeof position.x === 'number' &&
        Number.isFinite(position.x) &&
        typeof position.y === 'number' &&
        Number.isFinite(position.y)
      ) {
        const velocityIsValid =
          typeof position.vx === 'number' &&
          Number.isFinite(position.vx) &&
          typeof position.vy === 'number' &&
          Number.isFinite(position.vy)
        positions[id] = velocityIsValid
          ? { x: position.x, y: position.y, vx: position.vx, vy: position.vy }
          : { x: position.x, y: position.y }
      }
    }
    const viewport = readViewport(parsed.viewport)
    return {
      positions,
      physicsActive: parsed.physicsActive === true,
      ...(viewport ? { viewport } : {}),
    }
  } catch {
    return { positions: {}, physicsActive: false }
  }
}

export function readNetworkPositions(
  workspaceId: string,
  storage: Storage = localStorage,
): NetworkPositions {
  return Object.fromEntries(
    Object.entries(readNetworkLayout(workspaceId, storage).positions).map(([id, position]) => [
      id,
      { x: position.x, y: position.y },
    ]),
  )
}

export function writeNetworkPositions(
  workspaceId: string,
  positions: NetworkPositions,
  storage: Storage = localStorage,
  canonicalNodeIds?: Iterable<string>,
  knownPositions?: NetworkPositions,
): NetworkPositions {
  const latestLayout = readNetworkLayout(workspaceId, storage)
  return writeNetworkLayout(
    workspaceId,
    {
      positions,
      physicsActive: latestLayout.physicsActive,
      viewport: latestLayout.viewport,
    },
    storage,
    canonicalNodeIds,
    knownPositions,
    latestLayout,
  ).positions
}

export function writeNetworkLayout(
  workspaceId: string,
  layout: NetworkLayout,
  storage: Storage = localStorage,
  canonicalNodeIds?: Iterable<string>,
  knownPositions?: NetworkPositions,
  latestLayout: NetworkLayout = readNetworkLayout(workspaceId, storage),
): NetworkLayout {
  const latest = latestLayout.positions
  const merged = { ...latest }
  for (const [id, position] of Object.entries(layout.positions)) {
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
      JSON.stringify({
        v: NETWORK_LAYOUT_VERSION,
        positions: next,
        physicsActive: layout.physicsActive,
        viewport: layout.viewport,
      } satisfies StoredNetworkLayout),
    )
  } catch {
    // Persistence is optional; storage can be unavailable or over quota.
    return latestLayout
  }
  return {
    positions: next,
    physicsActive: layout.physicsActive,
    ...(layout.viewport ? { viewport: layout.viewport } : {}),
  }
}

function readViewport(value: unknown): NetworkViewport | undefined {
  if (value === undefined) return undefined
  if (
    !isRecord(value) ||
    typeof value.scale !== 'number' ||
    !Number.isFinite(value.scale) ||
    value.scale <= 0 ||
    !isRecord(value.position) ||
    typeof value.position.x !== 'number' ||
    !Number.isFinite(value.position.x) ||
    typeof value.position.y !== 'number' ||
    !Number.isFinite(value.position.y)
  ) {
    return undefined
  }
  return { scale: value.scale, position: { x: value.position.x, y: value.position.y } }
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
  return a.x === b.x && a.y === b.y && a.vx === b.vx && a.vy === b.vy
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

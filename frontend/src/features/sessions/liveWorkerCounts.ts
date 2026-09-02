export interface SessionLineage {
  id: string
  coordinatorSessionId?: string
  rootCoordinatorSessionId?: string
}

export interface RuntimeLineage {
  running: boolean
  coordinatorSessionId?: string
  rootCoordinatorSessionId?: string
}

interface LineageNode {
  coordinatorSessionId?: string
  rootCoordinatorSessionId?: string
}

// Count every live worker once for each coordinator above it. Runtime entries
// cover the whole workspace; loaded sessions preserve compatibility with an
// older server and make local streaming state visible before the next poll.
export function countLiveDescendantWorkers(
  sessions: readonly SessionLineage[],
  runtimeById?: ReadonlyMap<string, RuntimeLineage>,
  streamingSessionIds?: ReadonlySet<string>,
): Map<string, number> {
  const lineageById = new Map<string, LineageNode>()
  for (const session of sessions) {
    lineageById.set(session.id, {
      coordinatorSessionId: session.coordinatorSessionId,
      rootCoordinatorSessionId: session.rootCoordinatorSessionId,
    })
  }

  const liveSessionIds = new Set<string>(streamingSessionIds)
  for (const [sessionId, runtime] of runtimeById ?? []) {
    const fallback = lineageById.get(sessionId)
    lineageById.set(sessionId, {
      coordinatorSessionId: runtime.coordinatorSessionId ?? fallback?.coordinatorSessionId,
      rootCoordinatorSessionId:
        runtime.rootCoordinatorSessionId ?? fallback?.rootCoordinatorSessionId,
    })
    if (runtime.running) liveSessionIds.add(sessionId)
  }

  const counts = new Map<string, number>()
  const increment = (sessionId: string) => counts.set(sessionId, (counts.get(sessionId) ?? 0) + 1)

  for (const liveSessionId of liveSessionIds) {
    const liveNode = lineageById.get(liveSessionId)
    if (!liveNode?.coordinatorSessionId) continue

    const seen = new Set([liveSessionId])
    let parentSessionId: string | undefined = liveNode.coordinatorSessionId
    let rootSessionId = liveNode.rootCoordinatorSessionId

    while (parentSessionId && !seen.has(parentSessionId)) {
      increment(parentSessionId)
      seen.add(parentSessionId)

      const parent = lineageById.get(parentSessionId)
      if (!parent) break
      rootSessionId = parent.rootCoordinatorSessionId ?? rootSessionId
      parentSessionId = parent.coordinatorSessionId
    }

    // Corrupt or partially migrated lineages can miss an intermediate node. The
    // stamped root still keeps the top-level coordinator's busy state truthful.
    if (rootSessionId && !seen.has(rootSessionId)) increment(rootSessionId)
  }

  return counts
}

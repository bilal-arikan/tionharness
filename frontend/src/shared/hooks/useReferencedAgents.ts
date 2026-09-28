import { useEffect, useMemo, useState } from 'react'
import { api } from '@/api'
import type { Agent } from '@/types'
import { useKeyedReset } from '@/shared/lib/useKeyedReset'

// Resolve only the authors needed by the current view. Archive records never
// enter the default roster, and references are released when the view changes.
export function useReferencedAgents(
  agents: Agent[],
  ids: (string | null | undefined)[],
  scope: string | null,
) {
  const missing = [...new Set(ids.filter((id): id is string => !!id && id !== '*'))]
    .filter((id) => !agents.some((agent) => agent.id === id))
    .sort()
  const key = JSON.stringify([scope, missing])
  const [resolved, setResolved] = useState<{ key: string; agents: Agent[] }>({
    key: '',
    agents: [],
  })
  useKeyedReset(key, () => setResolved({ key, agents: [] }))
  useEffect(() => {
    const [, wanted] = JSON.parse(key) as [string | null, string[]]
    if (!scope || wanted.length === 0) return
    let cancelled = false
    Promise.all(wanted.map((id) => api.getAgent(id).catch(() => null))).then((rows) => {
      if (!cancelled) setResolved({ key, agents: rows.filter((row): row is Agent => row !== null) })
    })
    return () => {
      cancelled = true
    }
  }, [key, scope])
  return useMemo(
    () => (resolved.key === key ? [...agents, ...resolved.agents] : agents),
    [agents, key, resolved],
  )
}

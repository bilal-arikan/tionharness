import { useCallback, useEffect, useMemo, useState } from 'react'

// Collapsed nodes of the map: a node the user folded from the side panel hides
// everything only it reaches (its subtree). Persisted per browser and
// workspace so a fold survives leaving the screen, like the layout does.
function explorerCollapseKey(workspaceId: string): string {
  return `tionharness.explorerCollapsed.${encodeURIComponent(workspaceId)}`
}

function parseCollapsed(raw: string | null | undefined): string[] {
  if (!raw) return []
  try {
    const parsed: unknown = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter((v): v is string => typeof v === 'string') : []
  } catch {
    return []
  }
}

export interface ExplorerCollapseState {
  collapsed: ReadonlySet<string>
  isCollapsed: (key: string) => boolean
  toggle: (key: string) => void
}

export function useExplorerCollapse(workspaceId: string): ExplorerCollapseState {
  const storageKey = explorerCollapseKey(workspaceId)
  const [keys, setKeys] = useState<string[]>(() => {
    try {
      return parseCollapsed(globalThis.localStorage?.getItem(storageKey))
    } catch {
      return []
    }
  })
  // A workspace switch re-reads that workspace's own list.
  useEffect(() => {
    try {
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setKeys(parseCollapsed(globalThis.localStorage?.getItem(storageKey)))
    } catch {
      setKeys([])
    }
  }, [storageKey])
  useEffect(() => {
    try {
      if (keys.length === 0) globalThis.localStorage?.removeItem(storageKey)
      else globalThis.localStorage?.setItem(storageKey, JSON.stringify(keys))
    } catch {
      // Persistence is best-effort when storage is blocked by browser policy.
    }
  }, [storageKey, keys])

  const collapsed = useMemo(() => new Set(keys), [keys])
  const isCollapsed = useCallback((key: string) => collapsed.has(key), [collapsed])
  const toggle = useCallback(
    (key: string) =>
      setKeys((current) =>
        current.includes(key) ? current.filter((k) => k !== key) : [...current, key],
      ),
    [],
  )
  return { collapsed, isCollapsed, toggle }
}

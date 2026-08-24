import { useCallback, useEffect, useMemo, useState } from 'react'

export interface UseGroupedListOptions<T> {
  // Group key for one item. Items sharing a key land in the same bucket.
  keyOf: (item: T) => string
  // Optional comparator ordering the emitted [key, items] pairs. When omitted,
  // buckets keep first-seen insertion order.
  sortGroups?: (a: string, b: string) => number
  // When set, collapse state is persisted to localStorage under this key.
  persistKey?: string
}

export interface GroupedList<T> {
  // Ordered [groupKey, items] pairs (bucketing preserves within-group order).
  groups: Array<[string, T[]]>
  // Distinct group keys, in emission order.
  groupNames: string[]
  // Currently collapsed (folded-in) group keys.
  collapsed: Set<string>
  isCollapsed: (name: string) => boolean
  toggle: (name: string) => void
  // True when every group is collapsed (and there is at least one group).
  allCollapsed: boolean
  // Collapse-all when any group is open, otherwise expand-all.
  toggleAll: () => void
}

function loadPersisted(key: string | undefined): Set<string> {
  if (!key) return new Set()
  try {
    const raw = localStorage.getItem(key)
    return new Set(raw ? (JSON.parse(raw) as string[]) : [])
  } catch {
    return new Set()
  }
}

// useGroupedList buckets a flat list by a key, tracks per-group collapse state
// (optionally persisted to localStorage), and exposes toggle helpers. The
// bucketing preserves the incoming order within each group.
export function useGroupedList<T>(list: T[], opts: UseGroupedListOptions<T>): GroupedList<T> {
  const { keyOf, sortGroups, persistKey } = opts

  const [collapsed, setCollapsed] = useState<Set<string>>(() => loadPersisted(persistKey))

  useEffect(() => {
    if (!persistKey) return
    localStorage.setItem(persistKey, JSON.stringify([...collapsed]))
  }, [collapsed, persistKey])

  const groups = useMemo(() => {
    const buckets = new Map<string, T[]>()
    for (const item of list) {
      const key = keyOf(item)
      const arr = buckets.get(key)
      if (arr) arr.push(item)
      else buckets.set(key, [item])
    }
    const entries = [...buckets.entries()]
    if (sortGroups) entries.sort(([a], [b]) => sortGroups(a, b))
    return entries
  }, [list, keyOf, sortGroups])

  const groupNames = useMemo(() => groups.map(([name]) => name), [groups])

  const toggle = useCallback((name: string) => {
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }, [])

  const isCollapsed = useCallback((name: string) => collapsed.has(name), [collapsed])

  const allCollapsed = groups.length > 0 && groups.every(([name]) => collapsed.has(name))

  const toggleAll = useCallback(() => {
    setCollapsed(() => (allCollapsed ? new Set() : new Set(groups.map(([name]) => name))))
  }, [allCollapsed, groups])

  return { groups, groupNames, collapsed, isCollapsed, toggle, allCollapsed, toggleAll }
}

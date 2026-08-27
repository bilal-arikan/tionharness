// useExplorerGraph owns the Explorer map's node/edge state: which nodes are
// expanded, their fetched children, the current selection, and the lazy fetch +
// cycle-safe layout. The screen (ExplorerView) is a thin consumer.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { ViewRef } from '@/types'
import { parseRef, refToString } from '@/types'
import { buildGraph, isDrillable, nextExpandedSet, ROOT_KEY, ROOT_REF } from './explorerModel'

interface Options {
  // search dims non-matching nodes (focus+context); empty = no search.
  search: string
  onError?: (msg: string) => void
  // initialSelected restores a deep-linked selection (a ref string) on mount.
  initialSelected?: string | null
  // onSelect fires when the selected node changes, so the URL can track it.
  onSelect?: (refString: string) => void
}

export function useExplorerGraph({ search, onError, initialSelected, onSelect }: Options) {
  // refByKey/labelByKey/childrenByKey grow as nodes are fetched. Seeded with the
  // root so the workspace node renders (and is selectable) before any fetch.
  const [refByKey, setRefByKey] = useState<Record<string, ViewRef>>({ [ROOT_KEY]: ROOT_REF })
  const [labelByKey, setLabelByKey] = useState<Record<string, string>>({ [ROOT_KEY]: 'Workspace' })
  const [childrenByKey, setChildrenByKey] = useState<Record<string, ViewRef[]>>({})
  // parentByKey maps each fetched node's ref-string to its parent's ref-string —
  // the sibling relation the single-expand (accordion) rule needs. It grows in
  // fetchChildren, so it is complete for every expandable node (a node can only
  // be expanded after its parent's children arrived).
  const [parentByKey, setParentByKey] = useState<Record<string, string>>({})
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState<Set<string>>(new Set())
  // Seed the selection from a deep link when the ref string parses; otherwise root.
  const [selectedKey, setSelectedKey] = useState<string>(() => {
    const parsed = initialSelected ? parseRef(initialSelected) : null
    return parsed ? refToString(parsed) : ROOT_KEY
  })

  // Live mirrors of the current expanded set + refs, so refreshExpanded can re-fetch
  // open branches without being re-created on every expand (it is wired to an SSE
  // tick). Updated in effects (never during render) per the refs lint rule.
  const expandedRef = useRef(expanded)
  const refByKeyRef = useRef(refByKey)
  const parentByKeyRef = useRef(parentByKey)
  useEffect(() => {
    expandedRef.current = expanded
  }, [expanded])
  useEffect(() => {
    refByKeyRef.current = refByKey
  }, [refByKey])
  useEffect(() => {
    parentByKeyRef.current = parentByKey
  }, [parentByKey])

  const fetchChildren = useCallback(
    async (ref: ViewRef) => {
      const key = refToString(ref)
      setLoading((s) => new Set(s).add(key))
      try {
        const res = await api.viewChildren(ref)
        const childRefs = res.children.map((h) => h.ref)
        setChildrenByKey((m) => ({ ...m, [key]: childRefs }))
        setParentByKey((m) => {
          const n = { ...m }
          for (const c of childRefs) n[refToString(c)] = key
          return n
        })
        setRefByKey((m) => {
          const n = { ...m }
          for (const h of res.children) n[refToString(h.ref)] = h.ref
          return n
        })
        setLabelByKey((m) => {
          const n = { ...m }
          for (const h of res.children) n[refToString(h.ref)] = h.label
          return n
        })
      } catch (e) {
        // Surface the failure rather than leaving a node silently un-expanded.
        onError?.(e instanceof Error ? e.message : String(e))
      } finally {
        setLoading((s) => {
          const n = new Set(s)
          n.delete(key)
          return n
        })
      }
    },
    [onError],
  )

  // toggle drills one layer in (fetching children on first expand) or collapses.
  // A leaf kind only selects — there is nothing to expand into. Expanding
  // applies the accordion rule: every other open sibling closes first.
  const toggle = useCallback(
    (ref: ViewRef) => {
      const key = refToString(ref)
      setSelectedKey(key)
      onSelect?.(key)
      if (!isDrillable(ref)) return
      setExpanded((prev) => {
        const next = nextExpandedSet(prev, key, parentByKeyRef.current)
        if (next.has(key) && !childrenByKey[key]) void fetchChildren(ref)
        return next
      })
    },
    [childrenByKey, fetchChildren, onSelect],
  )

  // refreshExpanded re-fetches every currently-expanded node's children. Used on
  // an 'explorer' SSE tick
  // (only open branches are refreshed, not the whole map).
  const refreshExpanded = useCallback(() => {
    for (const key of expandedRef.current) {
      const ref = refByKeyRef.current[key]
      if (ref) void fetchChildren(ref)
    }
  }, [fetchChildren])

  // reload seeds/refreshes the map: auto-expand the root (so the eleven buckets
  // are visible immediately, not a lone workspace node), drop stale child caches,
  // and re-fetch every open branch. Runs on mount.
  const reload = useCallback(() => {
    setChildrenByKey({})
    setExpanded((prev) => (prev.has(ROOT_KEY) ? prev : new Set(prev).add(ROOT_KEY)))
    void fetchChildren(ROOT_REF)
    for (const key of expandedRef.current) {
      if (key === ROOT_KEY) continue
      const ref = refByKeyRef.current[key]
      if (ref) void fetchChildren(ref)
    }
  }, [fetchChildren])
  useEffect(() => {
    // Fetch-on-mount synchronization; reload seeds the root and re-fetches open
    // branches.
    // eslint-disable-next-line react-hooks/set-state-in-effect
    reload()
  }, [reload])

  const { nodes, edges } = useMemo(
    () =>
      buildGraph({ refByKey, labelByKey, childrenByKey, expanded, loading, selectedKey, search }),
    [refByKey, labelByKey, childrenByKey, expanded, loading, selectedKey, search],
  )

  const selectedRef = refByKey[selectedKey] ?? parseRef(selectedKey) ?? ROOT_REF

  return { nodes, edges, toggle, selectedRef, refreshExpanded }
}

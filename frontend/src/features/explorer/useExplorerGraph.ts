import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { ViewNeighborhoodResult, ViewRef } from '@/types'
import { parseRef, refToString } from '@/types'
import { buildFocusGraph, ROOT_REF } from './explorerModel'

interface Options {
  search: string
  onError?: (msg: string) => void
  initialFocus?: string | null
  onFocus?: (refString: string) => void
}

interface CacheEntry {
  data?: ViewNeighborhoodResult
  loading: boolean
  error?: string
}

export function useExplorerGraph({ search, onError, initialFocus, onFocus }: Options) {
  const initialRef = () => (initialFocus ? parseRef(initialFocus) : null) ?? ROOT_REF
  const [focusRef, setFocusRef] = useState<ViewRef>(initialRef)
  const [selectedRef, setSelectedRef] = useState<ViewRef>(initialRef)
  const [cache, setCache] = useState<Record<string, CacheEntry>>({})
  const cacheRef = useRef(cache)
  const activeRequest = useRef<AbortController | null>(null)
  const sequenceByKey = useRef<Record<string, number>>({})
  useEffect(() => {
    cacheRef.current = cache
  }, [cache])

  const fetchFocus = useCallback(
    async (ref: ViewRef, force = false) => {
      const key = refToString(ref)
      if (!force && cacheRef.current[key]?.data) return
      activeRequest.current?.abort()
      const controller = new AbortController()
      activeRequest.current = controller
      const sequence = (sequenceByKey.current[key] ?? 0) + 1
      sequenceByKey.current[key] = sequence
      setCache((current) => ({
        ...current,
        [key]: { ...current[key], loading: true, error: undefined },
      }))
      try {
        const data = await api.viewNeighborhood(ref, controller.signal)
        if (sequenceByKey.current[key] !== sequence) return
        setCache((current) => ({ ...current, [key]: { data, loading: false } }))
      } catch (error) {
        if (controller.signal.aborted || sequenceByKey.current[key] !== sequence) return
        const message = error instanceof Error ? error.message : String(error)
        setCache((current) => ({ ...current, [key]: { loading: false, error: message } }))
        onError?.(message)
      }
    },
    [onError],
  )

  useEffect(() => {
    // eslint-disable-next-line react-hooks/set-state-in-effect
    void fetchFocus(focusRef)
    return () => activeRequest.current?.abort()
  }, [fetchFocus, focusRef])

  const select = useCallback((ref: ViewRef) => setSelectedRef(ref), [])
  const focus = useCallback(
    (ref: ViewRef) => {
      setFocusRef(ref)
      setSelectedRef(ref)
      onFocus?.(refToString(ref))
    },
    [onFocus],
  )
  const refreshFocused = useCallback(() => void fetchFocus(focusRef, true), [fetchFocus, focusRef])

  const focusKey = refToString(focusRef)
  const entry = cache[focusKey]
  const graph = useMemo(
    () =>
      entry?.data
        ? buildFocusGraph({
            neighborhood: entry.data,
            selectedKey: refToString(selectedRef),
            search,
            loading: entry.loading,
          })
        : { nodes: [], edges: [] },
    [entry, search, selectedRef],
  )

  return {
    ...graph,
    select,
    focus,
    selectedRef,
    focusRef,
    focusLoading: entry?.loading ?? false,
    focusError: entry?.error,
    refreshFocused,
  }
}

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { ViewNeighborhoodResult, ViewRef } from '@/types'
import { parseRef, refToString } from '@/types'
import { buildFocusGraph, ROOT_REF } from './explorerModel'

interface Options {
  search: string
  onError?: (msg: string) => void
  initialFocus?: string | null
  onFocus?: (refString: string | null) => void
}

interface CacheEntry {
  data?: ViewNeighborhoodResult
  loading: boolean
  error?: string
}

export function useExplorerGraph({ search, onError, initialFocus, onFocus }: Options) {
  const parsedInitialFocus = initialFocus ? parseRef(initialFocus) : ROOT_REF
  const initialRef = () => parsedInitialFocus ?? ROOT_REF
  const [focusRef, setFocusRef] = useState<ViewRef>(initialRef)
  const [selectedRef, setSelectedRef] = useState<ViewRef>(initialRef)
  const [deepLinkError, setDeepLinkError] = useState<string | undefined>(() =>
    initialFocus && !parsedInitialFocus ? `Geçersiz odak bağlantısı: ${initialFocus}` : undefined,
  )
  const [cache, setCache] = useState<Record<string, CacheEntry>>({})
  const cacheRef = useRef(cache)
  const activeRequest = useRef<AbortController | null>(null)
  const sequenceByKey = useRef<Record<string, number>>({})
  useEffect(() => {
    cacheRef.current = cache
  }, [cache])

  useEffect(() => {
    const next = initialFocus ? parseRef(initialFocus) : ROOT_REF
    if (!next) {
      // URL navigation is external state; mirror it atomically into graph state.
      // eslint-disable-next-line react-hooks/set-state-in-effect
      setDeepLinkError(`Geçersiz odak bağlantısı: ${initialFocus}`)
      setFocusRef((current) =>
        refToString(current) === refToString(ROOT_REF) ? current : ROOT_REF,
      )
      setSelectedRef((current) =>
        refToString(current) === refToString(ROOT_REF) ? current : ROOT_REF,
      )
      return
    }
    setDeepLinkError(undefined)
    setFocusRef((current) => (refToString(current) === refToString(next) ? current : next))
    setSelectedRef((current) => (refToString(current) === refToString(next) ? current : next))
  }, [initialFocus])

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
      setDeepLinkError(undefined)
      setFocusRef(ref)
      setSelectedRef(ref)
      onFocus?.(refToString(ref) === refToString(ROOT_REF) ? null : refToString(ref))
    },
    [onFocus],
  )
  const fallbackToRoot = useCallback(() => focus(ROOT_REF), [focus])
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
    deepLinkError,
    fallbackToRoot,
    refreshFocused,
  }
}

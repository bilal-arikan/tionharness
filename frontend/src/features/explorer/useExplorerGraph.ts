import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { ViewHandle, ViewNeighborhoodResult, ViewRef } from '@/types'
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
  const [lineage, setLineage] = useState<ViewHandle[]>(() => [
    { ref: initialRef(), label: initialRef().id },
  ])
  const focusRefCurrent = useRef(focusRef)
  const lineageRef = useRef(lineage)
  const [deepLinkError, setDeepLinkError] = useState<string | undefined>(() =>
    initialFocus && !parsedInitialFocus ? `Geçersiz odak bağlantısı: ${initialFocus}` : undefined,
  )
  const [cache, setCache] = useState<Record<string, CacheEntry>>({})
  const cacheRef = useRef(cache)
  const activeRequest = useRef<AbortController | null>(null)
  const deepLinkRequest = useRef<AbortController | null>(null)
  const deepLinkSequence = useRef(0)
  const sequenceByKey = useRef<Record<string, number>>({})
  useEffect(() => {
    cacheRef.current = cache
  }, [cache])

  useEffect(() => {
    deepLinkRequest.current?.abort()
    const sequence = deepLinkSequence.current + 1
    deepLinkSequence.current = sequence
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
      setLineage([{ ref: ROOT_REF, label: ROOT_REF.id }])
      focusRefCurrent.current = ROOT_REF
      lineageRef.current = [{ ref: ROOT_REF, label: ROOT_REF.id }]
      return
    }
    setDeepLinkError(undefined)
    setFocusRef((current) => (refToString(current) === refToString(next) ? current : next))
    setSelectedRef((current) => (refToString(current) === refToString(next) ? current : next))
    focusRefCurrent.current = next
    if (!initialFocus || refToString(next) === refToString(ROOT_REF)) {
      const rootLineage = [{ ref: ROOT_REF, label: ROOT_REF.id }]
      setLineage(rootLineage)
      lineageRef.current = rootLineage
      return
    }

    const controller = new AbortController()
    deepLinkRequest.current = controller
    const targetLineage = [{ ref: next, label: next.id }]
    setLineage(targetLineage)
    lineageRef.current = targetLineage

    const resolveLineage = async () => {
      const path = new Set<string>()
      const findRootPath = async (ref: ViewRef): Promise<ViewHandle[] | null> => {
        const key = refToString(ref)
        if (key === refToString(ROOT_REF)) return [{ ref: ROOT_REF, label: ROOT_REF.id }]
        if (path.has(key)) return null
        path.add(key)
        try {
          const cached = cacheRef.current[key]?.data
          const data = cached ?? (await api.viewNeighborhood(ref, controller.signal))
          if (controller.signal.aborted || deepLinkSequence.current !== sequence) return null
          if (!cached) {
            cacheRef.current = { ...cacheRef.current, [key]: { data, loading: false } }
            setCache(cacheRef.current)
          }
          const parents = [...data.parents].sort((a, b) =>
            refToString(a.ref).localeCompare(refToString(b.ref)),
          )
          for (const parent of parents) {
            const parentPath = await findRootPath(parent.ref)
            if (parentPath) return [...parentPath, data.focus]
          }
          return null
        } finally {
          path.delete(key)
        }
      }

      try {
        const resolved = await findRootPath(next)
        if (controller.signal.aborted || deepLinkSequence.current !== sequence) return
        if (!resolved) throw new Error(`Odak köke bağlanamadı: ${refToString(next)}`)
        lineageRef.current = resolved
        setLineage(resolved)
      } catch (error) {
        if (controller.signal.aborted || deepLinkSequence.current !== sequence) return
        const message = error instanceof Error ? error.message : String(error)
        setDeepLinkError(message)
        onError?.(message)
      }
    }
    void resolveLineage()
    return () => controller.abort()
  }, [initialFocus, onError])

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
      deepLinkRequest.current?.abort()
      deepLinkSequence.current += 1
      const nextKey = refToString(ref)
      const currentKey = refToString(focusRefCurrent.current)
      const currentData = cacheRef.current[currentKey]?.data
      const current = lineageRef.current
      const nextLineage = (() => {
        const existingIndex = current.findIndex((item) => refToString(item.ref) === nextKey)
        if (existingIndex >= 0) return current.slice(0, existingIndex + 1)

        const target = [...(currentData?.parents ?? []), ...(currentData?.children ?? [])].find(
          (item) => refToString(item.ref) === nextKey,
        )
        const currentIndex = current.findIndex((item) => refToString(item.ref) === currentKey)
        if (target && currentIndex === current.length - 1) return [...current, target]
        return [{ ref, label: target?.label || ref.id }]
      })()
      lineageRef.current = nextLineage
      focusRefCurrent.current = ref
      setLineage(nextLineage)
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
            lineage,
            selectedKey: refToString(selectedRef),
            search,
            loading: entry.loading,
          })
        : { nodes: [], edges: [] },
    [entry, lineage, search, selectedRef],
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

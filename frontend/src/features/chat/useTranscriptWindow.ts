import { useCallback, useLayoutEffect, useMemo, useRef, useState, type RefObject } from 'react'
import type { Message } from '@/types'

const ESTIMATED_HEIGHT = 320
const OVERSCAN = 900

export function indexAtOffset(offsets: number[], position: number): number {
  let lo = 0,
    hi = Math.max(0, offsets.length - 2)
  while (lo < hi) {
    const mid = (lo + hi + 1) >>> 1
    if (offsets[mid] <= position) lo = mid
    else hi = mid - 1
  }
  return lo
}

// Variable-height rows share one observer. Only the viewport plus overscan is
// mounted; heights are keyed by message identity and discarded on removal.
export function useTranscriptWindow(
  messages: Message[],
  scrollRef: RefObject<HTMLDivElement | null>,
) {
  const heights = useRef(new Map<string, number>())
  const nodes = useRef(new Set<HTMLElement>())
  const observer = useRef<ResizeObserver | null>(null)
  const anchor = useRef<{ id: string; top: number } | null>(null)
  const anchoredScrollTop = useRef(0)
  const [revision, setRevision] = useState(0)
  const [viewport, setViewport] = useState({ top: 0, height: 800 })
  const ids = useMemo(() => messages.map((m) => m.id), [messages])
  const offsets = useMemo(() => {
    const result = [0]
    for (const id of ids)
      result.push(result.at(-1)! + (heights.current.get(id) ?? ESTIMATED_HEIGHT))
    return result
  }, [ids, revision])
  const layout = useRef({ ids, offsets })
  const updateViewport = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    setViewport((previous) =>
      previous.top === el.scrollTop && previous.height === el.clientHeight
        ? previous
        : { top: el.scrollTop, height: el.clientHeight || 800 },
    )
  }, [scrollRef])

  useLayoutEffect(() => {
    const el = scrollRef.current
    const old = layout.current
    // Preserve the visible message and its intra-row offset when a page is
    // prepended or the bounded window drops rows from its opposite edge.
    if (el && !anchor.current && old.ids[0] !== ids[0] && old.ids.length && ids.length) {
      const oldIndex = indexAtOffset(old.offsets, el.scrollTop)
      const nextIndex = ids.indexOf(old.ids[oldIndex])
      if (nextIndex >= 0) el.scrollTop += offsets[nextIndex] - old.offsets[oldIndex]
    }
    layout.current = { ids, offsets }
    if (el && anchor.current) {
      const index = ids.indexOf(anchor.current.id)
      if (index < 0) anchor.current = null
      else {
        const row = el.querySelector<HTMLElement>(
          `[data-msg-id="${CSS.escape(anchor.current.id)}"]`,
        )
        if (row)
          el.scrollTop +=
            row.getBoundingClientRect().top - el.getBoundingClientRect().top - anchor.current.top
        else el.scrollTop = offsets[index]
        anchoredScrollTop.current = el.scrollTop
      }
    }
    const present = new Set(ids)
    for (const id of heights.current.keys()) if (!present.has(id)) heights.current.delete(id)
    updateViewport()
  }, [ids, offsets, scrollRef, updateViewport, viewport.top, viewport.height])

  const measureNodes = useCallback(
    (elements: HTMLElement[]) => {
      const el = scrollRef.current
      const current = layout.current
      let changed = false,
        adjustment = 0
      const pinned = !!el && el.scrollHeight - el.scrollTop - el.clientHeight < 80
      for (const node of elements) {
        const id = node.dataset.virtualId
        const height = node.getBoundingClientRect().height
        if (!id || height <= 0) continue
        const previous = heights.current.get(id) ?? ESTIMATED_HEIGHT
        if (Math.abs(previous - height) < 1) continue
        const index = current.ids.indexOf(id)
        if (el && !anchor.current && !pinned && index >= 0 && current.offsets[index] < el.scrollTop)
          adjustment += height - previous
        heights.current.set(id, height)
        changed = true
      }
      if (changed) {
        if (el && adjustment) el.scrollTop += adjustment
        setRevision((n) => n + 1)
        updateViewport()
      }
    },
    [scrollRef, updateViewport],
  )

  useLayoutEffect(() => {
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver((entries) =>
      measureNodes(entries.map((e) => e.target as HTMLElement)),
    )
    observer.current = ro
    for (const node of nodes.current) ro.observe(node)
    const el = scrollRef.current
    let width = el?.clientWidth
    const containerObserver = new ResizeObserver(() => {
      if (el && el.clientWidth !== width) {
        width = el.clientWidth
        heights.current.clear()
        setRevision((n) => n + 1)
        measureNodes([...nodes.current])
      }
      updateViewport()
    })
    if (el) containerObserver.observe(el)
    return () => {
      ro.disconnect()
      containerObserver.disconnect()
      observer.current = null
    }
  }, [measureNodes, scrollRef, updateViewport])

  const measure = useCallback(
    (node: HTMLDivElement | null) => {
      // React 19 ref cleanup releases detached nodes immediately.
      if (!node) return
      nodes.current.add(node)
      observer.current?.observe(node)
      measureNodes([node])
      return () => {
        nodes.current.delete(node)
        observer.current?.unobserve(node)
      }
    },
    [measureNodes],
  )
  const scrollToIndex = useCallback(
    (index: number) => {
      anchor.current = null
      const el = scrollRef.current
      if (!el) return
      el.scrollTop = Math.max(0, (layout.current.offsets[index] ?? 0) - 8)
      updateViewport()
    },
    [scrollRef, updateViewport],
  )
  const captureAnchor = useCallback(() => {
    const el = scrollRef.current
    if (!el) return
    const top = el.getBoundingClientRect().top
    const row = [...el.querySelectorAll<HTMLElement>('[data-msg-id]')].find(
      (node) => node.getBoundingClientRect().bottom > top + 1,
    )
    if (row?.dataset.msgId)
      anchor.current = { id: row.dataset.msgId, top: row.getBoundingClientRect().top - top }
    anchoredScrollTop.current = el.scrollTop
  }, [scrollRef])
  const clearAnchor = useCallback(() => {
    anchor.current = null
  }, [])
  const onScroll = useCallback(() => {
    const el = scrollRef.current
    // Scrollbar, keyboard, accessibility tools and script-driven navigation do
    // not necessarily emit a wheel/pointer event. Only our own anchor correction
    // may retain the anchor; every other scroll must release it immediately.
    if (el && anchor.current && Math.abs(el.scrollTop - anchoredScrollTop.current) > 1)
      anchor.current = null
    updateViewport()
  }, [scrollRef, updateViewport])

  const enabled = messages.length >= 30
  const start = enabled ? indexAtOffset(offsets, Math.max(0, viewport.top - OVERSCAN)) : 0
  const end = enabled
    ? Math.min(
        messages.length,
        indexAtOffset(offsets, viewport.top + viewport.height + OVERSCAN) + 1,
      )
    : messages.length
  return {
    start,
    end,
    top: offsets[start] ?? 0,
    bottom: (offsets.at(-1) ?? 0) - (offsets[end] ?? 0),
    total: offsets.at(-1) ?? 0,
    height: viewport.height,
    offsets,
    measure,
    updateViewport,
    scrollToIndex,
    captureAnchor,
    clearAnchor,
    onScroll,
  }
}

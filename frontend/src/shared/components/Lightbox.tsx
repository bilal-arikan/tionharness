import { useCallback, useEffect, useRef, useState, type ReactNode } from 'react'
import { X, ZoomIn, ZoomOut, Maximize2, ChevronLeft, ChevronRight } from 'lucide-react'

export interface LightboxImage {
  src: string
  alt?: string
  type?: 'image' | 'video'
}

interface Props {
  onClose: () => void
  title?: string
  // Single image, an arbitrary child (e.g. an SVG), or a gallery of images with
  // prev/next navigation. Provide exactly one of imageSrc / images / children.
  imageSrc?: string
  imageAlt?: string
  images?: LightboxImage[]
  index?: number
  children?: ReactNode
}

const MIN_SCALE = 0.2
const MAX_SCALE = 8

const clamp = (v: number) => Math.min(MAX_SCALE, Math.max(MIN_SCALE, v))

// Lightbox is a full-screen overlay that shows an image or arbitrary content
// (e.g. a rendered mermaid SVG) with mouse-wheel zoom (toward the cursor),
// drag-to-pan, a zoom toolbar, double-click to toggle, and Escape/backdrop to
// close. Shared by inline chat images, attachment previews, image artifacts and
// the mermaid Expand view so they all behave the same.
export function Lightbox({ onClose, title, imageSrc, imageAlt, images, index, children }: Props) {
  const [scale, setScale] = useState(1)
  const [tx, setTx] = useState(0)
  const [ty, setTy] = useState(0)
  const [dragging, setDragging] = useState(false)
  const [cur, setCur] = useState(index ?? 0)
  const stageRef = useRef<HTMLDivElement>(null)

  const gallery = images && images.length > 0 ? images : null
  const item = gallery ? gallery[Math.min(cur, gallery.length - 1)] : null
  const src = item ? item.src : imageSrc
  const alt = item ? item.alt || '' : imageAlt || ''
  const isVideo = item ? item.type === 'video' : false
  // Drag origin for panning.
  const drag = useRef<{ x: number; y: number; tx: number; ty: number } | null>(null)
  // Whether the pointer moved enough to count as a pan. Kept in its OWN ref that
  // is reset only on the next pointerdown — NOT cleared on pointerup — so the
  // `click` event (which fires after pointerup) can still tell a pan from a plain
  // click. A pan must not close the lightbox; a clean backdrop click must.
  const movedRef = useRef(false)
  // Active pointers on the stage, by pointerId. Two simultaneous pointers turn
  // the gesture into a pinch (zoom + two-finger pan); one is a plain drag-pan.
  const pointers = useRef(new Map<number, { x: number; y: number }>())
  // Baseline captured when the second finger lands: the pinch is applied
  // relative to it, so the transform never accumulates rounding drift.
  const pinch = useRef<{
    dist: number
    cx: number
    cy: number
    scale: number
    tx: number
    ty: number
  } | null>(null)

  // Pinch center/distance in stage coordinates (relative to the stage centre,
  // which is the transform origin).
  const pinchState = () => {
    const [a, b] = Array.from(pointers.current.values())
    const el = stageRef.current
    if (!a || !b || !el) return null
    const rect = el.getBoundingClientRect()
    return {
      dist: Math.hypot(a.x - b.x, a.y - b.y),
      cx: (a.x + b.x) / 2 - (rect.left + rect.width / 2),
      cy: (a.y + b.y) / 2 - (rect.top + rect.height / 2),
    }
  }

  const reset = useCallback(() => {
    setScale(1)
    setTx(0)
    setTy(0)
  }, [])

  // Navigate the gallery (wraps around); resets zoom/pan for the new image.
  const go = useCallback(
    (delta: number) => {
      if (!gallery) return
      setCur((c) => (c + delta + gallery.length) % gallery.length)
      reset()
    },
    [gallery, reset],
  )

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose()
      else if (e.key === '0') reset()
      else if (e.key === 'ArrowLeft') go(-1)
      else if (e.key === 'ArrowRight') go(1)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [onClose, reset, go])

  // Zoom toward the cursor. Registered as a non-passive listener so we can
  // preventDefault and stop the page from scrolling. The translation is adjusted
  // so the point under the cursor stays fixed across the zoom step.
  useEffect(() => {
    const el = stageRef.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      const rect = el.getBoundingClientRect()
      const sx = e.clientX - (rect.left + rect.width / 2)
      const sy = e.clientY - (rect.top + rect.height / 2)
      setScale((prev) => {
        const next = clamp(prev * (e.deltaY < 0 ? 1.15 : 1 / 1.15))
        const ratio = next / prev
        setTx((t) => sx * (1 - ratio) + t * ratio)
        setTy((t) => sy * (1 - ratio) + t * ratio)
        return next
      })
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [])

  const onPointerDown = (e: React.PointerEvent) => {
    if (e.button !== 0) return
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY })
    if (pointers.current.size === 2) {
      // Second finger: switch from pan to pinch. The pan is cancelled but the
      // gesture still counts as "moved", so lifting off must not close.
      const p = pinchState()
      if (p) {
        pinch.current = { ...p, scale, tx, ty }
        drag.current = null
        movedRef.current = true
        setDragging(false)
      }
      return
    }
    drag.current = { x: e.clientX, y: e.clientY, tx, ty }
    movedRef.current = false
    setDragging(true)
    try {
      ;(e.currentTarget as HTMLElement).setPointerCapture?.(e.pointerId)
    } catch {
      // setPointerCapture can throw for a stale/synthetic pointer id — pan still
      // works via pointermove, so ignore.
    }
  }
  const onPointerMove = (e: React.PointerEvent) => {
    if (pointers.current.has(e.pointerId))
      pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY })
    const p0 = pinch.current
    if (p0) {
      const p = pinchState()
      if (!p || p0.dist === 0) return
      const next = clamp(p0.scale * (p.dist / p0.dist))
      const ratio = next / p0.scale
      // Keep the point under the pinch centre fixed, then follow the centre so
      // two fingers can pan while zooming.
      setScale(next)
      setTx(p0.cx * (1 - ratio) + p0.tx * ratio + (p.cx - p0.cx))
      setTy(p0.cy * (1 - ratio) + p0.ty * ratio + (p.cy - p0.cy))
      return
    }
    const d = drag.current
    if (!d) return
    const dx = e.clientX - d.x
    const dy = e.clientY - d.y
    if (!movedRef.current && Math.hypot(dx, dy) > 3) movedRef.current = true
    setTx(d.tx + dx)
    setTy(d.ty + dy)
  }
  const onPointerUp = (e: React.PointerEvent) => {
    pointers.current.delete(e.pointerId)
    if (pointers.current.size < 2) pinch.current = null
    setDragging(false)
    drag.current = null
  }
  // Close on a clean click of the empty stage (no pan). movedRef survives the
  // pointerup→click sequence, so a drag-release is correctly ignored.
  const onStageClick = () => {
    if (!movedRef.current) onClose()
  }

  const zoomBy = (k: number) => setScale((p) => clamp(p * k))

  const btn =
    'flex h-8 w-8 items-center justify-center rounded-md bg-white/10 text-white transition hover:bg-white/20'

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-black/85 backdrop-blur-sm">
      <div
        className="flex items-center justify-between gap-3 px-4 py-2"
        onClick={(e) => e.stopPropagation()}
      >
        <span className="truncate text-xs text-white/50">
          {gallery ? `${cur + 1} / ${gallery.length}${alt ? ` · ${alt}` : ''}` : title}
        </span>
        <div className="flex items-center gap-1">
          <button type="button" className={btn} title="Uzaklaştır" onClick={() => zoomBy(1 / 1.25)}>
            <ZoomOut size={16} />
          </button>
          <span className="w-12 text-center text-xs tabular-nums text-white/70">
            {Math.round(scale * 100)}%
          </span>
          <button type="button" className={btn} title="Yakınlaştır" onClick={() => zoomBy(1.25)}>
            <ZoomIn size={16} />
          </button>
          <button type="button" className={btn} title="Sıfırla (0)" onClick={reset}>
            <Maximize2 size={16} />
          </button>
          <button type="button" className={btn} title="Kapat (Esc)" onClick={onClose}>
            <X size={16} />
          </button>
        </div>
      </div>

      <div
        ref={stageRef}
        className="relative flex-1 overflow-hidden"
        style={{ cursor: dragging ? 'grabbing' : 'grab', touchAction: 'none' }}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
        onClick={onStageClick}
        onDoubleClick={() => (scale === 1 ? zoomBy(2) : reset())}
      >
        <div
          className="absolute left-1/2 top-1/2 select-none"
          style={{ transform: `translate(-50%,-50%) translate(${tx}px,${ty}px) scale(${scale})` }}
          onClick={(e) => e.stopPropagation()}
        >
          {src ? (
            isVideo ? (
              <video
                src={src}
                controls
                autoPlay
                // Let the video's own controls receive clicks instead of starting a pan.
                onPointerDown={(e) => e.stopPropagation()}
                className="max-h-[85vh] max-w-[90vw] object-contain"
              />
            ) : (
              <img
                src={src}
                alt={alt}
                draggable={false}
                className="max-h-[85vh] max-w-[90vw] object-contain"
              />
            )
          ) : (
            children
          )}
        </div>

        {gallery && gallery.length > 1 && (
          <>
            <button
              type="button"
              title="Önceki (←)"
              onClick={(e) => {
                e.stopPropagation()
                go(-1)
              }}
              className="absolute left-3 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/25"
            >
              <ChevronLeft size={22} />
            </button>
            <button
              type="button"
              title="Sonraki (→)"
              onClick={(e) => {
                e.stopPropagation()
                go(1)
              }}
              className="absolute right-3 top-1/2 flex h-10 w-10 -translate-y-1/2 items-center justify-center rounded-full bg-white/10 text-white transition hover:bg-white/25"
            >
              <ChevronRight size={22} />
            </button>
          </>
        )}
      </div>
    </div>
  )
}

import { useMemo, useState } from 'react'
import { Play } from 'lucide-react'
import { Lightbox, type LightboxImage } from '../common'
import { isMediaPath, isVideoPath, mediaUrl } from '../../lib/paths'

interface Props {
  code: string
}

interface RawItem {
  src?: string
  url?: string
  path?: string
  alt?: string
  caption?: string
  label?: string
}

const isExternal = (s: string) => /^(https?:|data:|blob:)/i.test(s)

// Resolve a path/URL to something the <img> can load: external/data/blob URLs
// and already-built /api/files URLs pass through; a bare local image path is
// routed through the backend file server.
function resolveSrc(raw: string): string {
  const s = raw.trim()
  if (!s) return s
  if (isExternal(s) || s.startsWith('/api/') || s.startsWith('/')) return s
  return isMediaPath(s) ? mediaUrl(s) : s
}

// Build a gallery item, detecting video vs image from the raw path/URL extension.
function toItem(raw: string, alt?: string): LightboxImage {
  return { src: resolveSrc(raw), alt: alt || undefined, type: isVideoPath(raw) ? 'video' : 'image' }
}

// Parse a ```gallery block. Accepts JSON ({ "images": [...] } or a bare array of
// strings/objects) or, as a fallback, a newline/comma-separated list of paths.
function parse(code: string): { title?: string; images: LightboxImage[] } {
  const text = code.trim()
  let title: string | undefined
  let items: (string | RawItem)[] = []

  try {
    const data = JSON.parse(text)
    if (Array.isArray(data)) {
      items = data
    } else if (data && typeof data === 'object') {
      title = typeof data.title === 'string' ? data.title : undefined
      const arr = data.images ?? data.items ?? data.srcs ?? data.src
      if (Array.isArray(arr)) items = arr
      else if (typeof arr === 'string') items = [arr]
    }
  } catch {
    // Not JSON: treat each non-empty line (or comma-separated value) as a path.
    items = text
      .split(/[\n,]/)
      .map((s) => s.trim())
      .filter(Boolean)
  }

  const images: LightboxImage[] = []
  for (const it of items) {
    if (typeof it === 'string') {
      images.push(toItem(it))
    } else if (it && typeof it === 'object') {
      const raw = it.src || it.url || it.path
      if (raw) images.push(toItem(raw, it.alt || it.caption || it.label))
    }
  }
  return { title, images }
}

// Gallery renders a ```gallery fenced block as a responsive thumbnail grid.
// Clicking a thumbnail opens the shared Lightbox at that index with prev/next
// navigation (the external agent project-style image gallery).
export function Gallery({ code }: Props) {
  const { title, images } = useMemo(() => parse(code), [code])
  const [open, setOpen] = useState<number | null>(null)

  if (images.length === 0) {
    return (
      <div className="my-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-xs text-[var(--color-text-dim)]">
        gallery · görsel bulunamadı
      </div>
    )
  }

  return (
    <div className="my-2">
      {title && <div className="mb-1.5 text-xs font-medium text-[var(--color-text-dim)]">{title}</div>}
      <div className="grid grid-cols-[repeat(auto-fill,minmax(120px,1fr))] gap-2">
        {images.map((img, i) => (
          <button
            key={i}
            type="button"
            onClick={() => setOpen(i)}
            title={img.alt || `Görsel ${i + 1}`}
            className="group relative aspect-square overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)]"
          >
            {img.type === 'video' ? (
              <>
                <video
                  src={img.src}
                  muted
                  preload="metadata"
                  className="h-full w-full object-cover transition group-hover:scale-105"
                />
                <span className="absolute inset-0 flex items-center justify-center">
                  <span className="flex h-9 w-9 items-center justify-center rounded-full bg-black/55 text-white">
                    <Play size={18} className="ml-0.5" />
                  </span>
                </span>
              </>
            ) : (
              <img
                src={img.src}
                alt={img.alt || ''}
                loading="lazy"
                className="h-full w-full cursor-zoom-in object-cover transition group-hover:scale-105"
              />
            )}
            {img.alt && (
              <span className="absolute inset-x-0 bottom-0 truncate bg-black/55 px-1.5 py-0.5 text-[10px] text-white opacity-0 transition group-hover:opacity-100">
                {img.alt}
              </span>
            )}
          </button>
        ))}
      </div>
      {open !== null && (
        <Lightbox
          images={images}
          index={open}
          title={title}
          onClose={() => setOpen(null)}
        />
      )}
    </div>
  )
}

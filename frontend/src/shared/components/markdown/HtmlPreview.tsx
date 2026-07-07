import { useEffect, useMemo, useState } from 'react'
import { fileTextURL } from '@/shared/lib/attachments'

interface Props {
  code: string
}

interface RawItem {
  src?: string
  path?: string
  url?: string
  label?: string
}

interface PreviewItem {
  src: string
  label?: string
}

// parse reads an ```html-preview block: JSON with a single "src" or an "items"
// array (each {src,label}), plus an optional "title". A bare (non-JSON) body is
// treated as a single src path.
function parse(code: string): { title?: string; items: PreviewItem[] } {
  const text = code.trim()
  let title: string | undefined
  const items: PreviewItem[] = []
  try {
    const data = JSON.parse(text)
    if (typeof data === 'string') {
      pushItem(items, { src: data })
    } else if (Array.isArray(data)) {
      for (const it of data as RawItem[]) pushItem(items, it)
    } else if (data && typeof data === 'object') {
      title = typeof data.title === 'string' ? data.title : undefined
      if (Array.isArray(data.items)) {
        for (const it of data.items as RawItem[]) pushItem(items, it)
      } else {
        pushItem(items, data as RawItem)
      }
    }
  } catch {
    // Not JSON: treat the whole body as a single path.
    if (text) pushItem(items, { src: text })
  }
  return { title, items }
}

function pushItem(items: PreviewItem[], it: RawItem) {
  const src = (it.src || it.path || it.url || '').trim()
  if (src) items.push({ src, label: it.label })
}

// HtmlPreview renders an ```html-preview fenced block. It fetches the referenced
// file's TEXT (via /api/files?...&as=text, restricted server-side to the render
// root) and injects it into a sandboxed iframe. `sandbox="allow-scripts"` WITHOUT
// `allow-same-origin` gives the iframe an opaque origin: scripts run but cannot
// reach the parent DOM, cookies, or the workspace API — the same isolation the
// artifact HTML viewer uses. Multiple `items` render as switchable tabs.
export function HtmlPreview({ code }: Props) {
  const { title, items } = useMemo(() => parse(code), [code])
  const [active, setActive] = useState(0)

  if (items.length === 0) {
    return (
      <div className="my-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-3 text-xs text-[var(--color-text-dim)]">
        html-preview · kaynak bulunamadı
      </div>
    )
  }

  const idx = Math.min(active, items.length - 1)
  const current = items[idx]
  const showBar = Boolean(title) || items.length > 1

  return (
    <div className="my-2 overflow-hidden rounded-lg border border-[var(--color-border)]">
      {showBar && (
        <div className="flex items-center gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1">
          {title && (
            <span className="mr-1 text-xs font-medium text-[var(--color-text-dim)]">{title}</span>
          )}
          {items.length > 1 && (
            <div className="flex flex-wrap gap-1">
              {items.map((it, i) => (
                <button
                  key={i}
                  type="button"
                  onClick={() => setActive(i)}
                  className={
                    'rounded px-2 py-0.5 text-[11px] transition ' +
                    (i === idx
                      ? 'bg-[var(--color-accent)] text-white'
                      : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]')
                  }
                >
                  {it.label || `Sekme ${i + 1}`}
                </button>
              ))}
            </div>
          )}
        </div>
      )}
      <HtmlFrame src={current.src} />
    </div>
  )
}

// HtmlFrame fetches one file's text and renders it inside a sandboxed iframe.
function HtmlFrame({ src }: { src: string }) {
  const [html, setHtml] = useState<string | null>(null)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    let cancelled = false
    setHtml(null)
    setError(null)
    const url = fileTextURL(src)
    if (!url) {
      setError('geçersiz kaynak yolu')
      return
    }
    fetch(url)
      .then(async (res) => {
        if (!res.ok) throw new Error(`${res.status} ${res.statusText}`)
        return res.text()
      })
      .then((text) => {
        if (!cancelled) setHtml(text)
      })
      .catch((e) => {
        if (!cancelled) setError(String(e?.message || e))
      })
    return () => {
      cancelled = true
    }
  }, [src])

  if (error) {
    return (
      <div className="bg-[var(--color-bg)] p-3 text-xs text-[var(--color-danger)]">
        html-preview yüklenemedi: {error}
      </div>
    )
  }
  if (html === null) {
    return (
      <div className="bg-[var(--color-bg)] p-3 text-xs text-[var(--color-text-dim)]">yükleniyor…</div>
    )
  }
  return (
    <iframe
      sandbox="allow-scripts"
      srcDoc={html}
      title="HTML preview"
      className="h-[440px] w-full border-0 bg-white"
    />
  )
}

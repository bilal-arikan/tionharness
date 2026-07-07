import { useEffect, useId, useRef, useState } from 'react'
import { Lightbox } from '@/shared/components'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { Copy, Check } from 'lucide-react'

interface Props {
  code: string
}

// A monotonic counter keeps every mermaid render id unique even when several
// diagrams share the same React useId base (mermaid mutates the DOM by id).
let renderSeq = 0

// Resolve the mermaid base theme from the document's appearance. The app marks
// light mode with data-theme="light" on <html>; its absence means dark.
function currentBase(): 'dark' | 'default' {
  return document.documentElement.getAttribute('data-theme') === 'light' ? 'default' : 'dark'
}

// Pull a few palette custom properties so the diagram blends with the active
// theme/preset instead of mermaid's stock colors.
function themeVariables(): Record<string, string> {
  const cs = getComputedStyle(document.documentElement)
  const v = (name: string) => cs.getPropertyValue(name).trim()
  const accent = v('--color-accent') || '#6366f1'
  const text = v('--color-text') || '#e5e7eb'
  const surface = v('--color-surface') || '#1f2937'
  const surface2 = v('--color-surface-2') || '#111827'
  const border = v('--color-border') || '#374151'
  return {
    primaryColor: surface,
    primaryTextColor: text,
    primaryBorderColor: accent,
    lineColor: border,
    secondaryColor: surface2,
    tertiaryColor: surface2,
    background: 'transparent',
    fontSize: '13px',
  }
}

// MermaidDiagram renders a ```mermaid fenced block as an SVG. Mermaid is loaded
// lazily (dynamic import) so its ~heavy bundle never weighs on the initial app
// load — it only ships when a diagram actually appears. Rendering is debounced
// and error-tolerant: while a diagram streams in token-by-token the source is
// often syntactically incomplete, so failures fall back to the raw code instead
// of throwing. The diagram re-renders when the document theme changes.
export function MermaidDiagram({ code }: Props) {
  const baseId = useId().replace(/:/g, '')
  const [svg, setSvg] = useState('')
  const [error, setError] = useState(false)
  const [showSource, setShowSource] = useState(false)
  const [expanded, setExpanded] = useState(false)
  const [copied, setCopied] = useState(false)
  // Bumped by a MutationObserver on <html data-theme> to force a re-render.
  const [themeTick, setThemeTick] = useState(0)
  const aliveRef = useRef(true)

  useEffect(() => {
    aliveRef.current = true
    return () => {
      aliveRef.current = false
    }
  }, [])

  // Re-render diagrams when the appearance (light/dark or preset) changes.
  useEffect(() => {
    const obs = new MutationObserver(() => setThemeTick((t) => t + 1))
    obs.observe(document.documentElement, { attributes: true, attributeFilter: ['data-theme', 'style'] })
    return () => obs.disconnect()
  }, [])

  useEffect(() => {
    const trimmed = code.trim()
    if (!trimmed) {
      setSvg('')
      setError(false)
      return
    }
    // Debounce so streaming tokens don't trigger a render per character.
    const handle = setTimeout(async () => {
      const id = `mermaid-${baseId}-${renderSeq++}`
      try {
        const mermaid = (await import('mermaid')).default
        mermaid.initialize({
          startOnLoad: false,
          securityLevel: 'strict',
          theme: currentBase(),
          themeVariables: themeVariables(),
        })
        const { svg: out } = await mermaid.render(id, trimmed)
        if (!aliveRef.current) return
        setSvg(out)
        setError(false)
      } catch {
        if (!aliveRef.current) return
        setError(true)
      } finally {
        // On a parse error mermaid leaves an orphaned error graphic appended to
        // <body> (id or "d"+id). Remove it so the bomb/"Syntax error" SVG never
        // leaks outside our contained fallback — important while streaming.
        document.getElementById(id)?.remove()
        document.getElementById('d' + id)?.remove()
      }
    }, 120)
    return () => clearTimeout(handle)
  }, [code, baseId, themeTick])

  const copy = () => {
    copyToClipboard(code).then((ok) => {
      if (!ok) return
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    })
  }

  // Before the first successful render (or on a transient parse error mid-stream)
  // show the source so the user always sees the content.
  if (!svg) {
    return (
      <div className="my-2 overflow-hidden rounded-lg border border-[var(--color-border)]">
        <div className="border-b border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 text-[11px] text-[var(--color-text-dim)]">
          mermaid {error ? '· (geçersiz sözdizimi)' : '· işleniyor…'}
        </div>
        <pre className="overflow-x-auto bg-[var(--color-bg)] p-3 text-xs leading-relaxed">
          <code>{code}</code>
        </pre>
      </div>
    )
  }

  return (
    <>
      <div className="group relative my-2 overflow-hidden rounded-lg border border-[var(--color-border)]">
        <div className="flex items-center justify-between border-b border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1 text-[11px] text-[var(--color-text-dim)]">
          <span>mermaid</span>
          <div className="flex items-center gap-1 opacity-0 transition group-hover:opacity-100">
            <button
              onClick={() => setShowSource((s) => !s)}
              title={showSource ? 'Diyagramı göster' : 'Kaynağı göster'}
              className="rounded px-1.5 py-0.5 hover:bg-[var(--color-surface-2)]"
            >
              {showSource ? 'Diagram' : 'Source'}
            </button>
            {!showSource && (
              <button
                onClick={() => setExpanded(true)}
                title="Tam ekran"
                className="rounded px-1.5 py-0.5 hover:bg-[var(--color-surface-2)]"
              >
                Expand
              </button>
            )}
            <button
              onClick={copy}
              title={copied ? 'Kopyalandı' : 'Kodu kopyala'}
              aria-label={copied ? 'Kopyalandı' : 'Kodu kopyala'}
              className="rounded px-1.5 py-0.5 hover:bg-[var(--color-surface-2)]"
            >
              {copied ? <Check size={14} className="text-[var(--color-success)]" /> : <Copy size={14} />}
            </button>
          </div>
        </div>
        {showSource ? (
          <pre className="overflow-x-auto bg-[var(--color-bg)] p-3 text-xs leading-relaxed">
            <code>{code}</code>
          </pre>
        ) : (
          <div
            className="sg-mermaid flex justify-center overflow-x-auto bg-[var(--color-bg)] p-3 [&_svg]:max-w-full [&_svg]:h-auto"
            onClick={() => setExpanded(true)}
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        )}
      </div>

      {expanded && (
        <Lightbox title="mermaid" onClose={() => setExpanded(false)}>
          <div
            className="sg-mermaid rounded-lg bg-[var(--color-bg)] p-6 [&_svg]:h-auto [&_svg]:max-w-[88vw]"
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        </Lightbox>
      )}
    </>
  )
}

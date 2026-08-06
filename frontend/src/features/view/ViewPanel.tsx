import { useCallback, useEffect, useState } from 'react'
import { Loader2, Copy, RefreshCw, X, ChevronLeft } from 'lucide-react'
import { api } from '@/api'
import { toast } from '@/shared/components'
import { VIEW_LENS_LABEL, refToString } from '@/types'
import type { ViewLens, ViewLevel, ViewRef, ViewResult } from '@/types'

const LEVELS: ViewLevel[] = ['tiny', 'card', 'full']
const LENSES: ViewLens[] = ['health', 'stale', 'recent', 'errors']

interface Props {
  // The entity to project. Changing it resets the drill-down trail.
  target: ViewRef
  onClose: () => void
  // Hand the projection to an agent (paste `view://…` into a composer). Absent →
  // the button is hidden.
  onSend?: (text: string, ref: ViewRef) => void
}

// ViewPanel is the "◱ Özet" drawer: the same compact projection an agent gets,
// shown verbatim.
//
// Two deliberate choices (see _Docs/66-VIEW-KATMANI.md):
//   - The raw DSL is displayed in monospace, NOT re-rendered as pretty cards. If
//     the projection is wrong, misleading or stale, the user sees exactly what
//     the model saw.
//   - The token estimate and the asOf stamp are always on screen, so an expensive
//     or stale view is obvious rather than something to discover later.
export function ViewPanel({ target, onClose, onSend }: Props) {
  const [trail, setTrail] = useState<ViewRef[]>([target])
  const [level, setLevel] = useState<ViewLevel>('card')
  const [lens, setLens] = useState<ViewLens>('health')
  const [result, setResult] = useState<ViewResult | null>(null)
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(false)

  const ref = trail[trail.length - 1]

  // A new target is a new subject: drop the drill-down trail rather than leaving
  // the user inside a breadcrumb belonging to the previous entity.
  useEffect(() => {
    setTrail([target])
  }, [target])

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setResult(await api.getView(ref, level, lens))
      setError(null)
    } catch (e) {
      // Surface the failure instead of showing a stale projection as if current.
      setResult(null)
      setError(e instanceof Error ? e.message : String(e))
    } finally {
      setLoading(false)
    }
  }, [ref, level, lens])

  useEffect(() => {
    void load()
  }, [load])

  const copy = async () => {
    if (!result) return
    await navigator.clipboard.writeText(result.text)
    toast.info('Panoya kopyalandı')
  }

  return (
    <div className="flex h-full w-full max-w-[560px] flex-col border-l border-[var(--color-border)] bg-[var(--color-surface)]">
      <header className="flex items-center gap-2 border-b border-[var(--color-border)] px-3 py-2">
        {trail.length > 1 && (
          <button
            type="button"
            onClick={() => setTrail((t) => t.slice(0, -1))}
            title="Geri"
            className="text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          >
            <ChevronLeft size={15} />
          </button>
        )}
        <span className="truncate text-sm font-medium">◱ Özet</span>
        <span className="truncate font-mono text-xs text-[var(--color-text-dim)]">
          {refToString(ref)}
        </span>
        <button
          type="button"
          onClick={onClose}
          title="Kapat"
          className="ml-auto text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
        >
          <X size={15} />
        </button>
      </header>

      {/* Controls: budget tier + lens. Both are part of the contract the agent
          uses, so they are named identically here and in the get_view tool. */}
      <div className="flex flex-wrap items-center gap-2 border-b border-[var(--color-border)] px-3 py-2 text-xs">
        <div className="flex overflow-hidden rounded-md border border-[var(--color-border)]">
          {LEVELS.map((l) => (
            <button
              key={l}
              type="button"
              onClick={() => setLevel(l)}
              className={`px-2 py-1 transition ${
                level === l
                  ? 'bg-[var(--color-accent)] text-white'
                  : 'text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
              }`}
            >
              {l}
            </button>
          ))}
        </div>
        <select
          value={lens}
          onChange={(e) => setLens(e.target.value as ViewLens)}
          className="rounded-md border border-[var(--color-border)] bg-transparent px-2 py-1 text-[var(--color-text)]"
        >
          {LENSES.map((l) => (
            <option key={l} value={l}>
              {VIEW_LENS_LABEL[l]}
            </option>
          ))}
        </select>
        <button
          type="button"
          onClick={() => void load()}
          title="Yenile"
          className="text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
        >
          {loading ? <Loader2 size={14} className="animate-spin" /> : <RefreshCw size={14} />}
        </button>
        {result && (
          <span className="ml-auto flex items-center gap-2 text-[var(--color-text-dim)]">
            <span title={`Kaynak sürüm: ${result.source}`}>
              asOf {new Date(result.asOf).toLocaleTimeString()}
            </span>
            <span title="Yaklaşık token maliyeti (karakter/4)">~{result.tokens} tok</span>
          </span>
        )}
      </div>

      <div className="min-h-0 flex-1 overflow-auto p-3">
        {error ? (
          <p className="text-xs text-[var(--color-danger)]">{error}</p>
        ) : result ? (
          <pre className="whitespace-pre-wrap break-words font-mono text-xs leading-relaxed text-[var(--color-text)]">
            {result.text}
          </pre>
        ) : (
          <p className="text-xs text-[var(--color-text-dim)]">Yükleniyor…</p>
        )}
      </div>

      {result && (
        <footer className="border-t border-[var(--color-border)] px-3 py-2">
          {/* Elision is reported unconditionally: a summary that quietly drops
              items misleads whoever reads it, model or human. */}
          {result.elided > 0 && (
            <p className="mb-2 text-xs text-[var(--color-text-dim)]">
              {result.elided} {result.elidedUnit || 'öğe'} gizlendi — daha fazlası için seviyeyi
              yükselt veya bir bağlantıyı aç.
            </p>
          )}
          {(result.handles ?? []).length > 0 && (
            <div className="mb-2 flex flex-wrap gap-1.5">
              {(result.handles ?? []).map((h) => (
                <button
                  key={refToString(h.ref) + h.label}
                  type="button"
                  onClick={() => {
                    setTrail((t) => [...t, h.ref])
                    if (h.level) setLevel(h.level)
                  }}
                  className="rounded-full border border-[var(--color-border)] px-2 py-0.5 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                >
                  ↳ {h.label}
                </button>
              ))}
            </div>
          )}
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => void copy()}
              className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              <Copy size={13} />
              Kopyala
            </button>
            {onSend && (
              <button
                type="button"
                onClick={() => onSend(result.text, ref)}
                className="rounded-md border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
              >
                ⤓ Ajana gönder
              </button>
            )}
          </div>
        </footer>
      )}
    </div>
  )
}

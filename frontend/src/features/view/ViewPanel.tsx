import { useCallback, useEffect, useState } from 'react'
import { Loader2, Copy, RefreshCw, X, ChevronLeft } from 'lucide-react'
import { api } from '@/api'
import { toast } from '@/shared/components'
import { VIEW_LENS_LABEL, refToString } from '@/types'
import type { ViewLens, ViewLevel, ViewRef, ViewResult } from '@/types'
import { formatTime } from '@/shared/lib/intl'

const LEVELS: ViewLevel[] = ['tiny', 'card', 'full']
const LENSES: ViewLens[] = ['health', 'stale', 'recent', 'errors']

interface Props {
  // The entity to project. Changing it resets the drill-down trail.
  target: ViewRef
  // Close affordance. Optional: when embedded inline (e.g. inside the session
  // detail panel) there is no drawer to close, so the X button is hidden.
  onClose?: () => void
  // Hand the projection to an agent (paste `view://…` into a composer). Absent →
  // the button is hidden.
  onSend?: (text: string, ref: ViewRef) => void
  // Embedded mode: render as a self-contained block inside a host panel instead
  // of a full-height right-anchored drawer (drops the side border / fixed height
  // / bg so it flows in the host's scroll).
  embedded?: boolean
  // lens hands lens ownership to the host. The Explorer screen has its own lens
  // selector that filters the MAP; leaving the panel a second, independent one
  // let the two disagree — the map showing only failures while the panel beside
  // it rendered the healthy summary. When set, the panel follows it and hides
  // its own selector; when absent the panel owns its lens as before.
  lens?: ViewLens
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
export function ViewPanel({ target, onClose, onSend, embedded, lens: hostLens }: Props) {
  const [trail, setTrail] = useState<ViewRef[]>([target])
  const [level, setLevel] = useState<ViewLevel>('card')
  const [ownLens, setOwnLens] = useState<ViewLens>('health')
  const lens = hostLens ?? ownLens
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
    // Copy all three budget tiers so the user sees exactly what the agent would
    // get at each level — no extra UI toggle needed.
    try {
      const [tiny, card, full] = await Promise.all([
        api.getView(ref, 'tiny', lens),
        api.getView(ref, 'card', lens),
        api.getView(ref, 'full', lens),
      ])
      // Tiers that render identically are merged into one block instead of being
      // pasted three times. Small entities (a skill, a schedule, a category) have
      // nothing extra to say above `card`, and three identical blocks under three
      // different headings read as a bug — the reader assumes the levels were
      // ignored rather than that this entity genuinely fits in one tier.
      const tiers: Array<{ name: string; r: ViewResult }> = [
        { name: 'TINY', r: tiny },
        { name: 'CARD', r: card },
        { name: 'FULL', r: full },
      ]
      const blocks: Array<{ names: string[]; r: ViewResult }> = []
      for (const t of tiers) {
        const last = blocks[blocks.length - 1]
        if (last && last.r.text === t.r.text) last.names.push(t.name)
        else blocks.push({ names: [t.name], r: t.r })
      }
      const combined = blocks
        .map(
          (b) =>
            `[${b.names.join(' = ')} — ~${b.r.tokens} tok · asOf ${formatTime(new Date(b.r.asOf), {
              timeStyle: 'medium',
            })}]\n${b.r.text}`,
        )
        .join('\n\n')
      await navigator.clipboard.writeText(combined)
      toast.info(
        blocks.length === 1
          ? 'Panoya kopyalandı (üç seviye de aynı içeriği veriyor)'
          : `${blocks.length} farklı seviye panoya kopyalandı`,
      )
    } catch {
      // Fallback: copy just the current level if the others fail.
      await navigator.clipboard.writeText(result.text)
      toast.info('Panoya kopyalandı')
    }
  }

  return (
    <div
      className={
        embedded
          ? 'flex w-full flex-col'
          : 'flex h-full w-full max-w-[560px] flex-col border-l border-[var(--color-border)] bg-[var(--color-surface)]'
      }
    >
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
        {onClose && (
          <button
            type="button"
            onClick={onClose}
            title="Kapat"
            className="ml-auto text-[var(--color-text-dim)] transition hover:text-[var(--color-danger)]"
          >
            <X size={15} />
          </button>
        )}
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
        {hostLens ? (
          // The host owns the lens: show which one is in force, do not offer a
          // second control that could disagree with it.
          <span className="text-[var(--color-text-dim)]">mercek: {VIEW_LENS_LABEL[lens]}</span>
        ) : (
          <select
            value={lens}
            onChange={(e) => setOwnLens(e.target.value as ViewLens)}
            className="rounded-md border border-[var(--color-border)] bg-transparent px-2 py-1 text-[var(--color-text)]"
          >
            {LENSES.map((l) => (
              <option key={l} value={l}>
                {VIEW_LENS_LABEL[l]}
              </option>
            ))}
          </select>
        )}
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
              asOf {formatTime(new Date(result.asOf), { timeStyle: 'medium' })}
            </span>
            <span title="Yaklaşık token maliyeti (karakter/4)">~{result.tokens} tok</span>
          </span>
        )}
      </div>

      <div
        className={
          embedded ? 'max-h-[420px] overflow-auto p-3' : 'min-h-0 flex-1 overflow-auto p-3'
        }
      >
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

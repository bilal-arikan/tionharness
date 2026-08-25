import { useMemo, useRef, useState } from 'react'
import { parseDiff, foldableRanges, diffRows } from '@/shared/lib/diff'
import { useVirtualRows } from '@/shared/hooks/useVirtualRows'
import { count } from '@/shared/lib/format'

interface Props {
  text: string
  /**
   * 'inline' (default) — the small in-chat diff card: wraps long lines and folds
   * nothing, because wrapped rows have no fixed height and so cannot be
   * virtualized. A long patch is therefore capped behind an explicit expander
   * (see INLINE_CAP) — every line of a big diff would otherwise be a real DOM
   * node living in the transcript for as long as the session is open.
   *
   * 'panel' — the bulk changes popup, sized for patches that can run to tens of
   * thousands of lines: long unchanged runs fold, lines do not wrap (fixed row
   * height is what makes virtualization possible), rows past a threshold are
   * windowed, and a truly huge patch is capped behind an explicit expander.
   */
  variant?: 'inline' | 'panel'
}

const ROW: Record<string, string> = {
  add: 'bg-[color-mix(in_srgb,var(--color-success)_14%,transparent)] text-[var(--color-success)]',
  del: 'bg-[color-mix(in_srgb,var(--color-danger)_14%,transparent)] text-[var(--color-danger)]',
  hunk: 'bg-[var(--color-accent)]/10 text-[var(--color-accent)]',
  meta: 'text-[var(--color-text-dim)]',
  ctx: 'text-[var(--color-text)]',
}

const GUTTER: Record<string, string> = {
  add: '+',
  del: '-',
  hunk: ' ',
  meta: ' ',
  ctx: ' ',
}

// Row height in px. MUST match the rendered row exactly (see useVirtualRows).
const ROW_H = 18
// Above this many rows the panel switches to windowed rendering. Below it the
// plain list is cheaper than the scroll bookkeeping.
const VIRTUAL_MIN = 400
// A patch this long is not read line by line — it is skimmed or opened in an
// editor. Render a head slice and make the full mount an explicit choice, with
// the real size stated. Never a silent truncation.
const HARD_CAP = 20_000
const CAP_PREVIEW = 2_000

// The inline card's own cap. Far lower than the panel's because inline rows wrap
// (no fixed height → no virtualization), so every rendered line is a permanent
// DOM node in the transcript. Diffs under the cap render exactly as before.
const INLINE_CAP = 500
const INLINE_PREVIEW = 300

// DiffView renders a unified diff with per-line +/- coloring and a small
// add/remove stat header, matching the file-change cards in External Agent chat.
export function DiffView({ text, variant = 'inline' }: Props) {
  // parseDiff walks the whole patch; on a multi-megabyte one that is the single
  // most expensive thing here, so it is keyed to the text and nothing else.
  const { lines, stats } = useMemo(() => parseDiff(text), [text])

  if (variant === 'inline') {
    return <DiffInline lines={lines} added={stats.added} removed={stats.removed} />
  }
  return <DiffPanel lines={lines} added={stats.added} removed={stats.removed} />
}

// DiffInline is the in-chat card. Short patches render whole; a long one shows a
// head slice plus an expander that states the real size — never a silent
// truncation, same rule as the panel.
function DiffInline({
  lines,
  added,
  removed,
}: {
  lines: ReturnType<typeof parseDiff>['lines']
  added: number
  removed: number
}) {
  const [uncapped, setUncapped] = useState(false)
  const capped = !uncapped && lines.length > INLINE_CAP
  const shown = capped ? INLINE_PREVIEW : lines.length

  return (
    <Frame
      added={added}
      removed={removed}
      extra={
        capped ? (
          <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)]">
            {count(lines.length)} satır
          </span>
        ) : undefined
      }
    >
      <pre className="overflow-x-auto bg-[var(--color-bg)] py-1 text-xs leading-relaxed">
        <code className="block font-mono">
          {lines.slice(0, shown).map((l, i) => (
            <div key={i} className={`flex px-3 ${ROW[l.kind]}`}>
              <span className="mr-2 select-none opacity-50">{GUTTER[l.kind]}</span>
              <span className="whitespace-pre-wrap break-all">
                {l.kind === 'add' || l.kind === 'del' ? l.text.slice(1) : l.text}
              </span>
            </div>
          ))}
        </code>
      </pre>
      {capped && (
        <button
          onClick={() => setUncapped(true)}
          className="shrink-0 border-t border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-left text-[11px] text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
        >
          İlk {count(INLINE_PREVIEW)} satır gösteriliyor ·{' '}
          <span className="font-medium">
            kalan {count(lines.length - INLINE_PREVIEW)} satırı da yükle
          </span>
        </button>
      )}
    </Frame>
  )
}

function Frame({
  added,
  removed,
  extra,
  children,
}: {
  added: number
  removed: number
  extra?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <div className="flex min-h-0 flex-col overflow-hidden rounded-lg border border-[var(--color-border)]">
      <div className="flex shrink-0 items-center gap-3 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-1.5 text-xs">
        <span className="text-[var(--color-success)]">+{added}</span>
        <span className="text-[var(--color-danger)]">−{removed}</span>
        {extra}
      </div>
      {children}
    </div>
  )
}

// DiffPanel is the large-patch renderer: fold → cap → virtualize, in that order.
// Folding runs first because it is the only step that shrinks the actual work;
// the cap and the window only limit what is painted.
function DiffPanel({
  lines,
  added,
  removed,
}: {
  lines: ReturnType<typeof parseDiff>['lines']
  added: number
  removed: number
}) {
  const [expanded, setExpanded] = useState<Set<number>>(() => new Set())
  const [uncapped, setUncapped] = useState(false)

  const capped = !uncapped && lines.length > HARD_CAP
  const shown = capped ? CAP_PREVIEW : lines.length

  const ranges = useMemo(() => foldableRanges(lines.slice(0, shown)), [lines, shown])
  const rows = useMemo(() => diffRows(shown, ranges, expanded), [shown, ranges, expanded])

  const virtual = rows.length > VIRTUAL_MIN
  const scrollRef = useRef<HTMLDivElement>(null)
  const win = useVirtualRows(scrollRef, rows.length, ROW_H, 24)
  const from = virtual ? win.start : 0
  const to = virtual ? win.end : rows.length

  const toggleFold = (id: number) =>
    setExpanded((prev) => {
      const next = new Set(prev)
      if (!next.delete(id)) next.add(id)
      return next
    })

  return (
    <Frame
      added={added}
      removed={removed}
      extra={
        <span className="ml-auto font-mono text-[10px] text-[var(--color-text-dim)]">
          {count(lines.length)} satır
        </span>
      }
    >
      {/* Horizontal scroll instead of wrapping: a wrapped line is taller than
          ROW_H, which would desync the virtual spacers from the real content. */}
      <div ref={scrollRef} className="min-h-0 flex-1 overflow-auto bg-[var(--color-bg)]">
        <div
          style={{
            paddingTop: virtual ? win.padTop : 0,
            paddingBottom: virtual ? win.padBottom : 0,
          }}
        >
          <code className="block w-max min-w-full font-mono text-xs">
            {rows.slice(from, to).map((row, k) => {
              const i = from + k
              if (row.kind === 'fold') {
                return (
                  <button
                    key={`f${row.id}`}
                    onClick={() => toggleFold(row.id)}
                    style={{ height: ROW_H }}
                    className="flex w-full items-center px-3 text-left text-[11px] leading-none text-[var(--color-text-dim)] transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-accent)]"
                  >
                    ⋯ {count(row.count)} değişmeyen satır — göster
                  </button>
                )
              }
              const l = lines[row.index]
              return (
                <div
                  key={i}
                  style={{ height: ROW_H }}
                  className={`flex items-center px-3 leading-none ${ROW[l.kind]}`}
                >
                  <span className="mr-2 w-2 shrink-0 select-none opacity-50">{GUTTER[l.kind]}</span>
                  <span className="whitespace-pre">
                    {l.kind === 'add' || l.kind === 'del' ? l.text.slice(1) : l.text}
                  </span>
                </div>
              )
            })}
          </code>
        </div>
      </div>
      {capped && (
        <button
          onClick={() => setUncapped(true)}
          className="shrink-0 border-t border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-left text-[11px] text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
        >
          İlk {count(CAP_PREVIEW)} satır gösteriliyor ·{' '}
          <span className="font-medium">
            kalan {count(lines.length - CAP_PREVIEW)} satırı da yükle
          </span>
        </button>
      )}
    </Frame>
  )
}

// A staged intervention waiting above the composer while a turn streams:
// a queued message (sent when the turn ends) or a steer (live guidance sent
// after a short cancellable delay). Either can be removed before it is applied.
export interface PendingItem {
  id: string
  text: string
  kind: 'queue' | 'steer'
  // The session this intervention belongs to. The tray is filtered to the
  // active session, and queue flush / steer dispatch target this session's
  // turn — so staged items for a background turn never apply to another.
  sid: string
}

interface Props {
  items: PendingItem[]
  onRemove: (id: string) => void
}

// PendingTray lists staged queue/steer items above the composer, each removable
// (✕) before it is processed.
export function PendingTray({ items, onRemove }: Props) {
  if (items.length === 0) return null
  return (
    <div className="flex flex-col gap-1.5 border-t border-[var(--color-border)] bg-[var(--color-surface)] px-6 pt-3">
      <span className="text-[10px] font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
        Bekleyenler — işleme alınmadan silebilirsin
      </span>
      {items.map((it) => (
        <div
          key={it.id}
          className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-2.5 py-1.5 text-sm"
        >
          <span
            className={`shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold ${
              it.kind === 'steer'
                ? 'bg-amber-500/20 text-amber-500'
                : 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
            }`}
            title={it.kind === 'steer' ? 'Canlı yönlendirme (birazdan gönderilecek)' : 'Sıradaki mesaj (tur bitince gönderilecek)'}
          >
            {it.kind === 'steer' ? '⏱ Yönlendir' : '⏳ Sırada'}
          </span>
          <span className="min-w-0 flex-1 truncate text-[var(--color-text)]">{it.text}</span>
          <button
            onClick={() => onRemove(it.id)}
            title="Sil (işleme alınmadan)"
            className="shrink-0 rounded p-0.5 text-[var(--color-text-dim)] transition hover:text-red-400"
          >
            ✕
          </button>
        </div>
      ))}
    </div>
  )
}

import { useState } from 'react'
import { ChevronRight, ChevronDown, LayoutGrid } from 'lucide-react'
import type { InsightFinding } from '@/types'
import { ChannelBadge, SeverityBadge, RegressedBadge, StatusBadge } from './insightBadges'

interface Props {
  f: InsightFinding
  selected: boolean
  onSelect: (id: string) => void
  onStatus: (id: string, status: string) => void
  onAddCard: (f: InsightFinding) => void
  onOpenSession: (sid: string) => void
  // When this card represents a cluster, the additional similar findings.
  similar?: InsightFinding[]
}

function accentColor(f: InsightFinding): string {
  if (f.regressed) return 'var(--color-danger)'
  if (f.severity === 'high') return 'var(--color-danger)'
  if (f.severity === 'med' || f.severity === 'medium') return 'var(--color-warning, #d97706)'
  return 'var(--color-border)'
}

export function FindingCard({ f, selected, onSelect, onStatus, onAddCard, onOpenSession, similar }: Props) {
  const [open, setOpen] = useState(false)
  const status = f.status || 'new'
  const extra = similar?.length ?? 0
  return (
    <div
      className={`rounded-md border border-[var(--color-border)] ${status === 'dismissed' ? 'opacity-60' : ''}`}
      style={{ borderLeft: `3px solid ${accentColor(f)}` }}
    >
      {/* Header row */}
      <div className="flex items-start gap-2 p-3">
        <input
          type="checkbox"
          checked={selected}
          onChange={() => onSelect(f.id)}
          className="mt-1"
          onClick={(e) => e.stopPropagation()}
        />
        <button onClick={() => setOpen((v) => !v)} className="mt-0.5 text-[var(--color-text-muted)]">
          {open ? <ChevronDown className="h-4 w-4" /> : <ChevronRight className="h-4 w-4" />}
        </button>
        <div className="min-w-0 flex-1 cursor-pointer" onClick={() => setOpen((v) => !v)}>
          <div className="flex flex-wrap items-center gap-2">
            <ChannelBadge channel={f.channel} />
            {f.regressed && <RegressedBadge />}
            <span className="font-medium">{f.title}</span>
            {f.severity && <SeverityBadge severity={f.severity} />}
            {f.occurrences > 1 && <span className="text-xs text-[var(--color-text-muted)]">×{f.occurrences}</span>}
            {status !== 'new' && <StatusBadge status={status} />}
            {extra > 0 && (
              <span className="rounded bg-[var(--color-accent)]/15 px-1.5 py-0.5 text-xs text-[var(--color-accent)]">
                +{extra} benzer
              </span>
            )}
          </div>
          {!open && f.rootCause && (
            <div className="mt-1 truncate text-sm text-[var(--color-text-muted)]">{f.rootCause}</div>
          )}
        </div>
      </div>

      {/* Expanded body */}
      {open && (
        <div className="border-t border-[var(--color-border)] px-3 py-2 text-sm">
          {f.rootCause && <p className="text-[var(--color-text-muted)]">{f.rootCause}</p>}
          {f.proposedFix && (
            <p className="mt-1">
              <span className="font-medium">Öneri:</span> {f.proposedFix}
            </p>
          )}
          {f.filePointer && (
            <code className="mt-1 block text-xs text-[var(--color-text-muted)]">{f.filePointer}</code>
          )}
          {f.evidenceSessionIds && f.evidenceSessionIds.length > 0 && (
            <div className="mt-2 flex flex-wrap items-center gap-1">
              <span className="text-xs text-[var(--color-text-muted)]">Kanıt:</span>
              {f.evidenceSessionIds.map((sid) => (
                <button
                  key={sid}
                  onClick={() => onOpenSession(sid)}
                  className="rounded border border-[var(--color-border)] px-1.5 py-0.5 text-xs hover:bg-[var(--color-surface-2)]"
                  title="Oturumu aç"
                >
                  {sid}
                </button>
              ))}
            </div>
          )}

          {/* Cluster members */}
          {extra > 0 && (
            <div className="mt-2 space-y-1 rounded bg-[var(--color-surface-2)] p-2">
              <div className="text-xs font-medium text-[var(--color-text-muted)]">Benzer bulgular:</div>
              {similar!.map((m) => (
                <div key={m.id} className="text-xs">
                  • {m.title} {m.severity && <span className="text-[var(--color-text-muted)]">({m.severity})</span>}
                </div>
              ))}
            </div>
          )}

          {/* Actions */}
          <div className="mt-2 flex flex-wrap gap-2">
            <ActBtn label="Kabul" onClick={() => onStatus(f.id, 'accepted')} />
            <ActBtn label="Uygulandı" onClick={() => onStatus(f.id, 'applied')} />
            <ActBtn label="Doğrulandı" onClick={() => onStatus(f.id, 'verified')} />
            <ActBtn label="Yoksay" muted onClick={() => onStatus(f.id, 'dismissed')} />
            <button
              onClick={() => onAddCard(f)}
              className="flex items-center gap-1 rounded bg-[var(--color-accent)] px-2 py-0.5 text-xs text-white"
            >
              <LayoutGrid className="h-3.5 w-3.5" /> Karta ekle
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

function ActBtn({ label, onClick, muted }: { label: string; onClick: () => void; muted?: boolean }) {
  return (
    <button
      onClick={onClick}
      className={`rounded px-2 py-0.5 text-xs hover:bg-[var(--color-surface-2)] ${
        muted ? 'text-[var(--color-text-muted)]' : ''
      }`}
    >
      {label}
    </button>
  )
}

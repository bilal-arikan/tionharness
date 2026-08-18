import { useMemo, useState } from 'react'
import { LayoutGrid, Trash2 } from 'lucide-react'
import { api } from '@/api'
import type { InsightFinding, InsightLens } from '@/types'
import { SelectionBar, SelectionBarButton } from '@/shared/components'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { SummaryHeader } from './SummaryHeader'
import { FilterBar } from './FilterBar'
import { FindingModal } from './FindingModal'
import { ChannelBadge, SeverityBadge } from './insightBadges'
import { applyFilter, priorityScore, summarize, type FindingFilter } from './insightHelpers'

interface Props {
  findings: InsightFinding[]
  lenses: InsightLens[]
  reload: () => void
  onOpenSession: (sid: string) => void
  onError: (msg: string) => void
  onNote: (msg: string) => void
}

// Fixed lifecycle columns (a finding with an empty/triaged status buckets into
// "Yeni"). Moving a card between columns = changing its status.
const COLUMNS: { key: string; label: string }[] = [
  { key: 'new', label: 'Yeni' },
  { key: 'accepted', label: 'Kabul' },
  { key: 'applied', label: 'Uygulandı' },
  { key: 'verified', label: 'Doğrulandı' },
  { key: 'dismissed', label: 'Yoksayıldı' },
]
const bucketOf = (status: string) => {
  const s = status || 'new'
  return COLUMNS.some((c) => c.key === s) ? s : 'new'
}

const SEV2PRI: Record<string, string> = {
  high: 'high',
  med: 'medium',
  medium: 'medium',
  low: 'low',
}

function cardBody(f: InsightFinding): string {
  const parts: string[] = []
  if (f.rootCause) parts.push(`**Root cause:** ${f.rootCause}`)
  if (f.proposedFix) parts.push(`**Proposed fix:** ${f.proposedFix}`)
  const meta: string[] = []
  if (f.filePointer) meta.push(`file: \`${f.filePointer}\``)
  if (f.lensId) meta.push(`lens: ${f.lensId}`)
  if (f.evidenceSessionIds?.length) meta.push(`evidence: ${f.evidenceSessionIds.join(', ')}`)
  if (meta.length) parts.push(`\n_${meta.join(' · ')}_`)
  parts.push(`\n<!-- insight-sig:${f.sig} -->`)
  return parts.join('\n\n')
}

// FindingsTab is the board-style triage view: findings laid out in fixed
// lifecycle columns, drag or bulk-move to change status, click a card for the
// detail popup, multi-select (Ctrl/Shift) for bulk status / board-card / delete.
export function FindingsTab({ findings, lenses, reload, onOpenSession, onError, onNote }: Props) {
  const [filter, setFilter] = useState<FindingFilter>({})
  const [modalId, setModalId] = useState<string | null>(null)
  const [dragId, setDragId] = useState<string | null>(null)
  const sel = useMultiSelect()

  const filtered = useMemo(() => applyFilter(findings, filter), [findings, filter])
  const summary = useMemo(() => summarize(findings), [findings])

  // Cards per column, priority-sorted; plus the flat ordered id list for Shift-range.
  const byColumn = useMemo(() => {
    const m: Record<string, InsightFinding[]> = {}
    for (const c of COLUMNS) m[c.key] = []
    for (const f of filtered) m[bucketOf(f.status)].push(f)
    for (const k of Object.keys(m)) {
      m[k].sort(
        (a, b) => priorityScore(b) - priorityScore(a) || (b.lastSeen ?? 0) - (a.lastSeen ?? 0),
      )
    }
    return m
  }, [filtered])
  const orderedIds = useMemo(
    () => COLUMNS.flatMap((c) => byColumn[c.key].map((f) => f.id)),
    [byColumn],
  )

  const setStatus = async (id: string, status: string) => {
    try {
      await api.setInsightFindingStatus(id, status)
      reload()
    } catch (e) {
      onError((e as Error).message)
    }
  }
  const remove = async (id: string) => {
    try {
      await api.deleteInsightFinding(id)
      reload()
    } catch (e) {
      onError((e as Error).message)
    }
  }
  const addCard = async (f: InsightFinding) => {
    try {
      await api.createTask({
        title: f.title.slice(0, 120),
        description: cardBody(f),
        boardState: 'todo',
        priority: (SEV2PRI[f.severity ?? ''] ?? undefined) as never,
        tags: ['insight', f.channel],
      })
      onNote(`Karta eklendi: ${f.title.slice(0, 60)}`)
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const bulkStatus = async (status: string) => {
    if (!status) return
    const ids = [...sel.selected]
    sel.clear()
    await Promise.all(ids.map((id) => api.setInsightFindingStatus(id, status).catch(() => {})))
    reload()
  }
  const bulkDelete = async () => {
    const ids = [...sel.selected]
    if (ids.length === 0 || !confirm(`${ids.length} bulgu silinsin mi?`)) return
    sel.clear()
    await Promise.all(ids.map((id) => api.deleteInsightFinding(id).catch(() => {})))
    reload()
  }
  const bulkCard = async () => {
    const byId = new Map(findings.map((f) => [f.id, f]))
    const ids = [...sel.selected]
    sel.clear()
    for (const id of ids) {
      const f = byId.get(id)
      if (f) await addCard(f)
    }
  }

  const modalFinding = modalId ? (findings.find((f) => f.id === modalId) ?? null) : null

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <SummaryHeader summary={summary} onPick={(patch) => setFilter({ ...patch })} />
      <div className="my-2">
        <FilterBar filter={filter} setFilter={setFilter} lenses={lenses} />
      </div>

      {/* Kanban */}
      <div className="flex flex-1 gap-3 overflow-x-auto pb-2">
        {COLUMNS.map((col) => {
          const cards = byColumn[col.key]
          return (
            <div
              key={col.key}
              onDragOver={(e) => e.preventDefault()}
              onDrop={() => {
                if (dragId) void setStatus(dragId, col.key)
                setDragId(null)
              }}
              className="flex w-64 flex-shrink-0 flex-col rounded-lg bg-[var(--color-surface)]"
            >
              <div className="flex items-center justify-between rounded-t-lg px-3 py-2 text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
                <span>{col.label}</span>
                <span className="rounded bg-[var(--color-surface-2)] px-1.5">{cards.length}</span>
              </div>
              <div className="flex-1 space-y-2 overflow-y-auto px-2 pb-2 pt-1">
                {cards.map((f) => (
                  <div
                    key={f.id}
                    draggable
                    onDragStart={() => setDragId(f.id)}
                    onClick={(e) => {
                      if (sel.handleClick(e, f.id, orderedIds)) return
                      setModalId(f.id)
                    }}
                    className={`cursor-pointer rounded-lg border p-2 text-sm shadow-[var(--shadow-sm)] transition hover:shadow-[var(--shadow-md)] ${
                      sel.isSelected(f.id)
                        ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] ring-1 ring-[var(--color-accent)]'
                        : f.regressed
                          ? 'border-[var(--color-danger)]/50 bg-[var(--color-surface-2)]'
                          : 'border-[var(--color-border)] bg-[var(--color-surface-2)] hover:border-[var(--color-accent)]'
                    }`}
                  >
                    <div className="mb-1 flex flex-wrap items-center gap-1">
                      <ChannelBadge channel={f.channel} />
                      {f.severity && <SeverityBadge severity={f.severity} />}
                      {f.regressed && (
                        <span className="text-xs font-semibold text-[var(--color-danger)]">⚠</span>
                      )}
                      {f.occurrences > 1 && (
                        <span className="text-[11px] text-[var(--color-text-dim)]">
                          ×{f.occurrences}
                        </span>
                      )}
                    </div>
                    <div className="line-clamp-3 leading-snug">{f.title}</div>
                  </div>
                ))}
                {cards.length === 0 && (
                  <div className="px-1 py-2 text-xs text-[var(--color-text-dim)]">—</div>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {filtered.length === 0 && (
        <div className="mt-2 text-sm text-[var(--color-text-dim)]">
          {findings.length === 0
            ? 'Henüz bulgu yok. Bir tarama başlat.'
            : 'Filtreyle eşleşen bulgu yok.'}
        </div>
      )}

      <SelectionBar
        count={sel.count}
        onClear={sel.clear}
        onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}
      >
        <select
          value=""
          onChange={(e) => bulkStatus(e.target.value)}
          title="Seçili bulguların statüsünü değiştir"
          className="rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs outline-none focus:border-[var(--color-accent)]"
        >
          <option value="">↦ Statü…</option>
          {COLUMNS.map((c) => (
            <option key={c.key} value={c.key}>
              {c.label}
            </option>
          ))}
        </select>
        <SelectionBarButton icon={<LayoutGrid size={13} />} onClick={bulkCard}>
          Karta ekle
        </SelectionBarButton>
        <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
          Sil
        </SelectionBarButton>
      </SelectionBar>

      {modalFinding && (
        <FindingModal
          f={modalFinding}
          onClose={() => setModalId(null)}
          onStatus={setStatus}
          onDelete={remove}
          onAddCard={addCard}
          onOpenSession={onOpenSession}
        />
      )}
    </div>
  )
}

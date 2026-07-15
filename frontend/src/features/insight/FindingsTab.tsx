import { useMemo, useState } from 'react'
import { LayoutGrid, Check, X } from 'lucide-react'
import { api } from '@/api'
import type { InsightFinding, InsightLens } from '@/types'
import { SummaryHeader } from './SummaryHeader'
import { FilterBar } from './FilterBar'
import { FindingCard } from './FindingCard'
import { applyFilter, clusterFindings, summarize, type FindingFilter } from './insightHelpers'

interface Props {
  findings: InsightFinding[]
  lenses: InsightLens[]
  reload: () => void
  onOpenSession: (sid: string) => void
  onError: (msg: string) => void
  onNote: (msg: string) => void
}

const SEV2PRI: Record<string, string> = { high: 'high', med: 'medium', medium: 'medium', low: 'low' }

function cardBody(f: InsightFinding) {
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

export function FindingsTab({ findings, lenses, reload, onOpenSession, onError, onNote }: Props) {
  const [filter, setFilter] = useState<FindingFilter>({})
  const [cluster, setCluster] = useState(false)
  const [selected, setSelected] = useState<Set<string>>(new Set())

  const filtered = useMemo(() => applyFilter(findings, filter), [findings, filter])
  const summary = useMemo(() => summarize(findings), [findings])
  const clusters = useMemo(() => (cluster ? clusterFindings(filtered) : null), [cluster, filtered])

  const toggleSelect = (id: string) =>
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const setStatus = async (id: string, status: string) => {
    try {
      await api.setInsightFindingStatus(id, status)
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
    const ids = Array.from(selected)
    for (const id of ids) await api.setInsightFindingStatus(id, status).catch(() => {})
    setSelected(new Set())
    reload()
  }

  const bulkCard = async () => {
    const byId = new Map(findings.map((f) => [f.id, f]))
    for (const id of selected) {
      const f = byId.get(id)
      if (f) await addCard(f)
    }
    setSelected(new Set())
  }

  const renderCard = (f: InsightFinding, similar?: InsightFinding[]) => (
    <FindingCard
      key={f.id}
      f={f}
      selected={selected.has(f.id)}
      onSelect={toggleSelect}
      onStatus={setStatus}
      onAddCard={addCard}
      onOpenSession={onOpenSession}
      similar={similar}
    />
  )

  return (
    <div className="space-y-3">
      <SummaryHeader summary={summary} onPick={(patch) => setFilter({ ...patch })} />
      <FilterBar filter={filter} setFilter={setFilter} lenses={lenses} cluster={cluster} setCluster={setCluster} />

      {selected.size > 0 && (
        <div className="flex items-center gap-2 rounded-md border border-[var(--color-accent)]/40 bg-[var(--color-accent)]/10 p-2 text-sm">
          <span className="font-medium">{selected.size} seçili</span>
          <button onClick={() => bulkStatus('accepted')} className="flex items-center gap-1 rounded px-2 py-0.5 hover:bg-[var(--color-surface-2)]">
            <Check className="h-3.5 w-3.5" /> Kabul
          </button>
          <button onClick={() => bulkStatus('dismissed')} className="rounded px-2 py-0.5 text-[var(--color-text-muted)] hover:bg-[var(--color-surface-2)]">
            Yoksay
          </button>
          <button onClick={bulkCard} className="flex items-center gap-1 rounded bg-[var(--color-accent)] px-2 py-0.5 text-white">
            <LayoutGrid className="h-3.5 w-3.5" /> Karta ekle ({selected.size})
          </button>
          <button onClick={() => setSelected(new Set())} className="ml-auto rounded p-1 hover:bg-[var(--color-surface-2)]">
            <X className="h-4 w-4" />
          </button>
        </div>
      )}

      <div className="space-y-2">
        {clusters
          ? clusters.map((c) => renderCard(c.representative, c.members.slice(1)))
          : filtered.map((f) => renderCard(f))}
        {filtered.length === 0 && (
          <div className="text-sm text-[var(--color-text-muted)]">
            {findings.length === 0 ? 'Henüz bulgu yok. Bir tarama başlat.' : 'Filtreyle eşleşen bulgu yok.'}
          </div>
        )}
      </div>
    </div>
  )
}

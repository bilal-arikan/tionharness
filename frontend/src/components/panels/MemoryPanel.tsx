import { useEffect, useState, lazy, Suspense } from 'react'
import { Sparkles, ChevronDown, ChevronRight, List, Share2, Trash2, Paperclip } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Memory, MemoryKind } from '../../types'
import { Markdown } from '../markdown/Markdown'
import { SelectionBar, SelectionBarButton, PaneHeader } from '../common'
import { useMultiSelect } from '../../hooks/useMultiSelect'
import { CoreMemoryCard } from './CoreMemoryCard'
import { AgentAvatar } from '../agents/AgentAvatar'

// The knowledge-graph view pulls in React Flow (~300KB); load it only when the
// user switches to the graph tab.
const MemoryGraphView = lazy(() =>
  import('../graph/MemoryGraphView').then((m) => ({ default: m.MemoryGraphView })),
)

interface Props {
  agent: Agent | null
  onError: (msg: string) => void
  // Open the left agent roster (mobile drawer) — wired to the PaneHeader hamburger
  // since the memory screen is now headerless (its own top bar).
  onToggleList?: () => void
}

const KIND_LABEL: Record<MemoryKind, string> = {
  document: 'Belge',
  journal: 'Günlük',
  reflection: 'Yansıma',
}

const KIND_COLOR: Record<MemoryKind, string> = {
  document: 'bg-[var(--color-accent-soft)] text-[var(--color-text)]',
  journal: 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)]',
  reflection: 'bg-[color-mix(in_srgb,var(--color-success)_15%,transparent)] text-[var(--color-success)]',
}

const FILTERS: { key: MemoryKind | 'all'; label: string }[] = [
  { key: 'all', label: 'Tümü' },
  { key: 'document', label: 'Belge' },
  { key: 'journal', label: 'Günlük' },
  { key: 'reflection', label: 'Yansıma' },
]

export function MemoryPanel({ agent, onError, onToggleList }: Props) {
  const [memories, setMemories] = useState<Memory[]>([])
  const [content, setContent] = useState('')
  const [filter, setFilter] = useState<MemoryKind | 'all'>('all')
  const [reflecting, setReflecting] = useState(false)
  // Memory view mode: the flat list or the similarity knowledge graph. Defaults
  // to the graph so the knowledge network is the primary memory view.
  const [mode, setMode] = useState<'list' | 'graph'>('graph')
  // Cards render collapsed (a 4-line plain-text preview) by default; this tracks
  // which ids the user has expanded into the full markdown view.
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  // Multi-select (modifier-click only, so plain clicks still expand/collapse).
  const sel = useMultiSelect()

  const toggleExpanded = (id: string) =>
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })

  const reload = (agentId: string) =>
    api.listMemories(agentId).then(setMemories).catch((e) => onError(e.message))

  useEffect(() => {
    if (agent) reload(agent.id)
    else setMemories([])
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [agent?.id])

  if (!agent) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <PaneHeader title="Hafıza" onToggleList={onToggleList} />
        <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
          Hafızayı görmek için soldan bir ajan seç.
        </div>
      </div>
    )
  }

  const add = async () => {
    if (!content.trim()) return
    try {
      const m = await api.createMemory(agent.id, content.trim(), 'document')
      setMemories((prev) => [m, ...prev])
      setContent('')
    } catch (e) {
      onError((e as Error).message)
    }
  }

  const remove = async (m: Memory) => {
    setMemories((prev) => prev.filter((x) => x.id !== m.id))
    try {
      await api.deleteMemory(m.id)
    } catch (e) {
      onError((e as Error).message)
      reload(agent.id)
    }
  }

  const bulkDelete = async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    if (!confirm(`${ids.length} hafıza kaydı silinsin mi?`)) return
    setMemories((prev) => prev.filter((x) => !sel.selected.has(x.id)))
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.deleteMemory(id)))
    } catch (e) {
      onError((e as Error).message)
      reload(agent.id)
    }
  }

  const reflect = async () => {
    setReflecting(true)
    try {
      await api.reflect(agent.id)
      // Reflection consumes (deletes) the journals it consolidated, so a full
      // reload is required: prepending the new reflection alone would leave the
      // now-deleted journals stale on screen.
      await reload(agent.id)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setReflecting(false)
    }
  }

  const shown =
    filter === 'all' ? memories : memories.filter((m) => m.kind === filter)

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      {/* Top bar: knowledge input + add-document (attach icon) + reflect. */}
      <PaneHeader
        onToggleList={onToggleList}
        titleSlot={
          <span className="flex min-w-0 flex-1 items-center gap-2">
            {/* Selected agent's avatar, left of the knowledge input. */}
            <AgentAvatar agent={agent} size={22} />
            <input
              data-testid="memory-content-input"
              value={content}
              onChange={(e) => setContent(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && add()}
              placeholder={`${agent.name} için hatırlanacak bir bilgi ekle…`}
              className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </span>
        }
        right={
          <>
            <button
              data-testid="memory-add-document"
              onClick={add}
              title="Belge ekle"
              aria-label="Belge ekle"
              className="flex items-center justify-center rounded border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
            >
              <Paperclip size={15} />
            </button>
            <button
              data-testid="memory-reflect"
              onClick={reflect}
              disabled={reflecting}
              className="flex items-center gap-1.5 rounded border border-[var(--color-border)] px-3 py-1 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-40"
              title="Günlük üzerine yansıma üret (dream cycle)"
            >
              <Sparkles size={14} />
              {reflecting ? '…' : 'Yansıt'}
            </button>
          </>
        }
      />
      <div className="flex min-h-0 flex-1 flex-col p-4">
      {/* Core memory (MemGPT): the agent's persistent, always-in-context block. */}
      <CoreMemoryCard agentId={agent.id} onError={onError} />

      {/* Filters (list mode) + list/graph mode toggle */}
      <div className="mb-3 flex items-center gap-1">
        {mode === 'list' &&
          FILTERS.map((f) => (
            <button
              key={f.key}
              data-testid="memory-filter"
              data-filter={f.key}
              onClick={() => setFilter(f.key)}
              className={`rounded-md px-2 py-0.5 text-xs transition ${
                filter === f.key
                  ? 'bg-[var(--color-accent)] text-white'
                  : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
              }`}
            >
              {f.label}
            </button>
          ))}
        <div className="ml-auto flex gap-1 rounded-md bg-[var(--color-surface-2)] p-0.5">
          <button
            onClick={() => setMode('list')}
            className={`flex items-center gap-1 rounded px-2 py-0.5 text-xs transition ${
              mode === 'list' ? 'bg-[var(--color-accent)] text-white' : 'text-[var(--color-text-dim)]'
            }`}
          >
            <List size={13} /> Liste
          </button>
          <button
            onClick={() => setMode('graph')}
            className={`flex items-center gap-1 rounded px-2 py-0.5 text-xs transition ${
              mode === 'graph' ? 'bg-[var(--color-accent)] text-white' : 'text-[var(--color-text-dim)]'
            }`}
          >
            <Share2 size={13} /> Ağ
          </button>
        </div>
      </div>

      {mode === 'graph' ? (
        <Suspense
          fallback={
            <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
              Grafik yükleniyor…
            </div>
          }
        >
          <MemoryGraphView agentId={agent.id} onError={onError} />
        </Suspense>
      ) : (
      /* Memory list */
      <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex-1 space-y-2 overflow-y-auto">
        {shown.length === 0 && (
          <p className="text-sm text-[var(--color-text-dim)]">
            Bu kategoride hafıza yok.
          </p>
        )}
        {shown.map((m) => {
          const isOpen = expanded.has(m.id)
          // Show the expand toggle only when the content is long enough to be
          // clipped by the 4-line collapsed preview.
          const lineCount = (m.content.match(/\n/g)?.length ?? 0) + 1
          const isLong = lineCount > 4 || m.content.length > 200
          const orderedIds = shown.map((x) => x.id)
          return (
            <div
              key={m.id}
              // Modifier-click selects; plain clicks fall through to the inner
              // expand/delete handlers (which ignore modifier clicks).
              onClick={(e) => {
                if (e.ctrlKey || e.metaKey || e.shiftKey) sel.handleClick(e, m.id, orderedIds)
              }}
              className={`group flex items-start gap-3 rounded-lg border bg-[var(--color-surface)] px-3 py-2 text-sm transition ${
                sel.isSelected(m.id)
                  ? 'border-[var(--color-accent)] ring-1 ring-[var(--color-accent)]'
                  : 'border-[var(--color-border)]'
              }`}
            >
              <span
                className={`mt-0.5 flex-shrink-0 rounded px-1.5 py-0.5 text-xs ${KIND_COLOR[m.kind]}`}
              >
                {KIND_LABEL[m.kind]}
              </span>
              <div className="min-w-0 flex-1">
                {isOpen ? (
                  <Markdown>{m.content}</Markdown>
                ) : (
                  <p
                    className={`line-clamp-4 whitespace-pre-wrap break-words text-[var(--color-text)] ${
                      isLong ? 'cursor-pointer' : ''
                    }`}
                    onClick={(e) => {
                      if (e.ctrlKey || e.metaKey || e.shiftKey) return
                      if (isLong) toggleExpanded(m.id)
                    }}
                    title={isLong ? 'Genişletmek için tıkla' : undefined}
                  >
                    {m.content}
                  </p>
                )}
                {isLong && (
                  <button
                    data-testid="memory-card-expand"
                    data-memory-id={m.id}
                    onClick={(e) => {
                      if (e.ctrlKey || e.metaKey || e.shiftKey) return
                      toggleExpanded(m.id)
                    }}
                    className="mt-1 flex items-center gap-0.5 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                  >
                    {isOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                    {isOpen ? 'Daha az' : 'Daha fazla'}
                  </button>
                )}
              </div>
              <button
                data-testid="memory-delete"
                data-memory-id={m.id}
                onClick={(e) => {
                  if (e.ctrlKey || e.metaKey || e.shiftKey) return
                  remove(m)
                }}
                className="flex-shrink-0 text-[var(--color-text-dim)] opacity-0 transition hover:text-[var(--color-danger)] group-hover:opacity-100"
                title="Sil"
              >
                ✕
              </button>
            </div>
          )
        })}
      </div>

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={shown.length ? () => sel.selectAll(shown.map((m) => m.id)) : undefined}
        >
          <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
            Sil
          </SelectionBarButton>
        </SelectionBar>
      </div>
      )}
      </div>
    </div>
  )
}

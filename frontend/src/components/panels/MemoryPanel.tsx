import { useEffect, useState } from 'react'
import { Sparkles, ChevronDown, ChevronRight } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Memory, MemoryKind } from '../../types'
import { Markdown } from '../markdown/Markdown'

interface Props {
  agent: Agent | null
  onError: (msg: string) => void
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
  { key: 'document', label: 'Belgeler' },
  { key: 'journal', label: 'Günlük' },
  { key: 'reflection', label: 'Yansımalar' },
]

export function MemoryPanel({ agent, onError }: Props) {
  const [memories, setMemories] = useState<Memory[]>([])
  const [content, setContent] = useState('')
  const [filter, setFilter] = useState<MemoryKind | 'all'>('all')
  const [reflecting, setReflecting] = useState(false)
  // Cards render collapsed (a 4-line plain-text preview) by default; this tracks
  // which ids the user has expanded into the full markdown view.
  const [expanded, setExpanded] = useState<Set<string>>(new Set())

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
      <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
        Hafızayı görmek için soldan bir ajan seç.
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

  const reflect = async () => {
    setReflecting(true)
    try {
      const r = await api.reflect(agent.id)
      setMemories((prev) => [r, ...prev])
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setReflecting(false)
    }
  }

  const shown =
    filter === 'all' ? memories : memories.filter((m) => m.kind === filter)

  return (
    <div className="flex min-h-0 flex-1 flex-col p-4">
      {/* Add memory + reflect */}
      <div className="mb-3 flex gap-2">
        <input
          value={content}
          onChange={(e) => setContent(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && add()}
          placeholder={`${agent.name} için hatırlanacak bir bilgi ekle…`}
          className="flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-sm outline-none focus:border-[var(--color-accent)]"
        />
        <button
          onClick={add}
          className="rounded bg-[var(--color-accent)] px-3 py-1 text-sm font-medium text-white hover:opacity-90"
        >
          + Belge
        </button>
        <button
          onClick={reflect}
          disabled={reflecting}
          className="flex items-center gap-1.5 rounded border border-[var(--color-border)] px-3 py-1 text-sm font-medium text-[var(--color-text)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-40"
          title="Günlük üzerine yansıma üret (dream cycle)"
        >
          <Sparkles size={14} />
          {reflecting ? '…' : 'Yansıt'}
        </button>
      </div>

      {/* Filters */}
      <div className="mb-3 flex gap-1">
        {FILTERS.map((f) => (
          <button
            key={f.key}
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
      </div>

      {/* Memory list */}
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
          return (
            <div
              key={m.id}
              className="group flex items-start gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-sm"
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
                    onClick={() => isLong && toggleExpanded(m.id)}
                    title={isLong ? 'Genişletmek için tıkla' : undefined}
                  >
                    {m.content}
                  </p>
                )}
                {isLong && (
                  <button
                    onClick={() => toggleExpanded(m.id)}
                    className="mt-1 flex items-center gap-0.5 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
                  >
                    {isOpen ? <ChevronDown size={12} /> : <ChevronRight size={12} />}
                    {isOpen ? 'Daha az' : 'Daha fazla'}
                  </button>
                )}
              </div>
              <button
                onClick={() => remove(m)}
                className="flex-shrink-0 text-[var(--color-text-dim)] opacity-0 transition hover:text-red-400 group-hover:opacity-100"
                title="Sil"
              >
                ✕
              </button>
            </div>
          )
        })}
      </div>
    </div>
  )
}

import { useCallback, useEffect, useMemo, useState } from 'react'
import { FileText, Code2, Globe, Image, GitBranch, FileCode, Trash2, ExternalLink, Copy, Check } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { api } from '../api'
import type { Agent, Artifact, ArtifactKind } from '../types'
import { ArtifactView } from './artifacts/ArtifactView'
import { AgentAvatar } from './AgentAvatar'
import { relativeTime } from '../lib/time'

interface Props {
  onError: (msg: string) => void
  agents: Agent[]
  // Deep-link target: when set, select this artifact once loaded (from a chat card).
  selectedId?: string | null
  // Jump back to the artifact's origin chat session.
  onOpenSession?: (sessionId: string) => void
}

const KIND_ICON: Record<ArtifactKind, LucideIcon> = {
  markdown: FileText,
  code: Code2,
  html: Globe,
  text: FileText,
  svg: Image,
  mermaid: GitBranch,
}

const KIND_LABEL: Record<ArtifactKind, string> = {
  markdown: 'Markdown',
  code: 'Kod',
  html: 'HTML',
  text: 'Metin',
  svg: 'SVG',
  mermaid: 'Mermaid',
}

// ArtifactsPanel is the dedicated artifacts screen: a list of saved artifacts on
// the left and a viewer on the right with version history, copy and delete.
export function ArtifactsPanel({ onError, agents, selectedId, onOpenSession }: Props) {
  const [list, setList] = useState<Artifact[]>([])
  const [activeId, setActiveId] = useState<string | null>(selectedId ?? null)
  const [active, setActive] = useState<Artifact | null>(null)
  const [viewVersion, setViewVersion] = useState<number | null>(null) // null = current
  const [copied, setCopied] = useState(false)

  const reload = useCallback(() => {
    api
      .listArtifacts()
      .then((rows) => {
        setList(rows)
        setActiveId((cur) => cur ?? rows[0]?.id ?? null)
      })
      .catch((e) => onError((e as Error).message))
  }, [onError])

  useEffect(() => reload(), [reload])

  // Honour an incoming deep-link selection (e.g. clicking an artifact card).
  useEffect(() => {
    if (selectedId) setActiveId(selectedId)
  }, [selectedId])

  // Load the full artifact (with revisions) whenever the selection changes.
  useEffect(() => {
    if (!activeId) {
      setActive(null)
      return
    }
    setViewVersion(null)
    api
      .getArtifact(activeId)
      .then(setActive)
      .catch((e) => onError((e as Error).message))
  }, [activeId, onError])

  const remove = useCallback(
    async (id: string) => {
      if (!confirm('Bu artifact kalıcı olarak silinsin mi?')) return
      try {
        await api.deleteArtifact(id)
        setList((prev) => prev.filter((a) => a.id !== id))
        setActiveId((cur) => (cur === id ? null : cur))
      } catch (e) {
        onError((e as Error).message)
      }
    },
    [onError],
  )

  // The content to show: a chosen historical revision, or the current content.
  const shown = useMemo(() => {
    if (!active) return ''
    if (viewVersion == null || viewVersion === active.version) return active.content
    return active.revisions.find((r) => r.version === viewVersion)?.content ?? active.content
  }, [active, viewVersion])

  const copy = useCallback(() => {
    navigator.clipboard.writeText(shown).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    })
  }, [shown])

  const creator = (a: Artifact) => agents.find((ag) => ag.id === a.agentId) ?? null

  return (
    <div className="flex min-h-0 flex-1">
      {/* List */}
      <div className="flex w-72 flex-shrink-0 flex-col border-r border-[var(--color-border)]">
        <div className="border-b border-[var(--color-border)] px-4 py-3 text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
          Artifactlar · {list.length}
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {list.length === 0 && (
            <p className="px-2 py-6 text-center text-sm text-[var(--color-text-dim)]">
              Henüz artifact yok. Ajanlar <code className="text-[var(--color-accent)]">create_artifact</code> aracıyla içerik kaydedebilir.
            </p>
          )}
          {list.map((a) => {
            const Icon = KIND_ICON[a.kind] ?? FileText
            const isActive = a.id === activeId
            return (
              <button
                key={a.id}
                onClick={() => setActiveId(a.id)}
                className={`group mb-1 flex w-full items-start gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition ${
                  isActive
                    ? 'bg-[var(--color-accent-soft)] text-[var(--color-accent)]'
                    : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <Icon size={16} className="mt-0.5 shrink-0" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{a.title}</span>
                  <span className="mt-0.5 block text-[11px] text-[var(--color-text-dim)]">
                    {KIND_LABEL[a.kind] ?? a.kind} · v{a.version} · {relativeTime(a.updatedAt)}
                  </span>
                </span>
              </button>
            )
          })}
        </div>
      </div>

      {/* Viewer */}
      <div className="flex min-w-0 flex-1 flex-col">
        {!active ? (
          <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
            Görüntülemek için bir artifact seç.
          </div>
        ) : (
          <>
            <div className="flex items-center justify-between gap-3 border-b border-[var(--color-border)] px-5 py-3">
              <div className="flex min-w-0 items-center gap-2">
                <FileCode size={16} className="shrink-0 text-[var(--color-accent)]" />
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold">{active.title}</div>
                  <div className="flex items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]">
                    <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5">
                      {KIND_LABEL[active.kind] ?? active.kind}
                      {active.language ? ` · ${active.language}` : ''}
                    </span>
                    {creator(active) && (
                      <span className="flex items-center gap-1">
                        <AgentAvatar agent={creator(active)!} size={14} />
                        {creator(active)!.name}
                      </span>
                    )}
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-1.5">
                {/* Version selector when there is history. */}
                {active.revisions.length > 0 && (
                  <select
                    value={viewVersion ?? active.version}
                    onChange={(e) => setViewVersion(Number(e.target.value))}
                    className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-xs"
                    title="Sürüm geçmişi"
                  >
                    {[...active.revisions]
                      .map((r) => r.version)
                      .concat(active.version)
                      .sort((a, b) => b - a)
                      .map((v) => (
                        <option key={v} value={v}>
                          v{v}
                          {v === active.version ? ' (güncel)' : ''}
                        </option>
                      ))}
                  </select>
                )}
                <button
                  onClick={copy}
                  title="Kopyala"
                  className="rounded-md border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                >
                  {copied ? <Check size={15} /> : <Copy size={15} />}
                </button>
                {active.sessionId && onOpenSession && (
                  <button
                    onClick={() => onOpenSession(active.sessionId)}
                    title="Kaynak sohbete git"
                    className="rounded-md border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
                  >
                    <ExternalLink size={15} />
                  </button>
                )}
                <button
                  onClick={() => remove(active.id)}
                  title="Sil"
                  className="rounded-md border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] hover:bg-red-500/10 hover:text-red-400"
                >
                  <Trash2 size={15} />
                </button>
              </div>
            </div>
            <div className="min-h-0 flex-1 overflow-y-auto p-5">
              <ArtifactView kind={active.kind} language={active.language} content={shown} />
            </div>
          </>
        )}
      </div>
    </div>
  )
}

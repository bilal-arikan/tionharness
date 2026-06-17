import { useCallback, useEffect, useState } from 'react'
import {
  FileText, Code2, Globe, Image, GitBranch, FileCode,
  Trash2, ExternalLink, Copy, Check, Pencil, Plus, Save, X,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Artifact, ArtifactKind } from '../../types'
import { ArtifactView } from '../artifacts/ArtifactView'
import { AgentAvatar } from '../agents/AgentAvatar'
import { relativeTime } from '../../lib/time'

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

const KINDS: ArtifactKind[] = ['markdown', 'code', 'html', 'text', 'svg', 'mermaid']

// Draft holds the editable fields while creating or editing an artifact.
interface Draft {
  title: string
  kind: ArtifactKind
  language: string
  content: string
}

// ArtifactsPanel is the dedicated artifacts screen: a list of saved artifacts on
// the left and a viewer/editor on the right with copy, manual editing (overwrites
// in place) and delete. Artifacts are not versioned.
export function ArtifactsPanel({ onError, agents, selectedId, onOpenSession }: Props) {
  const [list, setList] = useState<Artifact[]>([])
  const [activeId, setActiveId] = useState<string | null>(selectedId ?? null)
  const [active, setActive] = useState<Artifact | null>(null)
  const [copied, setCopied] = useState(false)
  // Edit/create state. When `draft` is set the viewer becomes an editor.
  const [draft, setDraft] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)

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

  // Load the full artifact whenever the selection changes.
  useEffect(() => {
    if (!activeId) {
      setActive(null)
      return
    }
    setDraft(null)
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

  // Create a blank artifact and drop straight into edit mode.
  const createNew = useCallback(async () => {
    try {
      const a = await api.createArtifact({ title: 'Yeni artifact', kind: 'markdown', content: '' })
      setList((prev) => [a, ...prev])
      setActiveId(a.id)
      setActive(a)
      setDraft({ title: a.title, kind: a.kind, language: a.language, content: a.content })
    } catch (e) {
      onError((e as Error).message)
    }
  }, [onError])

  // Enter edit mode for the current artifact.
  const startEdit = useCallback(() => {
    if (!active) return
    setDraft({
      title: active.title,
      kind: active.kind,
      language: active.language,
      content: active.content,
    })
  }, [active])

  // Save the draft: send only changed fields (a content change overwrites in place).
  const save = useCallback(async () => {
    if (!active || !draft) return
    const patch: { title?: string; kind?: ArtifactKind; language?: string; content?: string } = {}
    if (draft.title.trim() && draft.title !== active.title) patch.title = draft.title.trim()
    if (draft.kind !== active.kind) patch.kind = draft.kind
    if (draft.language !== active.language) patch.language = draft.language
    if (draft.content !== active.content) patch.content = draft.content
    if (Object.keys(patch).length === 0) {
      setDraft(null)
      return
    }
    setSaving(true)
    try {
      const updated = await api.updateArtifact(active.id, patch)
      setActive(updated)
      setDraft(null)
      setList((prev) => prev.map((a) => (a.id === updated.id ? updated : a)))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }, [active, draft, onError])

  const copy = useCallback(() => {
    if (!active) return
    navigator.clipboard.writeText(active.content).then(() => {
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    })
  }, [active])

  const creator = (a: Artifact) => agents.find((ag) => ag.id === a.agentId) ?? null

  const iconBtn =
    'rounded-md border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'

  return (
    <div className="flex min-h-0 flex-1">
      {/* List */}
      <div className="flex w-72 flex-shrink-0 flex-col border-r border-[var(--color-border)]">
        <div className="flex items-center justify-between border-b border-[var(--color-border)] px-4 py-3">
          <span className="text-xs font-medium uppercase tracking-wide text-[var(--color-text-dim)]">
            Artifactlar · {list.length}
          </span>
          <button
            onClick={createNew}
            title="Yeni artifact"
            className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <Plus size={13} /> Yeni
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {list.length === 0 && (
            <p className="px-2 py-6 text-center text-sm text-[var(--color-text-dim)]">
              Henüz artifact yok. Bir oturumda dosya/doküman ürettiğinde otomatik buraya düşer; ya da <strong>Yeni</strong> ile elle ekle.
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
                    {KIND_LABEL[a.kind] ?? a.kind} · {relativeTime(a.updatedAt)}
                  </span>
                </span>
              </button>
            )
          })}
        </div>
      </div>

      {/* Viewer / Editor */}
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
                {draft ? (
                  <>
                    <button
                      onClick={save}
                      disabled={saving}
                      className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-2.5 py-1.5 text-xs font-medium text-white hover:brightness-110 disabled:opacity-50"
                    >
                      <Save size={14} /> {saving ? 'Kaydediliyor…' : 'Kaydet'}
                    </button>
                    <button onClick={() => setDraft(null)} title="İptal" className={iconBtn}>
                      <X size={15} />
                    </button>
                  </>
                ) : (
                  <>
                    <button onClick={startEdit} title="Düzenle" className={iconBtn}>
                      <Pencil size={15} />
                    </button>
                    <button onClick={copy} title="Kopyala" className={iconBtn}>
                      {copied ? <Check size={15} /> : <Copy size={15} />}
                    </button>
                    {active.sessionId && onOpenSession && (
                      <button
                        onClick={() => onOpenSession(active.sessionId)}
                        title="Kaynak sohbete git"
                        className={iconBtn}
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
                  </>
                )}
              </div>
            </div>

            {draft ? (
              // Editor
              <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <input
                    value={draft.title}
                    onChange={(e) => setDraft({ ...draft, title: e.target.value })}
                    placeholder="Başlık"
                    className="min-w-48 flex-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm"
                  />
                  <select
                    value={draft.kind}
                    onChange={(e) => setDraft({ ...draft, kind: e.target.value as ArtifactKind })}
                    className="rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-sm"
                  >
                    {KINDS.map((k) => (
                      <option key={k} value={k}>
                        {KIND_LABEL[k]}
                      </option>
                    ))}
                  </select>
                  {draft.kind === 'code' && (
                    <input
                      value={draft.language}
                      onChange={(e) => setDraft({ ...draft, language: e.target.value })}
                      placeholder="dil (ör. go)"
                      className="w-32 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm"
                    />
                  )}
                </div>
                <textarea
                  value={draft.content}
                  onChange={(e) => setDraft({ ...draft, content: e.target.value })}
                  placeholder="İçerik…"
                  spellCheck={false}
                  className="min-h-[40vh] flex-1 resize-none rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] p-3 font-mono text-xs leading-relaxed"
                />
                <p className="text-[11px] text-[var(--color-text-dim)]">
                  Kaydedince içerik yerinde güncellenir.
                </p>
              </div>
            ) : (
              <div className="min-h-0 flex-1 overflow-y-auto p-5">
                <ArtifactView kind={active.kind} language={active.language} content={active.content} />
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

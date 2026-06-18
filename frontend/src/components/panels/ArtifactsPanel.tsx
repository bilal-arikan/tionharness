import { useCallback, useEffect, useRef, useState } from 'react'
import {
  FileText, Code2, Globe, Image, GitBranch, FileCode,
  FileVideo, FileAudio, File as FileIcon, UploadCloud,
  Trash2, ExternalLink, Copy, Check, Pencil, Plus, Save, X, FolderOpen,
} from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { api } from '../../api'
import type { Agent, Artifact, ArtifactKind } from '../../types'
import { ArtifactView } from '../artifacts/ArtifactView'
import { CopyPathButton } from '../CopyPathButton'
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
  image: Image,
  video: FileVideo,
  audio: FileAudio,
  file: FileIcon,
}

const KIND_LABEL: Record<ArtifactKind, string> = {
  markdown: 'Markdown',
  code: 'Kod',
  html: 'HTML',
  text: 'Metin',
  svg: 'SVG',
  mermaid: 'Mermaid',
  image: 'Görsel',
  video: 'Video',
  audio: 'Ses',
  file: 'Dosya',
}

// Manually creatable kinds (text-based). Media/file kinds arrive via drag-drop.
const KINDS: ArtifactKind[] = ['markdown', 'code', 'html', 'text', 'svg', 'mermaid']

// Media/file kinds whose body lives on disk (sourcePath), not in an editable
// text field — these are added by dropping files, not typed.
const MEDIA_KINDS = new Set<ArtifactKind>(['image', 'video', 'audio', 'file'])
const isMediaKind = (k: ArtifactKind) => MEDIA_KINDS.has(k)

// artifactKindForUpload maps an uploaded attachment (coarse backend kind + name)
// to the artifact kind used to render it. Media stays media; small text/code/
// markdown gets its native renderer; everything else is a stored file card.
function artifactKindForUpload(att: { kind: string; name: string; textContent?: string }): ArtifactKind {
  switch (att.kind) {
    case 'image':
      return 'image'
    case 'video':
      return 'video'
    case 'audio':
      return 'audio'
  }
  const lower = att.name.toLowerCase()
  if (att.textContent) {
    if (lower.endsWith('.md') || lower.endsWith('.markdown')) return 'markdown'
    if (att.kind === 'code') return 'code'
    if (att.kind === 'text') return 'text'
  }
  return 'file'
}

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
  // On-disk path of the active artifact (its source file, or its store JSON),
  // loaded lazily so the copy-path / open-folder actions have a target.
  const [activePath, setActivePath] = useState<string>('')
  // Edit/create state. When `draft` is set the viewer becomes an editor.
  const [draft, setDraft] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)
  // Drag-and-drop file import state. dragDepth tracks nested dragenter/leave so
  // the overlay does not flicker when dragging over child elements.
  const [dragging, setDragging] = useState(false)
  const [importing, setImporting] = useState(false)
  const dragDepth = useRef(0)

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

  // Resolve the active artifact's on-disk path for the copy/open-folder actions.
  useEffect(() => {
    if (!activeId) {
      setActivePath('')
      return
    }
    let cancelled = false
    api
      .artifactPath(activeId)
      .then((r) => {
        if (!cancelled) setActivePath(r.path)
      })
      .catch(() => {
        if (!cancelled) setActivePath('')
      })
    return () => {
      cancelled = true
    }
  }, [activeId])

  // Open the active artifact's folder in the OS file manager (local desktop app).
  const reveal = useCallback(() => {
    if (!activeId) return
    api.revealArtifact(activeId).catch((e) => onError((e as Error).message))
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

  // Import dropped files: upload each into the workspace, then create an artifact
  // pointing at it (media renders inline; small text/code keeps its content).
  const importFiles = useCallback(
    async (files: File[]) => {
      if (files.length === 0) return
      setImporting(true)
      let firstId: string | null = null
      try {
        for (const file of files) {
          try {
            // A fixed "artifacts" bucket — these uploads are not tied to a chat session.
            const att = await api.uploadFile('artifacts', file)
            const kind = artifactKindForUpload(att)
            const created = await api.createArtifact({
              title: att.name,
              kind,
              content: att.textContent ?? '',
              // Media/file kinds reference the file on disk; text/code embed content.
              sourcePath: isMediaKind(kind) ? att.relPath : undefined,
            })
            if (!firstId) firstId = created.id
          } catch (e) {
            onError(`"${file.name}" eklenemedi: ${(e as Error).message}`)
          }
        }
        reload()
        if (firstId) setActiveId(firstId)
      } finally {
        setImporting(false)
      }
    },
    [onError, reload],
  )

  // Drag-and-drop handlers (depth-counted so nested elements don't flicker).
  const onDragEnter = useCallback((e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types).includes('Files')) return
    e.preventDefault()
    dragDepth.current += 1
    setDragging(true)
  }, [])
  const onDragOver = useCallback((e: React.DragEvent) => {
    if (Array.from(e.dataTransfer.types).includes('Files')) e.preventDefault()
  }, [])
  const onDragLeave = useCallback((e: React.DragEvent) => {
    if (!Array.from(e.dataTransfer.types).includes('Files')) return
    dragDepth.current = Math.max(0, dragDepth.current - 1)
    if (dragDepth.current === 0) setDragging(false)
  }, [])
  const onDrop = useCallback(
    (e: React.DragEvent) => {
      e.preventDefault()
      dragDepth.current = 0
      setDragging(false)
      const files = Array.from(e.dataTransfer.files)
      if (files.length) void importFiles(files)
    },
    [importFiles],
  )

  const creator = (a: Artifact) => agents.find((ag) => ag.id === a.agentId) ?? null

  const iconBtn =
    'rounded-md border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'

  return (
    <div
      className="relative flex min-h-0 flex-1"
      onDragEnter={onDragEnter}
      onDragOver={onDragOver}
      onDragLeave={onDragLeave}
      onDrop={onDrop}
    >
      {/* Drop overlay */}
      {(dragging || importing) && (
        <div className="pointer-events-none absolute inset-0 z-30 flex flex-col items-center justify-center gap-3 border-2 border-dashed border-[var(--color-accent)] bg-[color-mix(in_srgb,var(--color-accent)_12%,var(--color-bg))]/90 backdrop-blur-sm">
          <UploadCloud size={40} className="text-[var(--color-accent)]" />
          <p className="text-sm font-medium text-[var(--color-text)]">
            {importing ? 'Ekleniyor…' : 'Dosyaları bırak — resim, video, ses veya dosya'}
          </p>
        </div>
      )}

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
            <div className="flex flex-col items-center gap-2 px-4 py-10 text-center text-sm text-[var(--color-text-dim)]">
              <FileCode size={28} className="opacity-40" />
              <p>
                Henüz artifact yok. Bir oturumda dosya/doküman ürettiğinde otomatik buraya düşer; <strong>Yeni</strong> ile elle ekle; ya da <strong>resim/video/ses dosyalarını buraya sürükle-bırak</strong>.
              </p>
            </div>
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
                    <CopyPathButton path={activePath} />
                    <button
                      onClick={reveal}
                      disabled={!activePath}
                      title="Klasörü aç"
                      className={iconBtn}
                    >
                      <FolderOpen size={15} />
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
                      className="rounded-md border border-[var(--color-border)] p-1.5 text-[var(--color-text-dim)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] hover:text-[var(--color-danger)]"
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
                  {!isMediaKind(draft.kind) && (
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
                  )}
                  {draft.kind === 'code' && (
                    <input
                      value={draft.language}
                      onChange={(e) => setDraft({ ...draft, language: e.target.value })}
                      placeholder="dil (ör. go)"
                      className="w-32 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm"
                    />
                  )}
                </div>
                {isMediaKind(draft.kind) ? (
                  <>
                    <div className="flex-1 overflow-y-auto">
                      <ArtifactView
                        kind={active.kind}
                        language={active.language}
                        content={active.content}
                        sourcePath={active.sourcePath}
                      />
                    </div>
                    <p className="text-[11px] text-[var(--color-text-dim)]">
                      Medya dosyasının içeriği düzenlenemez — yalnız başlığı değiştirebilirsin.
                    </p>
                  </>
                ) : (
                  <>
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
                  </>
                )}
              </div>
            ) : (
              <div className="min-h-0 flex-1 overflow-y-auto p-5">
                <ArtifactView
                kind={active.kind}
                language={active.language}
                content={active.content}
                sourcePath={active.sourcePath}
              />
              </div>
            )}
          </>
        )}
      </div>
    </div>
  )
}

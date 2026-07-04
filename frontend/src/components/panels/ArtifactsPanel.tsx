import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  FileText, FileCode, UploadCloud,
  Trash2, ExternalLink, Copy, Check, Pencil, Save, X, Search,
} from 'lucide-react'
import { api } from '../../api'
import type { Agent, Artifact, ArtifactKind } from '../../types'
import { ArtifactView } from '../artifacts/ArtifactView'
import { CopyPathButton } from '../CopyPathButton'
import { RevealButton } from '../RevealButton'
import { AgentAvatar } from '../agents/AgentAvatar'
import { relativeTime } from '../../lib/time'
import { copyToClipboard } from '../../lib/clipboard'
import { useMultiSelect } from '../../hooks/useMultiSelect'
import { SelectionBar, SelectionBarButton, ListPane, PaneHeader } from '../common'
import { useCollapsibleList } from '../../hooks/useCollapsibleList'
import { useRegisterDirty } from '../../lib/dirtySignals'
import {
  SidebarHeader, RefreshButton, NewItemButton,
  SELECTED_ITEM_CLS, SELECTED_ITEM_RING,
} from '../common/SidebarChrome'
import {
  KIND_ICON, KIND_LABEL, OriginBadge, KINDS, isMediaKind, artifactKindForUpload,
} from './artifactMeta'

interface Props {
  onError: (msg: string) => void
  agents: Agent[]
  // Deep-link target: when set, select this artifact once loaded (from a chat card).
  selectedId?: string | null
  // Jump back to the artifact's origin chat session.
  onOpenSession?: (sessionId: string) => void
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
  // List filters: free-text title search + an origin facet (Tümü / chat / manual /
  // agent / tool).
  const [query, setQuery] = useState('')
  const [originFilter, setOriginFilter] = useState<'all' | 'chat' | 'manual' | 'agent' | 'tool' | 'plan'>('all')
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return list.filter((a) => {
      if (originFilter !== 'all' && (a.origin ?? '') !== originFilter) return false
      if (q && !a.title.toLowerCase().includes(q)) return false
      return true
    })
  }, [list, query, originFilter])

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

  // Multi-select (Ctrl/Cmd+Click, Shift-range) for bulk artifact deletion.
  const sel = useMultiSelect()
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('swarmgo.artifactsListOpen')
  const bulkDelete = useCallback(async () => {
    const ids = [...sel.selected]
    if (ids.length === 0) return
    if (!confirm(`${ids.length} artifact kalıcı olarak silinsin mi?`)) return
    setList((prev) => prev.filter((a) => !sel.selected.has(a.id)))
    setActiveId((cur) => (cur && sel.selected.has(cur) ? null : cur))
    sel.clear()
    try {
      await Promise.all(ids.map((id) => api.deleteArtifact(id)))
    } catch (e) {
      onError((e as Error).message)
      reload()
    }
  }, [sel, onError, reload])

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

  // Unsaved-edits flag: an open draft whose fields differ from the persisted
  // artifact. Surfaces on the nav "Artifactlar" item + workspace label.
  const dirty = useMemo(
    () =>
      !!draft &&
      !!active &&
      (draft.title !== active.title ||
        draft.kind !== active.kind ||
        draft.language !== active.language ||
        draft.content !== active.content),
    [draft, active],
  )
  useRegisterDirty('artifacts', dirty)

  const copy = useCallback(() => {
    if (!active) return
    copyToClipboard(active.content).then((ok) => {
      if (!ok) return
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
            // Sessionless manual uploads share the "_shared" bucket under artifacts/.
            const att = await api.uploadFile('_shared', file)
            const kind = artifactKindForUpload(att)
            const created = await api.createArtifact({
              title: att.name,
              kind,
              content: att.textContent ?? '',
              // Every kind now references its uploaded file on disk; text/code also
              // keep their content inline for the editor.
              sourcePath: att.relPath,
              origin: 'manual',
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
      className="relative flex h-full min-h-0 flex-1"
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

      {/* Standard list column (ListPane: collapse + mobile drawer + resize + theme) */}
      <ListPane
        open={listOpen}
        onToggle={toggleList}
        widthKey="swarmgo.artifactsListWidth"
        defaultWidth={288}
        label="Artifactlar"
        testId="artifacts-list-toggle"
        hideRail
      >
        <SidebarHeader
          title={`Artifactlar · ${filtered.length === list.length ? list.length : `${filtered.length}/${list.length}`}`}
        >
          <RefreshButton onClick={reload} />
        </SidebarHeader>
        <NewItemButton
          onClick={createNew}
          label="Yeni Artifact"
          title="Yeni artifact"
          testId="artifacts-create-new"
        />

        {/* Filters: title search + origin facet. */}
        <div className="flex flex-col gap-2 border-b border-[var(--color-border)] px-3 py-2">
          <div className="relative">
            <Search size={13} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--color-text-dim)]" />
            <input
              data-testid="artifacts-search-input"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Başlıkta ara…"
              className="w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] py-1.5 pl-7 pr-7 text-xs outline-none focus:border-[var(--color-accent)]"
            />
            {query && (
              <button
                onClick={() => setQuery('')}
                title="Temizle"
                className="absolute right-1.5 top-1/2 -translate-y-1/2 rounded p-0.5 text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
              >
                <X size={13} />
              </button>
            )}
          </div>
          <div className="flex flex-wrap gap-1">
            {([
              ['all', 'Tümü'],
              ['chat', 'Sohbet eki'],
              ['manual', 'Manuel'],
              ['agent', 'Ajan'],
              ['tool', 'Tool'],
              ['plan', 'Plan'],
            ] as const).map(([val, label]) => (
              <button
                key={val}
                data-testid="artifacts-filter"
                data-origin={val}
                onClick={() => setOriginFilter(val)}
                className={`rounded px-1.5 py-0.5 text-[10px] font-medium transition ${
                  originFilter === val
                    ? 'bg-[var(--color-accent)] text-white'
                    : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
                }`}
              >
                {label}
              </button>
            ))}
          </div>
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
          {list.length > 0 && filtered.length === 0 && (
            <div className="px-4 py-8 text-center text-sm text-[var(--color-text-dim)]">
              Filtreyle eşleşen artifact yok.
            </div>
          )}
          {filtered.map((a) => {
            const Icon = KIND_ICON[a.kind] ?? FileText
            const isActive = a.id === activeId
            const orderedIds = filtered.map((x) => x.id)
            return (
              <button
                key={a.id}
                data-testid="artifacts-list-item"
                data-artifact-id={a.id}
                onClick={(e) => {
                  if (sel.handleClick(e, a.id, orderedIds)) return
                  setActiveId(a.id)
                }}
                className={`group mb-1 flex w-full items-start gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition ${
                  sel.isSelected(a.id)
                    ? `${SELECTED_ITEM_CLS} ${SELECTED_ITEM_RING}`
                    : isActive
                      ? SELECTED_ITEM_CLS
                      : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <Icon size={16} className="mt-0.5 shrink-0" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate font-medium">{a.title}</span>
                  <span className="mt-1 flex flex-wrap items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]">
                    <OriginBadge origin={a.origin} />
                    <span>{KIND_LABEL[a.kind] ?? a.kind} · {relativeTime(a.updatedAt)}</span>
                  </span>
                </span>
              </button>
            )
          })}
        </div>

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={filtered.length ? () => sel.selectAll(filtered.map((a) => a.id)) : undefined}
        >
          <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
            Sil
          </SelectionBarButton>
        </SelectionBar>
      </ListPane>

      {/* Viewer / Editor column: the standard title bar sits ONLY here, to the
          right of the list — like the chat header (never spans over the list). */}
      <div className="flex min-w-0 flex-1 flex-col">
        <PaneHeader
          listOpen={listOpen}
          onToggleList={toggleList}
          // Detail view: the top bar hosts the artifact identity + actions (no
          // redundant "Artifactlar" title / subtitle). Empty state keeps the title.
          title={active ? undefined : 'Artifactlar'}
          titleSlot={
            active ? (
              <div className="flex min-w-0 items-center gap-2">
                <FileCode size={16} className="shrink-0 text-[var(--color-accent)]" />
                <div className="min-w-0">
                  <div className="truncate text-sm font-semibold">{active.title}</div>
                  <div className="flex flex-wrap items-center gap-1.5 text-[11px] text-[var(--color-text-dim)]">
                    <OriginBadge origin={active.origin} />
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
            ) : undefined
          }
          right={
            active ? (
              <div className="flex flex-wrap items-center gap-1.5">
                {draft ? (
                  <>
                    <button
                      data-testid="artifact-edit-save"
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
                    <button data-testid="artifact-detail-edit" onClick={startEdit} title="Düzenle" className={iconBtn}>
                      <Pencil size={15} />
                    </button>
                    <button
                      data-testid="artifact-detail-copy"
                      onClick={copy}
                      title="İçeriği kopyala"
                      className="flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                    >
                      {copied ? <Check size={14} className="text-[var(--color-success)]" /> : <Copy size={14} />}
                      <span>{copied ? 'Kopyalandı' : 'İçeriği kopyala'}</span>
                    </button>
                    <CopyPathButton path={activePath} label="Yolu kopyala" labelClassName="hidden" title="Yolu kopyala" />
                    <RevealButton testId="artifact-detail-reveal" onReveal={reveal} disabled={!activePath} label="Aç" labelClassName="hidden sm:inline" />
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
                      data-testid="artifact-detail-delete"
                      onClick={() => remove(active.id)}
                      title="Sil"
                      className="flex items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2.5 py-1.5 text-xs text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] disabled:opacity-50"
                    >
                      <Trash2 size={14} /> Sil
                    </button>
                  </>
                )}
              </div>
            ) : undefined
          }
        />
        {!active ? (
          <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
            Görüntülemek için bir artifact seç.
          </div>
        ) : (
          <>
            {draft ? (
              // Editor
              <div className="flex min-h-0 flex-1 flex-col gap-3 overflow-y-auto p-5">
                <div className="flex flex-wrap items-center gap-2">
                  <input
                    data-testid="artifact-edit-title-input"
                    value={draft.title}
                    onChange={(e) => setDraft({ ...draft, title: e.target.value })}
                    placeholder="Başlık"
                    className="min-w-48 flex-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm"
                  />
                  {!isMediaKind(draft.kind) && (
                    <select
                      data-testid="artifact-edit-kind-select"
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
                      data-testid="artifact-edit-content-textarea"
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

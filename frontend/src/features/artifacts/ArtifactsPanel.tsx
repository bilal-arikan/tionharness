import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useSessionState } from '@/shared/hooks/useSessionState'
import {
  FileText, FileCode, UploadCloud,
  Trash2, ExternalLink, Copy, Check, Pencil, Save, X, Search,
  ChevronDown, ChevronRight, ChevronsDownUp, ChevronsUpDown, FolderInput,
  Archive, ArchiveRestore,
} from 'lucide-react'
import { api } from '@/api'
import type { Agent, Artifact, ArtifactKind } from '@/types'
import { ArtifactView } from './ArtifactView'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { RevealButton } from '@/shared/components/RevealButton'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { relativeTime } from '@/shared/lib/time'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useGroupedList } from '@/shared/hooks/useGroupedList'
import { useGroupDnD } from '@/shared/hooks/useGroupDnD'
import { SelectionBar, SelectionBarButton, ListPane, PaneHeader, LoadingState } from '@/shared/components'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import {
  SidebarHeader, RefreshButton, NewItemButton,
  SELECTED_ITEM_CLS, SELECTED_ITEM_RING,
} from '@/shared/components/SidebarChrome'
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

// Label for the bucket holding artifacts with no `group` set; always rendered last.
const UNGROUPED = 'Grupsuz'

// Group key for one artifact: its `group` field, or the ungrouped bucket.
function artifactGroupKey(a: Artifact): string {
  return a.group?.trim() || UNGROUPED
}

// Identity of one artifact row (module-scope so the drag hook's lookup map is stable).
function artifactId(a: Artifact): string {
  return a.id
}

// Order groups: named groups alphabetically (tr) first, ungrouped bucket last.
function sortArtifactGroups(a: string, b: string): number {
  if (a === UNGROUPED) return 1
  if (b === UNGROUPED) return -1
  return a.localeCompare(b, 'tr')
}

// Draft holds the editable fields while creating or editing an artifact.
interface Draft {
  title: string
  kind: ArtifactKind
  language: string
  content: string
  // Organisation bucket the artifact belongs to. Empty string = ungrouped.
  // Persisted through the dedicated group endpoint (not the content/meta patch).
  group: string
}

// ArtifactsPanel is the dedicated artifacts screen: a list of saved artifacts on
// the left and a viewer/editor on the right with copy, manual editing (overwrites
// in place) and delete. Artifacts are not versioned.
export function ArtifactsPanel({ onError, agents, selectedId, onOpenSession }: Props) {
  const [list, setList] = useState<Artifact[]>([])
  // Selection persists across screen switches within the session (resets on app
  // reload). A deep-link `selectedId` still overrides via the effect below.
  const [activeId, setActiveId] = useSessionState<string | null>('artifacts.activeId', selectedId ?? null)
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
  // Archived view toggle: false (default) hides archived artifacts and shows only
  // active ones; true flips to show ONLY archived artifacts (so they can be
  // reviewed and un-archived). Persisted so switching screens keeps the view.
  const [showArchived, setShowArchived] = useSessionState<boolean>('artifacts.showArchived', false)
  // Count of archived artifacts across the whole list — drives the toggle's badge
  // and lets us hide the toggle entirely when nothing has been archived yet.
  const archivedCount = useMemo(() => list.filter((a) => a.archived).length, [list])
  // True until the first artifact list lands — the list column shows a loading
  // state rather than the "no artifacts yet" onboarding copy.
  const [loading, setLoading] = useState(true)
  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return list.filter((a) => {
      // Archive facet: the default view lists active artifacts only; the archived
      // view lists archived ones only. They are never mixed.
      if (!!a.archived !== showArchived) return false
      if (originFilter !== 'all' && (a.origin ?? '') !== originFilter) return false
      if (q && !a.title.toLowerCase().includes(q)) return false
      return true
    })
  }, [list, query, originFilter, showArchived])

  const reload = useCallback(() => {
    api
      .listArtifacts()
      .then((rows) => {
        setList(rows)
        setActiveId((cur) => cur ?? rows[0]?.id ?? null)
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
  }, [onError])

  useEffect(() => reload(), [reload])

  // Once nothing is archived any more (e.g. the last archived artifact was
  // restored), fall back to the active view so the archived view can't strand
  // the user on a permanently empty list.
  useEffect(() => {
    if (showArchived && archivedCount === 0) setShowArchived(false)
  }, [showArchived, archivedCount, setShowArchived])

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
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionswarm.artifactsListOpen')
  // Draft group name + busy flag for the bulk "set group" action on the selection.
  const [bulkGroup, setBulkGroup] = useState('')
  const [bulkGroupBusy, setBulkGroupBusy] = useState(false)
  // Busy flag for the bulk archive / un-archive action on the selection.
  const [bulkArchiveBusy, setBulkArchiveBusy] = useState(false)

  // Filtered artifacts bucketed by group (named groups first, ungrouped last),
  // with persisted per-group collapse state. Mirrors the Skills screen so both
  // list screens organise the same way. Grouping runs on the already-filtered
  // list so search/origin facets still apply.
  const {
    groups: grouped,
    collapsed,
    toggle: toggleGroup,
    allCollapsed,
    toggleAll,
  } = useGroupedList(filtered, {
    keyOf: artifactGroupKey,
    sortGroups: sortArtifactGroups,
    persistKey: 'tionswarm.artifactsCollapsedGroups',
  })
  // Distinct existing group names (across the full list, not just the filtered
  // view), offered as bulk-group autocomplete suggestions.
  const groupNames = useMemo(
    () => [...new Set(list.map((a) => a.group?.trim()).filter((g): g is string => !!g))].sort((a, b) => a.localeCompare(b, 'tr')),
    [list],
  )
  // Flattened visible (non-collapsed) id order, so a Shift+Click range can cross
  // group boundaries but skips folded groups. Feeds multi-select + select-all.
  const orderedIds = useMemo(
    () => grouped.flatMap(([name, items]) => (collapsed.has(name) ? [] : items.map((a) => a.id))),
    [grouped, collapsed],
  )

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

  // Bulk-set the `group` of every selected artifact at once, so a batch lands
  // under one collapsible header without opening each artifact. An empty group
  // ungroups them. Keeps the selection so the user can chain another action; the
  // list re-buckets after the reload.
  const bulkSetGroup = useCallback(
    (group: string) => {
      const ids = [...sel.selected]
      if (ids.length === 0) return
      setBulkGroupBusy(true)
      Promise.all(ids.map((id) => api.setArtifactGroup(id, group)))
        .then(() => {
          setBulkGroup('')
          reload()
          if (activeId && sel.selected.has(activeId)) {
            api.getArtifact(activeId).then(setActive).catch(() => {})
          }
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => setBulkGroupBusy(false))
    },
    [sel.selected, reload, activeId, onError],
  )

  // Bulk archive / un-archive every selected artifact at once (a soft, reversible
  // hide). In the default view this archives the selection; in the archived view
  // it restores it. Clears the selection since the affected cards leave the
  // current view after the reload.
  const bulkSetArchived = useCallback(
    (archived: boolean) => {
      const ids = [...sel.selected]
      if (ids.length === 0) return
      setBulkArchiveBusy(true)
      sel.clear()
      Promise.all(ids.map((id) => api.setArtifactArchived(id, archived)))
        .then(() => {
          // The affected artifacts drop out of the current view; clear the
          // detail selection if it was one of them so the viewer doesn't dangle.
          setActiveId((cur) => (cur && ids.includes(cur) ? null : cur))
        })
        .catch((e) => onError((e as Error).message))
        .finally(() => {
          setBulkArchiveBusy(false)
          reload()
        })
    },
    [sel, reload, onError],
  )

  // Single-artifact archive / un-archive from the detail toolbar. Keeps the list
  // and the active artifact in sync after the flip.
  const setArchived = useCallback(
    async (id: string, archived: boolean) => {
      try {
        const updated = await api.setArtifactArchived(id, archived)
        setList((prev) => prev.map((a) => (a.id === id ? updated : a)))
        setActive((cur) => (cur && cur.id === id ? updated : cur))
        // An archived artifact leaves the default view (and vice-versa); drop the
        // selection so the viewer clears rather than showing a now-hidden card.
        setActiveId((cur) => (cur === id ? null : cur))
      } catch (e) {
        onError((e as Error).message)
      }
    },
    [onError],
  )

  // Drag-and-drop group move: dropping a card on a group header rewrites its
  // `group` through the same endpoint the bulk action uses. Dragging a card that
  // belongs to the current selection moves the whole selection.
  const moveToGroup = useCallback(
    (ids: string[], group: string) => {
      Promise.all(ids.map((id) => api.setArtifactGroup(id, group)))
        .then(() => {
          if (activeId && ids.includes(activeId)) return api.getArtifact(activeId).then(setActive)
        })
        .catch((e) => onError((e as Error).message))
        // Refresh either way: on success to re-bucket the list, on failure so the
        // cards snap back to the persisted truth instead of a half-applied move.
        .finally(() => reload())
    },
    [activeId, reload, onError],
  )
  const dnd = useGroupDnD<Artifact>({
    items: list,
    idOf: artifactId,
    groupOf: artifactGroupKey,
    ungroupedLabel: UNGROUPED,
    selectedIds: sel.selected,
    onMove: moveToGroup,
  })

  // Create a blank artifact and drop straight into edit mode.
  const createNew = useCallback(async () => {
    try {
      const a = await api.createArtifact({ title: 'Yeni artifact', kind: 'markdown', content: '' })
      setList((prev) => [a, ...prev])
      setActiveId(a.id)
      setActive(a)
      setDraft({ title: a.title, kind: a.kind, language: a.language, content: a.content, group: a.group ?? '' })
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
      group: active.group ?? '',
    })
  }, [active])

  // Save the draft: send only changed fields (a content change overwrites in
  // place). The `group` field is not part of the content/meta patch — the
  // backend `updateArtifact` handler ignores it — so a group change is persisted
  // through the dedicated `setArtifactGroup` endpoint (same one bulk + DnD use).
  const save = useCallback(async () => {
    if (!active || !draft) return
    const patch: { title?: string; kind?: ArtifactKind; language?: string; content?: string } = {}
    if (draft.title.trim() && draft.title !== active.title) patch.title = draft.title.trim()
    if (draft.kind !== active.kind) patch.kind = draft.kind
    if (draft.language !== active.language) patch.language = draft.language
    if (draft.content !== active.content) patch.content = draft.content
    const nextGroup = draft.group.trim()
    const groupChanged = nextGroup !== (active.group ?? '')
    if (Object.keys(patch).length === 0 && !groupChanged) {
      setDraft(null)
      return
    }
    setSaving(true)
    try {
      // Apply the content/meta patch first (if any), then the group change; the
      // last response is the authoritative post-save artifact.
      let updated = active
      if (Object.keys(patch).length > 0) {
        updated = await api.updateArtifact(active.id, patch)
      }
      if (groupChanged) {
        updated = await api.setArtifactGroup(active.id, nextGroup)
      }
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
        draft.content !== active.content ||
        draft.group.trim() !== (active.group ?? '')),
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
        widthKey="tionswarm.artifactsListWidth"
        defaultWidth={288}
        label="Artifactlar"
        testId="artifacts-list-toggle"
        hideRail
      >
        <SidebarHeader
          title={`Artifactlar · ${filtered.length === list.length ? list.length : `${filtered.length}/${list.length}`}`}
        >
          {grouped.length > 1 && (
            <button
              data-testid="artifacts-toggle-all"
              onClick={toggleAll}
              title={allCollapsed ? 'Tüm grupları aç' : 'Tüm grupları katla'}
              className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              {allCollapsed ? <ChevronsUpDown size={13} /> : <ChevronsDownUp size={13} />}
            </button>
          )}
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
          {/* Archived view toggle: flips the list between active and archived
              artifacts. Hidden until at least one artifact has been archived. */}
          {(archivedCount > 0 || showArchived) && (
            <button
              data-testid="artifacts-archived-toggle"
              data-active={showArchived}
              onClick={() => setShowArchived((v) => !v)}
              title={showArchived ? 'Aktif artifactlara dön' : 'Arşivlenen artifactları göster'}
              className={`flex items-center gap-1.5 self-start rounded px-1.5 py-0.5 text-[10px] font-medium transition ${
                showArchived
                  ? 'bg-[var(--color-accent)] text-white'
                  : 'bg-[var(--color-surface-2)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'
              }`}
            >
              <Archive size={12} />
              {showArchived ? 'Arşiv görünümü' : `Arşiv (${archivedCount})`}
            </button>
          )}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto p-2">
          {loading && <LoadingState label="Artifact'ler yükleniyor…" />}
          {!loading && list.length === 0 && (
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
          {grouped.map(([groupName, items]) => {
            const isCollapsed = collapsed.has(groupName)
            const isDropTarget = dnd.isOver(groupName)
            return (
              // The whole group (header + body) is the drop target, so a card can
              // be dropped on a folded group's header too.
              <div key={groupName} className="mb-1" {...dnd.groupProps(groupName)}>
                <button
                  data-testid="artifacts-group-header"
                  data-group-name={groupName}
                  data-collapsed={isCollapsed}
                  data-drop-active={isDropTarget}
                  onClick={() => toggleGroup(groupName)}
                  title={isCollapsed ? 'Grubu aç' : 'Grubu katla'}
                  className={`flex w-full items-center gap-1.5 rounded-md px-1.5 py-1 text-left text-[11px] font-semibold uppercase tracking-wide hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] ${
                    isDropTarget
                      ? `${SELECTED_ITEM_CLS} ${SELECTED_ITEM_RING}`
                      : 'text-[var(--color-text-dim)]'
                  }`}
                >
                  {isCollapsed ? <ChevronRight size={13} /> : <ChevronDown size={13} />}
                  <span className="min-w-0 flex-1 truncate">{groupName}</span>
                  <span className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] tabular-nums text-[var(--color-text-dim)]">
                    {items.length}
                  </span>
                </button>
                {!isCollapsed && (
                  <div className="mt-1 space-y-1 pl-1.5">
                    {items.map((a) => {
                      const Icon = KIND_ICON[a.kind] ?? FileText
                      const isActive = a.id === activeId
                      return (
                        <button
                          key={a.id}
                          data-testid="artifacts-list-item"
                          data-artifact-id={a.id}
                          {...dnd.itemProps(a)}
                          onClick={(e) => {
                            // A drag may end with a trailing click on the source
                            // card; that click must not change the selection.
                            if (dnd.consumedByDrag()) return
                            if (sel.handleClick(e, a.id, orderedIds, activeId)) return
                            setActiveId(a.id)
                          }}
                          className={`group flex w-full items-start gap-2 rounded-lg px-2.5 py-2 text-left text-sm transition ${
                            sel.isSelected(a.id)
                              ? `${SELECTED_ITEM_CLS} ${SELECTED_ITEM_RING}`
                              : isActive
                                ? SELECTED_ITEM_CLS
                                : 'text-[var(--color-text)] hover:bg-[var(--color-surface-2)]'
                          } ${dnd.dragIds.has(a.id) ? 'opacity-50' : ''}`}
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
                )}
              </div>
            )
          })}
        </div>

        <SelectionBar
          count={sel.count}
          onClear={sel.clear}
          onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}
        >
          {/* Bulk group: move every selected artifact into one organisation bucket. */}
          <div data-testid="artifacts-bulk-group" className="inline-flex items-center gap-1">
            <input
              list="artifacts-bulk-group-names"
              value={bulkGroup}
              onChange={(e) => setBulkGroup(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  bulkSetGroup(bulkGroup.trim())
                }
              }}
              disabled={bulkGroupBusy}
              placeholder="Grup ata…"
              data-testid="artifacts-bulk-group-input"
              className="w-28 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs text-[var(--color-text)] outline-none focus:border-[var(--color-accent)] disabled:opacity-50"
            />
            <datalist id="artifacts-bulk-group-names">
              {groupNames.map((n) => (
                <option key={n} value={n} />
              ))}
            </datalist>
            <SelectionBarButton
              icon={<FolderInput size={13} />}
              onClick={() => bulkSetGroup(bulkGroup.trim())}
              disabled={bulkGroupBusy}
              title={bulkGroup.trim() ? `Seçili artifactları "${bulkGroup.trim()}" grubuna taşı` : 'Seçili artifactları grupsuz yap'}
            >
              {bulkGroup.trim() ? 'Ata' : 'Grupsuz'}
            </SelectionBarButton>
          </div>
          {/* Bulk archive / un-archive: hides (or restores) the selection. The
              verb follows the current view — archive in the active view, restore
              in the archived view. */}
          <SelectionBarButton
            icon={showArchived ? <ArchiveRestore size={13} /> : <Archive size={13} />}
            onClick={() => bulkSetArchived(!showArchived)}
            disabled={bulkArchiveBusy}
            title={showArchived ? 'Seçili artifactları arşivden çıkar' : 'Seçili artifactları arşivle'}
          >
            {showArchived ? 'Arşivden çıkar' : 'Arşivle'}
          </SelectionBarButton>
          <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
            Sil
          </SelectionBarButton>
        </SelectionBar>
      </ListPane>

      {/* Viewer / Editor column: the standard title bar sits ONLY here, to the
          right of the list — like the chat header (never spans over the list). */}
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <PaneHeader
          listOpen={listOpen}
          onToggleList={toggleList}
          // Detail view: the top bar hosts the artifact identity + actions (no
          // redundant "Artifactlar" title / subtitle). Empty state keeps the title.
          // Chips + the İçerik (content-copy) button always live on their own second
          // row (every width), so the first row stays compact.
          secondaryAlwaysWrap
          title={active ? undefined : 'Artifactlar'}
          titleSlot={
            active ? (
              <div className="flex min-w-0 items-center gap-2">
                <FileCode size={16} className="shrink-0 text-[var(--color-accent)]" />
                <div className="truncate text-sm font-semibold">{active.title}</div>
              </div>
            ) : undefined
          }
          secondary={
            active ? (
              <>
                <OriginBadge origin={active.origin} />
                {active.archived && (
                  <span className="flex items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[color-mix(in_srgb,var(--color-warning)_15%,transparent)] text-[var(--color-warning)]">
                    <Archive size={11} /> Arşivlendi
                  </span>
                )}
                {active.group && (
                  <span className="rounded px-1.5 py-0.5 text-[10px] font-medium uppercase tracking-wide bg-[var(--color-surface-2)] text-[var(--color-text-dim)]">
                    {active.group}
                  </span>
                )}
                <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)]">
                  {KIND_LABEL[active.kind] ?? active.kind}
                  {active.language ? ` · ${active.language}` : ''}
                </span>
                {creator(active) && (
                  <span className="flex items-center gap-1 text-[11px] text-[var(--color-text-dim)]">
                    <AgentAvatar agent={creator(active)!} size={14} />
                    {creator(active)!.name}
                  </span>
                )}
                {!draft && (
                  <button
                    data-testid="artifact-detail-copy"
                    onClick={copy}
                    title="İçeriği kopyala"
                    className="ml-auto flex shrink-0 items-center gap-1.5 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                  >
                    {copied ? <Check size={14} className="text-[var(--color-success)]" /> : <Copy size={14} />}
                    <span>{copied ? 'Kopyalandı' : 'İçerik'}</span>
                  </button>
                )}
              </>
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
                      data-testid="artifact-detail-archive"
                      onClick={() => setArchived(active.id, !active.archived)}
                      title={active.archived ? 'Arşivden çıkar' : 'Arşivle'}
                      className={iconBtn}
                    >
                      {active.archived ? <ArchiveRestore size={15} /> : <Archive size={15} />}
                    </button>
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
                  {/* Per-artifact group: edit this single artifact's organisation
                      bucket directly, without entering multi-select or dragging.
                      Autocompletes to existing group names; blank = ungrouped. */}
                  <input
                    data-testid="artifact-edit-group-input"
                    list="artifacts-group-names"
                    value={draft.group}
                    onChange={(e) => setDraft({ ...draft, group: e.target.value })}
                    placeholder="Grup (opsiyonel)"
                    title="Bu artifact'in grubu — boş bırakırsan grupsuz olur"
                    className="w-40 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2.5 py-1.5 text-sm"
                  />
                  <datalist id="artifacts-group-names">
                    {groupNames.map((n) => (
                      <option key={n} value={n} />
                    ))}
                  </datalist>
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

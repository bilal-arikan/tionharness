import { ChevronsDownUp, ChevronsUpDown, UploadCloud } from 'lucide-react'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_ARTIFACTS } from '@/app/eventToRefreshSignals'
import type { Agent } from '@/types'
import { ArtifactView } from './ArtifactView'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { ListPane } from '@/shared/components'
import { useCollapsibleList } from '@/shared/hooks/useCollapsibleList'
import { SidebarHeader, RefreshButton, NewItemButton } from '@/shared/components/SidebarChrome'
import { ArtifactBulkBar } from './ArtifactBulkBar'
import { ArtifactDetailHeader } from './ArtifactDetailHeader'
import { ArtifactEditor } from './ArtifactEditor'
import { ArtifactGroupedList } from './ArtifactGroupedList'
import { ArtifactListFilters } from './ArtifactListFilters'
import { useArtifactBulkActions } from './useArtifactBulkActions'
import { useArtifactDetail } from './useArtifactDetail'
import { useArtifactImport } from './useArtifactImport'
import { useArtifactList } from './useArtifactList'

interface Props {
  onError: (msg: string) => void
  agents: Agent[]
  // Deep-link target: when set, select this artifact once loaded (from a chat card).
  selectedId?: string | null
  // Jump back to the artifact's origin chat session.
  onOpenSession?: (sessionId: string) => void
}

// ArtifactsPanel is the dedicated artifacts screen: a list of saved artifacts on
// the left and a viewer/editor on the right with copy, manual editing (overwrites
// in place) and delete. Artifacts are not versioned.
//
// The screen's state lives in four hooks, wired here in dependency order:
// useArtifactList (list + filters + paging) → useArtifactDetail (active
// artifact + draft + the unsaved-draft guard) → useArtifactBulkActions
// (multi-select + DnD) → useArtifactImport (file drop). The order matters:
// everything that can replace the selection needs `confirmDiscard`, which the
// detail hook derives from the open draft.
export function ArtifactsPanel({ onError, agents, selectedId, onOpenSession }: Props) {
  const artifactsTick = useRefreshTrigger(SIGNAL_ARTIFACTS)
  const listState = useArtifactList(selectedId, artifactsTick, onError)
  const { list, setList, activeId, setActiveId, reload, groupNames, orderedIds } = listState

  const detail = useArtifactDetail({ activeId, setActiveId, setList, onError })
  const { active, draft, setDraft } = detail

  // Multi-select (Ctrl/Cmd+Click, Shift-range) for bulk artifact actions.
  const sel = useMultiSelect()
  const { open: listOpen, toggle: toggleList } = useCollapsibleList('tionharness.artifactsListOpen')

  const bulk = useArtifactBulkActions({
    list,
    sel,
    activeId,
    setActiveId,
    setList,
    setActive: detail.setActive,
    reload,
    onError,
  })

  const { dragging, importing, dropZoneProps } = useArtifactImport({
    onError,
    reload,
    selectArtifact: detail.selectArtifact,
  })

  return (
    <div className="relative flex h-full min-h-0 flex-1" {...dropZoneProps}>
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
        widthKey="tionharness.artifactsListWidth"
        defaultWidth={288}
        label="Artifactlar"
        testId="artifacts-list-toggle"
        hideRail
      >
        <SidebarHeader
          title={`Artifactlar · ${list.length}${listState.hasMore ? ` / ${listState.total}` : ''}`}
        >
          {listState.grouped.length > 1 && (
            <button
              data-testid="artifacts-toggle-all"
              onClick={listState.toggleAll}
              title={listState.allCollapsed ? 'Tüm grupları aç' : 'Tüm grupları katla'}
              className="flex items-center gap-1 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              {listState.allCollapsed ? <ChevronsUpDown size={13} /> : <ChevronsDownUp size={13} />}
            </button>
          )}
          <RefreshButton onClick={reload} />
        </SidebarHeader>
        <NewItemButton
          onClick={detail.createNew}
          label="Yeni Artifact"
          title="Yeni artifact"
          testId="artifacts-create-new"
        />

        <ArtifactListFilters
          query={listState.query}
          setQuery={listState.setQuery}
          originFilter={listState.originFilter}
          setOriginFilter={listState.setOriginFilter}
          showArchived={listState.showArchived}
          setShowArchived={listState.setShowArchived}
          archivedCount={listState.archivedCount}
        />

        <ArtifactGroupedList
          loading={listState.loading}
          list={list}
          hasActiveFilters={listState.hasActiveFilters}
          grouped={listState.grouped}
          collapsed={listState.collapsed}
          toggleGroup={listState.toggleGroup}
          activeId={activeId}
          sel={sel}
          orderedIds={orderedIds}
          dnd={bulk.dnd}
          onSelect={detail.selectArtifact}
          hasMore={listState.hasMore}
          loadingMore={listState.loadingMore}
          total={listState.total}
          onLoadMore={listState.loadMore}
        />

        <ArtifactBulkBar
          sel={sel}
          orderedIds={orderedIds}
          groupNames={groupNames}
          showArchived={listState.showArchived}
          bulk={bulk}
        />
      </ListPane>

      {/* Viewer / Editor column: the standard title bar sits ONLY here, to the
          right of the list — like the chat header (never spans over the list). */}
      <div className="flex min-h-0 min-w-0 flex-1 flex-col">
        <ArtifactDetailHeader
          listOpen={listOpen}
          onToggleList={toggleList}
          active={active}
          editing={!!draft}
          saving={detail.saving}
          activePath={detail.activePath}
          agents={agents}
          onOpenSession={onOpenSession}
          onError={onError}
          onImageUpdated={detail.onImageUpdated}
          onStartEdit={detail.startEdit}
          onCopy={detail.copy}
          onSave={detail.save}
          onCancelEdit={() => setDraft(null)}
          onSetArchived={detail.setArchived}
          onDelete={detail.remove}
        />
        {!active ? (
          <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
            Görüntülemek için bir artifact seç.
          </div>
        ) : draft ? (
          <ArtifactEditor
            active={active}
            draft={draft}
            setDraft={setDraft}
            groupNames={groupNames}
          />
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
      </div>
    </div>
  )
}

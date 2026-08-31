import { ChevronDown, ChevronRight, FileCode, FileText } from 'lucide-react'
import type { Artifact } from '@/types'
import type { MultiSelect } from '@/shared/hooks/useMultiSelect'
import type { GroupDnD } from '@/shared/hooks/useGroupDnD'
import { LoadingState } from '@/shared/components'
import { SELECTED_ITEM_CLS, SELECTED_ITEM_RING } from '@/shared/components/SidebarChrome'
import { relativeTime } from '@/shared/lib/time'
import { KIND_ICON, KIND_LABEL } from './artifactMeta'
import { OriginBadge } from './OriginBadge'

interface Props {
  loading: boolean
  // Loaded rows (already server-side filtered) — drives the empty states.
  list: Artifact[]
  hasActiveFilters: boolean
  grouped: Array<[string, Artifact[]]>
  collapsed: Set<string>
  toggleGroup: (name: string) => void
  activeId: string | null
  sel: MultiSelect
  orderedIds: string[]
  dnd: GroupDnD<Artifact>
  onSelect: (id: string) => void
  hasMore: boolean
  loadingMore: boolean
  total: number
  onLoadMore: () => void
}

// ArtifactGroupedList renders the list column body: the loading/empty states,
// the collapsible group buckets with their drop targets, the artifact rows and
// the "load more" pager. Presentational — all state comes in as props.
export function ArtifactGroupedList({
  loading,
  list,
  hasActiveFilters,
  grouped,
  collapsed,
  toggleGroup,
  activeId,
  sel,
  orderedIds,
  dnd,
  onSelect,
  hasMore,
  loadingMore,
  total,
  onLoadMore,
}: Props) {
  return (
    <div className="min-h-0 flex-1 overflow-y-auto p-2">
      {loading && <LoadingState label="Artifact'ler yükleniyor…" />}
      {!loading &&
        list.length === 0 &&
        (hasActiveFilters ? (
          <div className="px-4 py-8 text-center text-sm text-[var(--color-text-dim)]">
            Filtreyle eşleşen artifact yok.
          </div>
        ) : (
          <div className="flex flex-col items-center gap-2 px-4 py-10 text-center text-sm text-[var(--color-text-dim)]">
            <FileCode size={28} className="opacity-40" />
            <p>
              Henüz artifact yok. Bir oturumda dosya/doküman ürettiğinde otomatik buraya düşer;{' '}
              <strong>Yeni</strong> ile elle ekle; ya da{' '}
              <strong>resim/video/ses dosyalarını buraya sürükle-bırak</strong>.
            </p>
          </div>
        ))}
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
                        onSelect(a.id)
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
                          <span>
                            {KIND_LABEL[a.kind] ?? a.kind} · {relativeTime(a.updatedAt)}
                          </span>
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
      {hasMore && (
        <div className="px-1 pb-1 pt-2">
          <button
            data-testid="artifacts-load-more"
            onClick={onLoadMore}
            disabled={loadingMore}
            className="w-full rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] py-1.5 text-xs font-medium text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
          >
            {loadingMore ? 'Yükleniyor…' : `Daha fazla yükle (${list.length}/${total})`}
          </button>
        </div>
      )}
    </div>
  )
}

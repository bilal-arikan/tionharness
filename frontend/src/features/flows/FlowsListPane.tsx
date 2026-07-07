import type { Dispatch, SetStateAction } from 'react'
import { Play, Trash2 } from 'lucide-react'
import { normalizeAvatar } from '@/shared/lib/avatar'
import { FLOW_TEMPLATES } from './flowTemplates'
import { flowNodeCount, type FlowsTab } from './flowsPanelShared'
import type { Flow, FlowRun } from '@/types'
import { SelectionBar, SelectionBarButton, ListPane } from '@/shared/components'
import {
  NewItemButton, SELECTED_ITEM_CLS, SELECTED_ITEM_RING,
} from '@/shared/components/SidebarChrome'
import type { MultiSelect } from '@/shared/hooks/useMultiSelect'

interface Props {
  flowsListOpen: boolean
  toggleFlowsList: () => void
  tab: FlowsTab
  setTab: Dispatch<SetStateAction<FlowsTab>>
  q: string
  setQ: Dispatch<SetStateAction<string>>
  flows: Flow[]
  runs: FlowRun[]
  templateId: string | null
  setTemplateId: Dispatch<SetStateAction<string | null>>
  selectedId: string | null
  selectedRunId: string | null
  setSelectedRunId: Dispatch<SetStateAction<string | null>>
  allTags: string[]
  tagFilter: string[]
  toggleTagFilter: (t: string) => void
  setTagFilter: Dispatch<SetStateAction<string[]>>
  sel: MultiSelect
  selectFlow: (f: Flow) => void
  createFlow: () => void
  removeFlow: (f: Flow) => void
  bulkRun: () => void
  bulkDelete: () => void
}

// FlowsListPane is the left column of the flows screen: the tab switch
// (Akışlarım / Şablonlar / Koşular), the per-tab search, and the active tab's
// list (own flows with tag filter + multi-select, template gallery, or run
// history). All state lives in FlowsPanel; this renders it.
export function FlowsListPane({
  flowsListOpen,
  toggleFlowsList,
  tab,
  setTab,
  q,
  setQ,
  flows,
  runs,
  templateId,
  setTemplateId,
  selectedId,
  selectedRunId,
  setSelectedRunId,
  allTags,
  tagFilter,
  toggleTagFilter,
  setTagFilter,
  sel,
  selectFlow,
  createFlow,
  removeFlow,
  bulkRun,
  bulkDelete,
}: Props) {
  return (
    <ListPane
      open={flowsListOpen}
      onToggle={toggleFlowsList}
      widthKey="tionswarm.flowsListWidth"
      defaultWidth={224}
      minWidth={180}
      label="Akışlar"
      testId="flows-list-toggle"
      hideRail
    >
      <div className="min-h-0 flex-1 overflow-y-auto p-3">
      {/* Tab switch */}
      <div className="mb-3 flex gap-1 rounded-lg bg-[var(--color-surface-2)] p-1 text-xs">
        {(['flows', 'templates', 'runs'] as const).map((t) => (
          <button
            key={t}
            onClick={() => setTab(t)}
            className={`flex-1 rounded-md px-1.5 py-1 ${
              tab === t ? 'bg-[var(--color-accent)] text-white' : 'text-[var(--color-text-dim)]'
            }`}
          >
            {t === 'flows' ? 'Akışlarım' : t === 'templates' ? 'Şablonlar' : 'Koşular'}
          </button>
        ))}
      </div>

      {/* Search (filters the active tab's list). */}
      <input
        value={q}
        onChange={(e) => setQ(e.target.value)}
        placeholder={tab === 'runs' ? 'Koşu ara (akış adı)…' : tab === 'templates' ? 'Şablon ara…' : 'Akış ara…'}
        className="mb-2 w-full rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 text-xs outline-none focus:border-[var(--color-accent)]"
      />

      {tab === 'templates' ? (
        <ul className="space-y-1">
          {FLOW_TEMPLATES.filter((t) => t.name.toLowerCase().includes(q.trim().toLowerCase())).map((t) => (
            <li key={t.id}>
              <button
                onClick={() => setTemplateId(t.id)}
                className={`w-full rounded-lg px-3 py-2 text-left text-sm ${
                  templateId === t.id
                    ? SELECTED_ITEM_CLS
                    : 'hover:bg-[var(--color-surface-2)]'
                }`}
              >
                <span className="block truncate">{t.name}</span>
                <span className="mt-0.5 block truncate text-xs text-[var(--color-text-dim)]">
                  {t.description}
                </span>
              </button>
            </li>
          ))}
        </ul>
      ) : tab === 'runs' ? (
        <ul className="space-y-1">
          {runs
            .filter((rn) => {
              const qq = q.trim().toLowerCase()
              if (!qq) return true
              return (flows.find((f) => f.id === rn.flowId)?.name ?? '').toLowerCase().includes(qq)
            })
            .map((rn) => {
            const rflow = flows.find((f) => f.id === rn.flowId)
            const fname = rflow?.name ?? '（silinmiş akış）'
            const femoji = normalizeAvatar(rflow?.emoji)
            const badge =
              rn.status === 'success' ? '✓' : rn.status === 'failure' ? '✕' : '▶'
            const badgeColor =
              rn.status === 'success'
                ? 'text-[var(--color-success)]'
                : rn.status === 'failure'
                  ? 'text-[var(--color-danger)]'
                  : 'text-[var(--color-accent)]'
            return (
              <li key={rn.id}>
                <button
                  onClick={() => setSelectedRunId(rn.id)}
                  className={`flex w-full items-start gap-2 rounded-lg px-3 py-2 text-left text-sm ${
                    selectedRunId === rn.id
                      ? SELECTED_ITEM_CLS
                      : 'hover:bg-[var(--color-surface-2)]'
                  }`}
                >
                  <span className={`mt-0.5 text-xs ${badgeColor}`}>{badge}</span>
                  <span className="min-w-0 flex-1">
                    <span className="block truncate">
                      {femoji && <span className="mr-1 leading-none">{femoji}</span>}
                      {fname}
                    </span>
                    <span className="mt-0.5 block truncate text-xs text-[var(--color-text-dim)]">
                      {new Date(rn.createdAt * 1000).toLocaleString()}
                    </span>
                  </span>
                </button>
              </li>
            )
          })}
          {runs.length === 0 && (
            <li className="text-sm text-[var(--color-text-dim)]">Henüz koşu yok.</li>
          )}
        </ul>
      ) : (
        <>
      <NewItemButton bare onClick={createFlow} label="Yeni akış" className="mb-3" />
      {/* Tag filter chips: click to narrow the list to flows carrying any of the
          selected tags. Only shown when at least one flow has a tag. */}
      {allTags.length > 0 && (
        <div className="mb-2 flex flex-wrap items-center gap-1">
          {allTags.map((t) => {
            const on = tagFilter.includes(t)
            return (
              <button
                key={t}
                onClick={() => toggleTagFilter(t)}
                className={`rounded-full px-1.5 py-0.5 text-[10px] transition ${
                  on
                    ? 'bg-[var(--color-accent)] text-white'
                    : 'bg-[var(--color-accent-soft)] text-[var(--color-accent)] hover:opacity-80'
                }`}
                title={on ? 'Filtreyi kaldır' : 'Bu etikete göre filtrele'}
              >
                #{t}
              </button>
            )
          })}
          {tagFilter.length > 0 && (
            <button
              onClick={() => setTagFilter([])}
              className="text-[10px] text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
              title="Etiket filtresini temizle"
            >
              temizle
            </button>
          )}
        </div>
      )}
      {(() => {
        const visible = flows.filter(
          (f) =>
            f.name.toLowerCase().includes(q.trim().toLowerCase()) &&
            (tagFilter.length === 0 || tagFilter.some((t) => f.tags?.includes(t))),
        )
        const orderedIds = visible.map((f) => f.id)
        return (
      <ul className="space-y-1">
        {visible.map((f) => (
          <li key={f.id}>
            <button
              onClick={(e) => {
                if (sel.handleClick(e, f.id, orderedIds, selectedId)) return
                selectFlow(f)
              }}
              className={`flex w-full items-start justify-between rounded-lg px-3 py-2 text-left text-sm ${
                sel.isSelected(f.id)
                  ? `${SELECTED_ITEM_CLS} ${SELECTED_ITEM_RING}`
                  : selectedId === f.id
                    ? SELECTED_ITEM_CLS
                    : 'hover:bg-[var(--color-surface-2)]'
              }`}
            >
              <span className="min-w-0 flex-1">
                <span className="flex items-center gap-1.5">
                  {normalizeAvatar(f.emoji) && (
                    <span className="shrink-0 leading-none">{normalizeAvatar(f.emoji)}</span>
                  )}
                  <span className="truncate">{f.name}</span>
                </span>
                <span className="mt-0.5 flex items-center gap-1.5 text-xs text-[var(--color-text-dim)]">
                  <span className="truncate font-mono text-[11px]">{f.id}</span>
                  <span className="flex-shrink-0">· {flowNodeCount(f.graph)} node</span>
                </span>
                {(f.tags?.length ?? 0) > 0 && (
                  <span className="mt-1 flex flex-wrap gap-1">
                    {f.tags!.slice(0, 4).map((t) => (
                      <span
                        key={t}
                        className="rounded-full bg-[var(--color-accent-soft)] px-1.5 py-0.5 text-[10px] text-[var(--color-accent)]"
                      >
                        #{t}
                      </span>
                    ))}
                    {f.tags!.length > 4 && (
                      <span className="text-[10px] text-[var(--color-text-dim)]">+{f.tags!.length - 4}</span>
                    )}
                  </span>
                )}
              </span>
              <span
                onClick={(e) => {
                  // Let modifier-clicks bubble up to the selection handler.
                  if (e.ctrlKey || e.metaKey || e.shiftKey) return
                  e.stopPropagation()
                  removeFlow(f)
                }}
                className="ml-2 text-xs text-[var(--color-text-dim)] hover:text-[var(--color-danger)]"
              >
                ✕
              </span>
            </button>
          </li>
        ))}
        {visible.length === 0 && (
          <li className="text-sm text-[var(--color-text-dim)]">
            {flows.length === 0 ? 'Henüz akış yok.' : 'Eşleşen akış yok.'}
          </li>
        )}
      </ul>
        )
      })()}
      <SelectionBar
        count={sel.count}
        onClear={sel.clear}
      >
        <SelectionBarButton icon={<Play size={13} />} onClick={bulkRun}>
          Çalıştır
        </SelectionBarButton>
        <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulkDelete} danger>
          Sil
        </SelectionBarButton>
      </SelectionBar>
        </>
      )}
      </div>
    </ListPane>
  )
}

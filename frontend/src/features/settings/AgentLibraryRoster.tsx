import { Search, Layers, Bot, Workflow, Users, CornerDownRight } from 'lucide-react'
import type { CatalogAgent, CatalogWorkspace } from '@/api/agentCatalog'
import type { Agent } from '@/types'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { useCatalog, resolveModelLabel } from '@/shared/lib/catalog'
import type { MultiSelect } from '@/shared/hooks/useMultiSelect'

import type { AgentKind } from './agentLibrary'
interface Props {
  rows: CatalogAgent[]
  byId: ReadonlyMap<string, Agent>
  workspaces: CatalogWorkspace[]
  selectedId: string | null
  onSelect: (id: string) => void
  query: string
  setQuery: (query: string) => void
  kind: AgentKind
  setKind: (kind: AgentKind) => void
  workspace: string
  setWorkspace: (id: string) => void
  selection: MultiSelect
}
export function AgentLibraryRoster({
  rows,
  byId,
  workspaces,
  selectedId,
  onSelect,
  query,
  setQuery,
  kind,
  setKind,
  workspace,
  setWorkspace,
  selection,
}: Props) {
  const catalog = useCatalog()
  const ids = rows.map((row) => row.agent.id)
  return (
    <>
      <div className="space-y-2 border-b border-[var(--color-border)] p-3">
        <label className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-[var(--color-text-dim)]">
          <Search size={15} />
          <input
            aria-label="Search agents"
            placeholder="Search name, model or workspace…"
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="min-w-0 flex-1 bg-transparent text-xs outline-none"
          />
        </label>
        <div className="grid grid-cols-4 gap-1 rounded-lg bg-[var(--color-surface-2)] p-1">
          {(
            [
              ['all', 'All', Layers],
              ['custom', 'Custom', Bot],
              ['services', 'Services', Workflow],
              ['workers', 'Workers', Users],
            ] as const
          ).map(([value, label, Icon]) => (
            <button
              key={value}
              onClick={() => setKind(value)}
              aria-pressed={kind === value}
              className={`flex items-center justify-center gap-1 rounded-md px-1 py-1.5 text-[10px] transition ${kind === value ? 'bg-[var(--color-surface)] text-[var(--color-accent)] shadow-sm' : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'}`}
            >
              <Icon size={12} className="shrink-0" />
              {label}
            </button>
          ))}
        </div>
        <select
          aria-label="Filter by workspace"
          value={workspace}
          onChange={(event) => setWorkspace(event.target.value)}
          className="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-xs"
        >
          <option value="">All workspaces</option>
          <option value="unassigned">Unassigned</option>
          {workspaces.map((entry) => (
            <option key={entry.id} value={entry.id}>
              {entry.name}
            </option>
          ))}
        </select>
      </div>
      <div className="flex-1 space-y-1 overflow-y-auto p-2">
        {rows.map(({ agent, assignments }) => {
          const active = selectedId === agent.id || selection.isSelected(agent.id)
          const parentName = agent.parentId
            ? (byId.get(agent.parentId)?.name ?? agent.parentId)
            : null
          return (
            <button
              key={agent.id}
              data-testid="system-agent-roster-item"
              data-agent-id={agent.id}
              title={[agent.name, ...assignments.map((entry) => entry.workspaceName)].join(' · ')}
              aria-pressed={active}
              onClick={(event) => {
                if (!selection.handleClick(event, agent.id, ids, selectedId)) onSelect(agent.id)
              }}
              className={`w-full rounded-lg border px-2.5 py-2 text-left transition ${active ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]' : 'border-transparent hover:bg-[var(--color-surface-2)]'} ${agent.disabled ? 'opacity-60' : ''}`}
            >
              <AgentIdentity
                agent={agent}
                size="sm"
                subtitle={[
                  resolveModelLabel(catalog, agent.provider, agent.model),
                  assignments.length === 0
                    ? 'Unassigned'
                    : `${assignments.length} workspace${assignments.length === 1 ? '' : 's'}`,
                ]
                  .filter(Boolean)
                  .join(' · ')}
                trailing={
                  <span className="flex shrink-0 flex-col items-end gap-0.5 text-[10px] text-[var(--color-text-dim)]">
                    <span className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5">
                      {agent.locked ? 'Built-in' : agent.system ? 'Role' : 'Custom'}
                    </span>
                    {agent.disabled && <span>Disabled</span>}
                  </span>
                }
              />
              {parentName && (
                <span
                  className="ml-7 mt-0.5 flex min-w-0 items-center gap-1 text-[10px] text-[var(--color-text-dim)]"
                  title={`Inherits from ${parentName}`}
                  aria-label={`Inherits from ${parentName}`}
                >
                  <CornerDownRight size={11} className="shrink-0 text-[var(--color-accent)]" />
                  <span className="truncate">{parentName}</span>
                </span>
              )}
            </button>
          )
        })}
        {rows.length === 0 && (
          <div className="rounded-xl border border-dashed border-[var(--color-border)] p-6 text-center text-xs text-[var(--color-text-dim)]">
            No agents match these filters.
          </div>
        )}
      </div>
    </>
  )
}

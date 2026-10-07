import { Search, Layers, Bot, Workflow, Users, CornerDownRight, Boxes } from 'lucide-react'
import { useTranslation } from 'react-i18next'
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
  const { t } = useTranslation('settingsMain')
  const catalog = useCatalog()
  const ids = rows.map((row) => row.agent.id)
  return (
    <>
      <div className="space-y-1.5 border-b border-[var(--color-border)] p-2">
        <label className="flex items-center gap-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-[var(--color-text-dim)]">
          <Search size={15} />
          <input
            aria-label={t('agentRoster.searchLabel')}
            placeholder={t('agentRoster.searchPlaceholder')}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            className="min-w-0 flex-1 bg-transparent text-xs outline-none"
          />
        </label>
        <div className="flex gap-1 rounded-lg bg-[var(--color-surface-2)] p-1">
          {(
            [
              ['all', t('agentRoster.kinds.all'), Layers],
              ['custom', t('agentRoster.kinds.custom'), Bot],
              ['services', t('agentRoster.kinds.services'), Workflow],
              ['workers', t('agentRoster.kinds.workers'), Users],
            ] as const
          ).map(([value, label, Icon]) => (
            <button
              key={value}
              onClick={() => setKind(value)}
              aria-pressed={kind === value}
              title={label}
              className={`flex min-w-0 flex-auto items-center justify-center gap-0.5 rounded-md px-0.5 py-1 text-[10px] transition ${kind === value ? 'bg-[var(--color-surface)] text-[var(--color-accent)] shadow-sm' : 'text-[var(--color-text-dim)] hover:text-[var(--color-text)]'}`}
            >
              <Icon size={11} className="shrink-0" />
              <span className="truncate">{label}</span>
            </button>
          ))}
        </div>
        <select
          aria-label={t('agentRoster.workspaceFilter')}
          value={workspace}
          onChange={(event) => setWorkspace(event.target.value)}
          className="w-full rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1.5 text-xs"
        >
          <option value="">{t('agentRoster.allWorkspaces')}</option>
          <option value="unassigned">{t('agentRoster.unassigned')}</option>
          {workspaces.map((entry) => (
            <option key={entry.id} value={entry.id}>
              {entry.name}
            </option>
          ))}
        </select>
      </div>
      <div className="flex-1 space-y-0.5 overflow-y-auto p-1.5">
        {rows.map(({ agent, assignments }) => {
          const active = selectedId === agent.id || selection.isSelected(agent.id)
          const workspaceLabel =
            assignments.length === 0
              ? t('agentRoster.unassigned')
              : t('agentRoster.workspaceCount', { count: assignments.length })
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
              className={`w-full rounded-md border px-2 py-1 text-left transition ${active ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]' : 'border-transparent hover:bg-[var(--color-surface-2)]'} ${agent.disabled ? 'opacity-60' : ''}`}
            >
              <AgentIdentity
                agent={agent}
                size="sm"
                subtitle={resolveModelLabel(catalog, agent.provider, agent.model)}
                trailing={
                  <span className="flex shrink-0 items-center gap-1 text-[10px] text-[var(--color-text-dim)]">
                    {/* Workspace count as a glyph + number; the wording lives in
                        the tooltip. Zero is tinted: an unassigned agent runs nowhere. */}
                    <span
                      title={workspaceLabel}
                      aria-label={workspaceLabel}
                      className={`flex items-center gap-0.5 tabular-nums ${assignments.length === 0 ? 'text-[var(--color-warning)]' : ''}`}
                    >
                      <Boxes size={10} className="shrink-0" />
                      {assignments.length}
                    </span>
                    <span className="rounded bg-[var(--color-surface-2)] px-1 py-px">
                      {agent.locked
                        ? t('agentRoster.badges.builtIn')
                        : agent.system
                          ? t('agentRoster.badges.role')
                          : t('agentRoster.badges.custom')}
                    </span>
                    {agent.disabled && <span>{t('agentRoster.badges.disabled')}</span>}
                  </span>
                }
              />
              {parentName && (
                <span
                  className="ml-7 flex min-w-0 items-center gap-1 text-[10px] text-[var(--color-text-dim)]"
                  title={t('agentRoster.inherits', { name: parentName })}
                  aria-label={t('agentRoster.inherits', { name: parentName })}
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
            {t('agentRoster.empty')}
          </div>
        )}
      </div>
    </>
  )
}

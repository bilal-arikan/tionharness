import { useEffect, useMemo, useState } from 'react'
import { createPortal } from 'react-dom'
import { Plus, RefreshCw, X } from 'lucide-react'
import { agentCatalogApi, catalogEditorApi } from '@/api/agentCatalog'
import type { AgentPatch } from '@/types'
import { AgentSettingsForm } from '@/features/agents/AgentSettingsForm'
import { AgentEditorContext } from '@/features/agents/AgentEditorContext'
import { indexAgents, eligibleParents, lineageOf } from '@/shared/lib/agentLineage'
import { LoadingState } from '@/shared/components'
import { useAsync } from '@/shared/hooks/useAsync'
import { useMultiSelect } from '@/shared/hooks/useMultiSelect'
import { useRefreshTrigger } from '@/shared/hooks/useRefreshTrigger'
import { SIGNAL_AGENTS } from '@/app/eventToRefreshSignals'
import { SystemAgentsBulkBar } from './SystemAgentsBulkBar'
import { AgentLibraryRoster } from './AgentLibraryRoster'
import { agentKind, type AgentKind } from './agentLibrary'
import { AgentWorkspaceAssignments } from './AgentWorkspaceAssignments'
import { CreateCatalogAgent } from './CreateCatalogAgent'
import { useTranslation } from 'react-i18next'

interface Props {
  onError: (message: string) => void
  headerTarget: HTMLElement | null
}

export function SystemAgentsPanel({ onError, headerTarget }: Props) {
  const { t } = useTranslation('settings')
  const agentsTick = useRefreshTrigger(SIGNAL_AGENTS)
  const { data, loading, error, refresh } = useAsync(agentCatalogApi.listAgentCatalog, [agentsTick])
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [query, setQuery] = useState('')
  const [kind, setKind] = useState<AgentKind>('all')
  const [workspace, setWorkspace] = useState('')
  const [pending, setPending] = useState(false)
  const [createOpen, setCreateOpen] = useState(false)
  const selection = useMultiSelect()
  useEffect(() => {
    if (error) onError(error)
  }, [error, onError])
  const entries = useMemo(() => data?.agents ?? [], [data])
  const agents = useMemo(() => entries.map((entry) => entry.agent), [entries])
  const byId = useMemo(() => indexAgents(agents), [agents])
  const visible = useMemo(
    () =>
      entries.filter((entry) => {
        if (kind !== 'all' && agentKind(entry) !== kind) return false
        if (workspace === 'unassigned' && entry.assignments.length > 0) return false
        if (
          workspace &&
          workspace !== 'unassigned' &&
          !entry.assignments.some((link) => link.workspaceId === workspace)
        )
          return false
        const text = [
          entry.agent.name,
          entry.agent.model,
          entry.agent.systemKey,
          ...entry.assignments.map((link) => link.workspaceName),
        ]
          .join(' ')
          .toLowerCase()
        return text.includes(query.trim().toLowerCase())
      }),
    [entries, kind, workspace, query],
  )
  const selected = entries.find((entry) => entry.agent.id === selectedId) ?? visible[0] ?? null
  const selectedAgent = selected?.agent
  const save = async (id: string, patch: AgentPatch) => {
    const result = await agentCatalogApi.updateCatalogAgent(id, patch)
    refresh()
    return result
  }
  const action = async (run: () => Promise<unknown>) => {
    setPending(true)
    try {
      await run()
      refresh()
    } catch (error) {
      onError((error as Error).message)
    } finally {
      setPending(false)
    }
  }
  const derive = async (opts: { bindRole: boolean }) => {
    if (!selectedAgent) return
    let id: string | undefined
    await action(async () => {
      const created = await agentCatalogApi.deriveCatalogAgent(selectedAgent.id, opts)
      id = created.id
      setSelectedId(id)
    })
    return id
  }
  if (loading && !data) return <LoadingState label={t('systemAgents.loading')} />
  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="system-agents-panel">
      {headerTarget &&
        createPortal(
          <>
            <span className="text-xs text-[var(--color-text-dim)]">
              {t('systemAgents.agentCount', { count: agents.length })}
            </span>
            <button
              aria-label={t('systemAgents.refresh')}
              data-testid="system-agents-refresh"
              onClick={refresh}
              disabled={loading}
              className="rounded-lg border border-[var(--color-border)] p-2 hover:text-[var(--color-accent)]"
            >
              <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            </button>
            <button
              onClick={() => setCreateOpen(!createOpen)}
              aria-expanded={createOpen}
              className="inline-flex items-center gap-1.5 rounded-lg bg-[var(--color-accent)] px-3 py-2 text-xs font-medium text-white"
            >
              {createOpen ? <X size={14} /> : <Plus size={14} />}
              {createOpen ? t('systemAgents.closeNew') : t('systemAgents.newAgent')}
            </button>
          </>,
          headerTarget,
        )}
      <CreateCatalogAgent
        open={createOpen}
        onClose={() => setCreateOpen(false)}
        onCreated={(id) => {
          setSelectedId(id)
          refresh()
        }}
      />
      {error && (
        <p role="alert" className="px-5 py-3 text-xs text-[var(--color-danger)]">
          {error}
        </p>
      )}
      <div className="flex min-h-0 flex-1 flex-col lg:flex-row">
        <aside className="flex max-h-80 min-h-0 flex-col border-b border-[var(--color-border)] bg-[var(--color-surface)] lg:max-h-none lg:w-72 lg:shrink-0 lg:border-b-0 lg:border-r">
          <AgentLibraryRoster
            rows={visible}
            byId={byId}
            workspaces={data?.workspaces ?? []}
            selectedId={selectedAgent?.id ?? null}
            onSelect={setSelectedId}
            query={query}
            setQuery={setQuery}
            kind={kind}
            setKind={setKind}
            workspace={workspace}
            setWorkspace={setWorkspace}
            selection={selection}
          />
          <SystemAgentsBulkBar
            sel={selection}
            agents={visible.map((entry) => entry.agent)}
            orderedIds={visible.map((entry) => entry.agent.id)}
            onSettled={refresh}
            onError={onError}
            onUpdateAgent={agentCatalogApi.updateCatalogAgent}
          />
        </aside>
        <div className="min-h-0 min-w-0 flex-1 overflow-y-auto">
          {selected && selectedAgent ? (
            <>
              <AgentWorkspaceAssignments
                key={selectedAgent.id}
                entry={selected}
                workspaces={data?.workspaces ?? []}
                onChanged={refresh}
                onError={onError}
              />
              <AgentEditorContext value={catalogEditorApi}>
                <AgentSettingsForm
                  key={selectedAgent.id}
                  agent={selectedAgent}
                  onSave={(patch) => save(selectedAgent.id, patch)}
                  onDerive={derive}
                  allowFreeDerive={!selectedAgent.system}
                  lineage={lineageOf(selectedAgent, byId)}
                  parent={
                    selectedAgent.parentId ? (byId.get(selectedAgent.parentId) ?? null) : null
                  }
                  parentOptions={eligibleParents(selectedAgent, agents)}
                  onSelectAgent={setSelectedId}
                  systemActionPending={pending}
                  onDelete={
                    selectedAgent.locked
                      ? undefined
                      : () => {
                          if (
                            confirm(t('systemAgents.deleteConfirm', { name: selectedAgent.name }))
                          )
                            void action(async () => {
                              await agentCatalogApi.deleteCatalogAgent(selectedAgent.id)
                              setSelectedId(null)
                            })
                        }
                  }
                  onRestoreDefault={
                    selectedAgent.locked || selectedAgent.parentId
                      ? () => {
                          if (
                            confirm(t('systemAgents.restoreConfirm', { name: selectedAgent.name }))
                          )
                            void action(() => agentCatalogApi.restoreCatalogAgent(selectedAgent.id))
                        }
                      : undefined
                  }
                  onToggleDisabled={
                    selectedAgent.locked
                      ? undefined
                      : () =>
                          void action(() =>
                            agentCatalogApi.updateCatalogAgent(selectedAgent.id, {
                              disabled: !selectedAgent.disabled,
                            }),
                          )
                  }
                  onDuplicate={
                    selectedAgent.locked
                      ? undefined
                      : async () => {
                          const created = await agentCatalogApi.duplicateCatalogAgent(
                            selectedAgent.id,
                          )
                          refresh()
                          setSelectedId(created.id)
                          return created.id
                        }
                  }
                />
              </AgentEditorContext>
            </>
          ) : (
            <div className="p-10 text-center text-sm text-[var(--color-text-dim)]">
              {t('systemAgents.selectAgent')}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Check, Info, Layers, Plus, X } from 'lucide-react'
import { agentCatalogApi, type CatalogAgent, type CatalogWorkspace } from '@/api/agentCatalog'

interface Props {
  entry: CatalogAgent
  workspaces: CatalogWorkspace[]
  onChanged: () => void
  onError: (message: string) => void
}
export function AgentWorkspaceAssignments({ entry, workspaces, onChanged, onError }: Props) {
  const { t } = useTranslation('settingsMain')
  const [busy, setBusy] = useState<string | null>(null)
  const assigned = new Map(entry.assignments.map((link) => [link.workspaceId, link]))
  const toggle = async (workspace: CatalogWorkspace) => {
    const removing = assigned.has(workspace.id)
    if (
      removing &&
      !confirm(
        t('assignments.removeConfirm', { agent: entry.agent.name, workspace: workspace.name }),
      )
    )
      return
    setBusy(workspace.id)
    try {
      if (removing) await agentCatalogApi.detachCatalogAgent(entry.agent.id, workspace.id)
      else await agentCatalogApi.assignCatalogAgent(entry.agent.id, workspace.id)
      onChanged()
    } catch (error) {
      onError((error as Error).message)
    } finally {
      setBusy(null)
    }
  }
  return (
    <section
      className="flex flex-wrap items-center gap-x-3 gap-y-2 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2.5"
      aria-label={t('assignments.label')}
    >
      <div className="flex items-center gap-1.5 text-xs font-medium">
        <Layers size={13} className="text-[var(--color-accent)]" />
        {t('assignments.label')}
        <span className="text-[10px] font-normal text-[var(--color-text-dim)]">
          ({entry.assignments.length})
        </span>
        <span
          tabIndex={0}
          role="note"
          aria-label={entry.agent.locked ? t('assignments.builtInHint') : t('assignments.syncHint')}
          title={entry.agent.locked ? t('assignments.builtInHint') : t('assignments.syncHint')}
          className="text-[var(--color-text-dim)]"
        >
          <Info size={12} />
        </span>
      </div>
      <div className="flex min-w-0 flex-wrap gap-1.5">
        {workspaces.map((workspace) => {
          const link = assigned.get(workspace.id)
          return (
            <button
              key={workspace.id}
              disabled={!!busy || workspace.degraded || !!entry.agent.locked}
              aria-label={t(link ? 'assignments.removeFrom' : 'assignments.assignTo', {
                workspace: workspace.name,
              })}
              aria-pressed={!!link}
              onClick={() => void toggle(workspace)}
              title={workspace.degradedReason || workspace.name}
              className={`group inline-flex max-w-full items-center gap-1 rounded-md border px-2 py-1 text-[11px] transition disabled:cursor-default ${link ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-accent)]' : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-text)]'} ${workspace.degraded ? 'opacity-50' : ''}`}
            >
              {link ? <Check size={12} /> : <Plus size={12} />}
              <span className="max-w-40 truncate">{workspace.name}</span>
              {link?.archived && <span>({t('assignments.archived')})</span>}
              {workspace.degraded && <span>({t('assignments.unavailable')})</span>}
              {busy === workspace.id ? '…' : link && !entry.agent.locked ? <X size={11} /> : null}
            </button>
          )
        })}
        {workspaces.length === 0 && (
          <span className="text-xs text-[var(--color-text-dim)]">{t('assignments.empty')}</span>
        )}
      </div>
    </section>
  )
}

import { useEffect, useMemo, useState } from 'react'
import { Layers, RefreshCw } from 'lucide-react'
import { api } from '@/api'
import type { Agent, AgentPatch } from '@/types'
import { AgentIdentity } from '@/shared/components/agents/AgentIdentity'
import { AgentSettingsForm } from '@/features/agents/AgentSettingsForm'
import { SystemAgentStatusBadge } from '@/features/agents/SystemAgentStatusBadge'
import { groupSystemAgents } from '@/features/agents/agentRoster'
import { indexAgents, eligibleParents, lineageOf } from '@/shared/lib/agentLineage'
import { useCatalog, resolveModelLabel } from '@/shared/lib/catalog'
import { LoadingState } from '@/shared/components'
import { useAsync } from '@/shared/hooks/useAsync'

interface Props {
  /** Surface load/save failures on the app banner. */
  onError: (msg: string) => void
}

const sectionHeading = (label: string, hint: string) => (
  <h3
    className="mb-1 mt-4 px-2.5 text-[11px] font-semibold uppercase tracking-wide text-[var(--color-text-dim)] first:mt-0"
    title={hint}
  >
    {label}
  </h3>
)

// SystemAgentsPanel is the Settings-screen editor for the agents the app runs
// for ITSELF — the service agents (titler, compaction, insight, …) and the
// worker profiles spawn_worker/run_subagent pick from. It mirrors the Agents
// screen's two-pane shape (roster left, settings right) but is scoped to system
// agents only, so the app's own behaviour can be tuned without leaving Settings.
//
// A LOCKED built-in renders read-only; "Özelleştir" derives a customisation
// bound to the same system role, and THAT row is what serves the role while it
// stays enabled (see SystemAgentStatusBadge).
export function SystemAgentsPanel({ onError }: Props) {
  const { data, loading, error, refresh } = useAsync(() => api.listAgents(), [])
  const agents = useMemo(() => data ?? [], [data])
  const catalog = useCatalog()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [actionPending, setActionPending] = useState(false)

  useEffect(() => {
    if (error) onError(error)
  }, [error, onError])

  const groups = useMemo(() => groupSystemAgents(agents), [agents])
  const systemAgents = useMemo(() => [...groups.services, ...groups.workers], [groups])
  const byId = useMemo(() => indexAgents(agents), [agents])

  // Keep a valid selection: the current one if it survived a refresh, else the
  // first system agent in roster order.
  useEffect(() => {
    if (selectedId && systemAgents.some((a) => a.id === selectedId)) return
    setSelectedId(systemAgents[0]?.id ?? null)
  }, [systemAgents, selectedId])

  const selected = systemAgents.find((a) => a.id === selectedId) ?? null

  // `refresh` from useAsync is fire-and-forget: it re-runs the fetch and lands
  // the result through state, so the spinner is driven by the hook's own
  // `loading` flag rather than an awaited promise.
  const doRefresh = () => refresh()

  const save = async (id: string, patch: AgentPatch) => {
    const res = await api.updateAgent(id, patch)
    refresh()
    return res
  }

  const derive = async (id: string, opts: { bindRole: boolean }) => {
    setActionPending(true)
    try {
      const created = await api.deriveAgent(id, opts)
      refresh()
      setSelectedId(created.id)
      return created.id
    } catch (e) {
      onError((e as Error).message)
      return undefined
    } finally {
      setActionPending(false)
    }
  }

  const toggleDisabled = async (agent: Agent) => {
    setActionPending(true)
    try {
      await api.updateAgent(agent.id, { disabled: !agent.disabled })
      refresh()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setActionPending(false)
    }
  }

  const restoreDefault = async (agent: Agent) => {
    const question = agent.locked
      ? `"${agent.name}" ajanının tüm özelleştirmeleri kaldırılsın mı? Her alan yeniden yerleşik tanımdan gelir ve bu TÜM workspaceʼleri etkiler; bu işlem geri alınamaz.`
      : `"${agent.name}" ajanının tüm override'ları kaldırılsın mı? Her alan yeniden ebeveyninden devralınır; bu işlem geri alınamaz.`
    if (!confirm(question)) return
    setActionPending(true)
    try {
      await api.restoreDefaultAgent(agent.id)
      refresh()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setActionPending(false)
    }
  }

  const remove = async (agent: Agent) => {
    if (
      !confirm(
        `"${agent.name}" özelleştirmesi silinsin mi?\n\nSilinince "${agent.systemKey}" rolünü yeniden yerleşik tanım sağlar. Bu işlem geri alınamaz.`,
      )
    )
      return
    setActionPending(true)
    try {
      await api.deleteAgent(agent.id)
      setSelectedId(null)
      refresh()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setActionPending(false)
    }
  }

  const rosterItem = (a: Agent) => (
    <button
      key={a.id}
      onClick={() => setSelectedId(a.id)}
      data-testid="system-agent-roster-item"
      data-agent-id={a.id}
      className={`mb-1 flex w-full items-stretch gap-2 rounded-lg py-1 pl-2 pr-1 text-left text-sm transition ${
        a.disabled ? 'opacity-50' : ''
      } ${
        selectedId === a.id
          ? 'bg-[var(--color-surface-2)] text-[var(--color-text)]'
          : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)]'
      }`}
    >
      <span className="flex min-w-0 flex-1 items-center py-1">
        <AgentIdentity
          agent={a}
          size="md"
          nameSuffix={<SystemAgentStatusBadge agent={a} />}
          subtitle={resolveModelLabel(catalog, a.provider, a.model)}
        />
      </span>
    </button>
  )

  if (loading && agents.length === 0) return <LoadingState label="Yükleniyor…" />

  return (
    <div className="flex min-h-0 flex-1 flex-col" data-testid="system-agents-panel">
      <div
        data-testid="system-agents-scope-note"
        className="flex items-start gap-2 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-4 py-2 text-xs text-[var(--color-text-dim)]"
      >
        <Layers size={14} className="mt-0.5 shrink-0" />
        <p>
          Yerleşik sistem ajanları burada <strong>doğrudan</strong> düzenlenir — kopya oluşmaz.
          Değişiklikler <strong>tüm workspaceʼlerde</strong> geçerlidir; dokunmadığın alanlar
          yerleşik tanımı izlemeye devam eder, böylece uygulama güncellendiğinde onlar da
          güncellenir.
        </p>
      </div>
      <div className="flex min-h-0 flex-1">
        <aside className="flex w-64 flex-shrink-0 flex-col border-r border-[var(--color-border)] bg-[var(--color-surface)]">
          <div className="flex items-center justify-between border-b border-[var(--color-border)] px-3 py-2">
            <span className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
              Sistem ajanları
            </span>
            <button
              onClick={doRefresh}
              disabled={loading}
              data-testid="system-agents-refresh"
              title="Yenile"
              className="rounded p-1 text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)] disabled:opacity-50"
            >
              <RefreshCw size={14} className={loading ? 'animate-spin' : ''} />
            </button>
          </div>
          <div className="flex-1 overflow-y-auto px-2 py-2">
            {groups.services.length > 0 && (
              <>
                {sectionHeading(
                  'Servisler',
                  'Uygulamanın kendi işleri (başlık, sıkıştırma, insight) için kullandığı yerleşik ajanlar ve onların workspace özelleştirmeleri',
                )}
                {groups.services.map(rosterItem)}
              </>
            )}
            {groups.workers.length > 0 && (
              <>
                {sectionHeading(
                  "Worker'lar",
                  "spawn_worker ve run_subagent'ın seçtiği yerleşik worker profilleri (explore, planner, coder, …) ve onların workspace özelleştirmeleri",
                )}
                {groups.workers.map(rosterItem)}
              </>
            )}
            {systemAgents.length === 0 && (
              <p className="px-3 py-2 text-xs text-[var(--color-text-dim)]">
                Bu workspace'te sistem ajanı bulunamadı.
              </p>
            )}
          </div>
        </aside>

        <div className="min-w-0 flex-1 overflow-y-auto">
          {selected ? (
            <AgentSettingsForm
              key={selected.id}
              agent={selected}
              onSave={(p) => save(selected.id, p)}
              onDerive={(opts) => derive(selected.id, opts)}
              // A system agent's only sanctioned derivation is "Özelleştir",
              // which binds the copy to the built-in's role. A free-standing
              // child would serve no role and only clutter the roster.
              allowFreeDerive={false}
              lineage={lineageOf(selected, byId)}
              parent={selected.parentId ? (byId.get(selected.parentId) ?? null) : null}
              parentOptions={eligibleParents(selected, agents)}
              onSelectAgent={setSelectedId}
              onDelete={selected.locked ? undefined : () => remove(selected)}
              onRestoreDefault={
                selected.parentId || selected.locked ? () => restoreDefault(selected) : undefined
              }
              onToggleDisabled={selected.locked ? undefined : () => toggleDisabled(selected)}
              systemActionPending={actionPending}
            />
          ) : (
            <div className="flex h-full items-center justify-center text-sm text-[var(--color-text-dim)]">
              Düzenlemek için soldan bir sistem ajanı seç.
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

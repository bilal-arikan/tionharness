// Dedicated "Dışa Aktar" (export-as-template) sub-panel for a workspace. Split
// out of the General settings panel so the export can be detailed: the user
// picks exactly which agents and which optional categories (flows, schedules,
// workspace skills, instructions, board columns) get captured into the template
// pack. Secrets and session history are never included (server-enforced).
import { useEffect, useMemo, useState } from 'react'
import { Boxes, GitBranch, Clock, Sparkles, FileText, Columns3, PackageCheck } from 'lucide-react'
import { api } from '../../api'
import type { WorkspaceExportInclude } from '../../api/market'
import type { Agent, Flow, Skill, Schedule, WorkspaceSettings } from '../../types'
import { Toggle } from '../settings/primitives'

interface Props {
  ws: WorkspaceSettings
  onError: (msg: string) => void
}

// Category flags mirror WorkspaceExportInclude minus the per-agent selection.
type CatKey = 'flows' | 'schedules' | 'skills' | 'instructions' | 'boardColumns'

export function WorkspaceExportPanel({ ws, onError }: Props) {
  const [agents, setAgents] = useState<Agent[]>([])
  const [flows, setFlows] = useState<Flow[]>([])
  const [skills, setSkills] = useState<Skill[]>([])
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [loading, setLoading] = useState(true)

  // Selection state. Agents default to all-selected; every category defaults on.
  const [pickedAgents, setPickedAgents] = useState<Set<string>>(new Set())
  const [cats, setCats] = useState<Record<CatKey, boolean>>({
    flows: true, schedules: true, skills: true, instructions: true, boardColumns: true,
  })

  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)

  useEffect(() => {
    Promise.all([api.listAgents(), api.listFlows(), api.listSkills(), api.listSchedules()])
      .then(([ag, fl, sk, sc]) => {
        setAgents(ag)
        setFlows(fl)
        // Only workspace-tier skills are exportable; global/bundled ones are shared.
        setSkills(sk.filter((s) => s.source === 'workspace'))
        setSchedules(sc)
        setPickedAgents(new Set(ag.map((a) => a.id)))
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const hasInstructions = (ws.instructions ?? '').trim().length > 0
  const boardCount = ws.boardColumns?.length ?? 0

  const toggleAgent = (id: string) =>
    setPickedAgents((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  const allPicked = agents.length > 0 && pickedAgents.size === agents.length
  const toggleAll = () =>
    setPickedAgents(allPicked ? new Set() : new Set(agents.map((a) => a.id)))
  const setCat = (k: CatKey, v: boolean) => setCats((c) => ({ ...c, [k]: v }))

  const canExport = pickedAgents.size > 0

  const catRows: { key: CatKey; label: string; hint: string; icon: typeof GitBranch; count: number }[] = useMemo(
    () => [
      { key: 'flows', label: 'Akışlar (Flows)', hint: 'Ajan düğümleri taşınabilir anahtarlara yeniden yazılır.', icon: GitBranch, count: flows.length },
      { key: 'schedules', label: 'Zamanlamalar', hint: 'Yalnızca seçili ajanlara bağlı zamanlamalar dahil edilir.', icon: Clock, count: schedules.length },
      { key: 'skills', label: 'Workspace skill’leri', hint: 'SKILL.md + ekli dosyalar birebir gömülür (global skill’ler paylaşımlı, dahil değil).', icon: Sparkles, count: skills.length },
      { key: 'instructions', label: 'Workspace talimatları', hint: hasInstructions ? 'config/instructions.md içeriği.' : 'Bu workspace’in talimatı boş.', icon: FileText, count: hasInstructions ? 1 : 0 },
      { key: 'boardColumns', label: 'Pano sütunları', hint: 'Kanban sütun düzeni.', icon: Columns3, count: boardCount },
    ],
    [flows.length, schedules.length, skills.length, hasInstructions, boardCount],
  )

  const doExport = async () => {
    if (!canExport) return
    setBusy(true)
    setMsg(null)
    try {
      const include: WorkspaceExportInclude = {
        // null = all; only send an explicit list when a subset is picked.
        agentIds: allPicked ? null : Array.from(pickedAgents),
        flows: cats.flows,
        schedules: cats.schedules,
        skills: cats.skills,
        instructions: cats.instructions,
        boardColumns: cats.boardColumns,
      }
      const pack = await api.publishPack('workspace', ws.id, include)
      setMsg({
        ok: true,
        text: `“${pack.name}” şablon olarak dışa aktarıldı (id: ${pack.id}). Artık market ve workspace oluşturma ekranında görünür.`,
      })
    } catch (e) {
      setMsg({ ok: false, text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  if (loading) {
    return <div className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</div>
  }

  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2.5 text-xs text-[var(--color-text-dim)]">
        <span className="font-medium text-[var(--color-text)]">{ws.name}</span> workspace’ini taşınabilir bir
        <span className="font-medium text-[var(--color-text)]"> şablon paketine</span> dönüştür. Aşağıdan neyin dahil
        edileceğini seç. <span className="font-medium text-[var(--color-text)]">Sırlar ve oturum geçmişi asla dahil edilmez.</span>
      </div>

      {/* Agents — at least one required */}
      <div className="space-y-2">
        <div className="flex items-center justify-between">
          <span className="flex items-center gap-2 text-sm font-semibold">
            <Boxes size={16} className="text-[var(--color-accent)]" /> Ajanlar
            <span className="text-xs font-normal text-[var(--color-text-dim)]">({pickedAgents.size}/{agents.length})</span>
          </span>
          {agents.length > 0 && (
            <button onClick={toggleAll} className="text-xs text-[var(--color-accent)] hover:underline">
              {allPicked ? 'Tümünü kaldır' : 'Tümünü seç'}
            </button>
          )}
        </div>
        {agents.length === 0 ? (
          <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
            Bu workspace’te ajan yok — dışa aktarım için en az bir ajan gerekir.
          </div>
        ) : (
          <div className="grid grid-cols-1 gap-1.5 sm:grid-cols-2">
            {agents.map((a) => {
              const picked = pickedAgents.has(a.id)
              return (
                <button
                  key={a.id}
                  onClick={() => toggleAgent(a.id)}
                  className={`flex items-center gap-2 rounded-lg border px-3 py-2 text-left text-sm transition ${
                    picked
                      ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)]'
                      : 'border-[var(--color-border)] bg-[var(--color-bg)] hover:bg-[var(--color-surface-2)]'
                  }`}
                >
                  <span
                    className={`flex h-4 w-4 flex-shrink-0 items-center justify-center rounded border text-[10px] ${
                      picked ? 'border-[var(--color-accent)] bg-[var(--color-accent)] text-white' : 'border-[var(--color-border)]'
                    }`}
                  >
                    {picked ? '✓' : ''}
                  </span>
                  <span className="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded bg-[var(--color-surface-2)] text-sm">
                    {a.avatar || '🤖'}
                  </span>
                  <span className="flex-1 truncate">{a.name}</span>
                </button>
              )
            })}
          </div>
        )}
      </div>

      {/* Optional categories */}
      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Dahil edilecekler
      </div>
      <div className="space-y-2">
        {catRows.map((row) => (
          <Toggle
            key={row.key}
            label={`${row.label}${row.count ? ` (${row.count})` : ''}`}
            hint={row.hint}
            checked={cats[row.key]}
            onChange={(v) => setCat(row.key, v)}
          />
        ))}
      </div>

      {/* Action */}
      <div className="mt-3 flex items-center justify-between gap-3 rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2.5">
        <span className="text-xs text-[var(--color-text-dim)]">
          {canExport ? 'Seçili içerik bir şablon paketine yazılır.' : 'Dışa aktarmak için en az bir ajan seç.'}
        </span>
        <button
          onClick={doExport}
          disabled={busy || !canExport}
          className="flex shrink-0 items-center gap-1.5 rounded border border-[var(--color-accent)] px-3 py-1.5 text-xs text-[var(--color-accent)] transition hover:bg-[var(--color-accent-soft)] disabled:opacity-50"
        >
          <PackageCheck size={14} /> {busy ? 'Dışa aktarılıyor…' : 'Şablon olarak dışa aktar'}
        </button>
      </div>
      {msg && (
        <p className={`text-xs ${msg.ok ? 'text-[var(--color-success,#10b981)]' : 'text-[var(--color-danger)]'}`}>
          {msg.text}
        </p>
      )}
    </>
  )
}

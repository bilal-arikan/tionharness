// Dedicated "Dışa Aktar" (export-as-template) sub-panel for a workspace. Split
// out of the General settings panel so the export can be detailed: the user
// picks exactly which agents and which optional categories (flows, schedules,
// workspace skills, instructions, board columns) get captured into the template
// pack. Secrets and session history are never included (server-enforced).
import { useEffect, useMemo, useState } from 'react'
import { Boxes, GitBranch, Clock, Sparkles, FileText, Columns3, PackageCheck, AlertTriangle } from 'lucide-react'
import { api } from '../../api'
import type { WorkspaceExportInclude, WorkspaceExportMeta } from '../../api/market'
import type { Agent, Flow, FlowGraph, Skill, Schedule, WorkspaceSettings } from '../../types'
import { Field, Toggle, inputCls } from '../settings/primitives'

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

  // Pack metadata. Name seeds from the workspace name; description/version stay
  // blank so the server applies its defaults unless the user overrides them.
  const [name, setName] = useState(ws.name)
  const [description, setDescription] = useState('')
  const [version, setVersion] = useState('1.0.0')

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

  // Agent ids each flow's graph references (agent-type nodes only). Parsed once;
  // an unparseable graph yields no refs (the backend skips it too).
  const flowAgentRefs = useMemo(
    () =>
      flows.map((f) => {
        let ids: string[] = []
        try {
          const g = JSON.parse(f.graph) as FlowGraph
          ids = (g.nodes ?? [])
            .filter((n) => n.type === 'agent' && n.agentId)
            .map((n) => n.agentId as string)
        } catch {
          // Malformed graph JSON — leave refs empty rather than failing the panel.
        }
        return { flow: f, agentIds: Array.from(new Set(ids)) }
      }),
    [flows],
  )

  // Dependency warnings: excluding an agent that a flow/schedule depends on has a
  // concrete effect — flows keep a dangling reference (the backend can only
  // rewrite ids it actually exports) and schedules are dropped entirely.
  const depWarnings = useMemo(() => {
    const nameOf = (id: string) => agents.find((a) => a.id === id)?.name ?? id
    const out: { key: string; label: string; detail: string }[] = []
    if (cats.flows) {
      for (const { flow, agentIds } of flowAgentRefs) {
        const missing = agentIds.filter((id) => !pickedAgents.has(id))
        if (missing.length > 0) {
          out.push({
            key: `flow:${flow.id}`,
            label: `Akış “${flow.name || flow.id}”`,
            detail: `hariç bırakılan ajan(lar)a bağlı — dışa aktarımda kopuk referans kalır: ${missing.map(nameOf).join(', ')}`,
          })
        }
      }
    }
    if (cats.schedules) {
      for (const sc of schedules) {
        if (!pickedAgents.has(sc.agentId)) {
          out.push({
            key: `sched:${sc.id}`,
            label: `Zamanlama “${sc.prompt?.slice(0, 32) || sc.id}”`,
            detail: `hariç bırakılan ajana bağlı (${nameOf(sc.agentId)}) — dışa aktarıma dahil edilmez`,
          })
        }
      }
    }
    return out
  }, [flowAgentRefs, pickedAgents, cats.flows, cats.schedules, schedules, agents])

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
      const meta: WorkspaceExportMeta = {
        name: name.trim() || undefined,
        description: description.trim() || undefined,
        version: version.trim() || undefined,
      }
      const pack = await api.publishPack('workspace', ws.id, include, meta)
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

      {/* Pack metadata */}
      <div className="space-y-2">
        <Field label="Şablon adı">
          <input value={name} onChange={(e) => setName(e.target.value)} placeholder={ws.name} className={inputCls} />
        </Field>
        <Field label="Açıklama" hint="Boş bırakılırsa otomatik bir açıklama üretilir.">
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder={`“${ws.name}” workspace'inden dışa aktarıldı.`}
            className={`${inputCls} resize-y`}
          />
        </Field>
        <Field label="Sürüm" hint="Örn. 1.0.0 — boş bırakılırsa 1.0.0 kullanılır.">
          <input value={version} onChange={(e) => setVersion(e.target.value)} placeholder="1.0.0" className={inputCls} />
        </Field>
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

      {/* Dependency warnings — excluded agents that flows/schedules rely on */}
      {depWarnings.length > 0 && (
        <div className="mt-2 space-y-1.5 rounded-lg border border-[color-mix(in_srgb,var(--color-warning,#f59e0b)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-warning,#f59e0b)_8%,transparent)] px-3 py-2.5">
          <div className="flex items-center gap-1.5 text-xs font-semibold text-[var(--color-warning,#f59e0b)]">
            <AlertTriangle size={14} /> Bağımlılık uyarısı ({depWarnings.length})
          </div>
          <ul className="space-y-1 text-xs text-[var(--color-text-dim)]">
            {depWarnings.map((w) => (
              <li key={w.key}>
                <span className="font-medium text-[var(--color-text)]">{w.label}</span> {w.detail}
              </li>
            ))}
          </ul>
        </div>
      )}

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

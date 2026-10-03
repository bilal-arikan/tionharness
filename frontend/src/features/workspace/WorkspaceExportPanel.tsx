// Dedicated "Dışa Aktar" (export-as-template) sub-panel for a workspace. The user
// picks exactly which agents, workspace-tier skills, schedules and
// automations get captured (each item-by-item), plus the file categories (workspace instructions,
// non-default runtime prompts + README, board columns) and pack metadata
// (name/description/version). Secrets and session history are never included
// (server-enforced). A live preview + dependency warnings reflect the selection.
import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  Boxes,
  Clock,
  Sparkles,
  Zap,
  FileText,
  Columns3,
  PackageCheck,
  AlertTriangle,
} from 'lucide-react'
import { api } from '@/api'
import type { WorkspaceExportInclude, WorkspaceExportMeta } from '@/api/market'
import type {
  Agent,
  Automation,
  Skill,
  Schedule,
  WorkspaceConfig,
  WorkspaceSettings,
} from '@/types'
import { Field, Toggle, inputCls } from '@/features/settings/primitives'
import { ExportPickList, type PickEntry } from './ExportPickList'

interface Props {
  ws: WorkspaceSettings
  onError: (msg: string) => void
}

// File-category flags (non-item categories) captured alongside the item lists.
type CatKey = 'instructions' | 'prompts' | 'boardColumns'

// slugify mirrors the Go server's slugify (internal/api/market_publish.go): lower,
// keep [a-z0-9], collapse other runs to a single dash, trim dashes. Non-ASCII
// (e.g. Turkish) letters are dropped — so the preview shows the same pack id the
// server will derive.
function slugify(s: string): string {
  let out = ''
  let prevDash = false
  for (const ch of s.toLowerCase().trim()) {
    if ((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9')) {
      out += ch
      prevDash = false
    } else if (!prevDash && out.length > 0) {
      out += '-'
      prevDash = true
    }
  }
  return out.replace(/^-+|-+$/g, '')
}

// selectionToIds returns null when every entry is picked (backend "all" shortcut),
// otherwise the explicit id array (possibly empty = none).
function selectionToIds(picked: Set<string>, all: PickEntry[]): string[] | null {
  return all.length > 0 && picked.size === all.length ? null : Array.from(picked)
}

export function WorkspaceExportPanel({ ws, onError }: Props) {
  const { t } = useTranslation('workspace')
  const [agents, setAgents] = useState<Agent[]>([])
  const [skills, setSkills] = useState<Skill[]>([])
  const [schedules, setSchedules] = useState<Schedule[]>([])
  const [automations, setAutomations] = useState<Automation[]>([])
  const [config, setConfig] = useState<WorkspaceConfig | null>(null)
  const [loading, setLoading] = useState(true)

  // Pack metadata. Name seeds from the workspace name; description/version stay
  // blank so the server applies its defaults unless the user overrides them.
  const [name, setName] = useState(ws.name)
  const [description, setDescription] = useState('')
  const [version, setVersion] = useState('1.0.0')

  // Per-item selection sets (default all-selected once loaded).
  const [pickedAgents, setPickedAgents] = useState<Set<string>>(new Set())
  const [pickedSkills, setPickedSkills] = useState<Set<string>>(new Set())
  const [pickedSchedules, setPickedSchedules] = useState<Set<string>>(new Set())
  const [pickedAutomations, setPickedAutomations] = useState<Set<string>>(new Set())
  // File categories (default on).
  const [cats, setCats] = useState<Record<CatKey, boolean>>({
    instructions: true,
    prompts: true,
    boardColumns: true,
  })

  const [busy, setBusy] = useState(false)
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)

  useEffect(() => {
    Promise.all([
      api.listAgents(),
      api.listSkills(),
      api.listSchedules(),
      api.listAutomations(),
      api.getWorkspaceConfig(),
    ])
      .then(([ag, sk, sc, au, cfg]) => {
        const wsSkills = sk.filter((s) => s.source === 'workspace') // global/bundled are shared, not exportable
        setAgents(ag)
        setSkills(wsSkills)
        setSchedules(sc)
        // Built-in seeded board rules are provisioned per workspace at open time,
        // so exporting them would duplicate (or resurrect) them on install.
        const ownAutos = au.filter((a) => !a.seed)
        setAutomations(ownAutos)
        setConfig(cfg)
        setPickedAgents(new Set(ag.map((a) => a.id)))
        setPickedSkills(new Set(wsSkills.map((s) => s.slug)))
        setPickedSchedules(new Set(sc.map((s) => s.id)))
        setPickedAutomations(new Set(ownAutos.map((a) => a.id)))
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => setLoading(false))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const hasInstructions = (ws.instructions ?? '').trim().length > 0
  const boardCount = ws.boardColumns?.length ?? 0

  // Non-default runtime prompts (current file content differs from the compiled-in
  // default) + a non-empty README — the "other prompts" the export can carry.
  const customPromptKeys = useMemo(() => {
    if (!config) return []
    return config.promptKeys.filter(
      (k) => (config.prompts[k] ?? '').trim() !== (config.defaults[k] ?? '').trim(),
    )
  }, [config])
  const hasReadme = (config?.readme ?? '').trim().length > 0
  const promptsCount = customPromptKeys.length + (hasReadme ? 1 : 0)

  const setCat = (k: CatKey, v: boolean) => setCats((c) => ({ ...c, [k]: v }))
  const canExport = pickedAgents.size > 0

  const agentName = useCallback(
    (id: string) => agents.find((a) => a.id === id)?.name ?? id,
    [agents],
  )

  // Section entries for the reusable pick lists.
  const agentEntries: PickEntry[] = useMemo(
    () => agents.map((a) => ({ id: a.id, label: a.name, emoji: a.avatar || '🤖' })),
    [agents],
  )
  const skillEntries: PickEntry[] = useMemo(
    () => skills.map((s) => ({ id: s.slug, label: s.name || s.slug, sub: s.slug })),
    [skills],
  )
  const automationEntries: PickEntry[] = useMemo(
    () =>
      automations.map((a) => ({
        id: a.id,
        label: a.name || a.id,
        sub: agentName(a.targetAgentId),
      })),
    [automations, agentName],
  )
  const scheduleEntries: PickEntry[] = useMemo(
    () =>
      schedules.map((s) => ({
        id: s.id,
        label: s.prompt?.trim() || t('export.noPrompt'),
        sub: `${agentName(s.agentId)} · ${s.cronExpr}`,
      })),
    [schedules, agentName, t],
  )

  // Dependency warnings: only for SELECTED schedules/automations whose agent is
  // excluded — such a rule is dropped by the backend (orphan).
  const depWarnings = useMemo(() => {
    const out: { key: string; label: string; detail: string }[] = []
    for (const sc of schedules) {
      if (!pickedSchedules.has(sc.id)) continue
      if (!pickedAgents.has(sc.agentId)) {
        out.push({
          key: `sched:${sc.id}`,
          label: t('export.warnings.scheduleLabel', {
            name: sc.prompt?.slice(0, 32) || sc.id,
          }),
          detail: t('export.warnings.excludedAgent', { agent: agentName(sc.agentId) }),
        })
      }
    }
    for (const au of automations) {
      if (!pickedAutomations.has(au.id)) continue
      if (au.targetAgentId && !pickedAgents.has(au.targetAgentId)) {
        out.push({
          key: `auto:${au.id}`,
          label: t('export.warnings.automationLabel', { name: au.name || au.id }),
          detail: t('export.warnings.excludedAgent', { agent: agentName(au.targetAgentId) }),
        })
      }
    }
    return out
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pickedSchedules, pickedAutomations, pickedAgents, schedules, automations, agents, t])

  // Live preview: the exact contents the current selection would produce. Schedules
  // and automations only survive when bound to a selected agent (orphans
  // dropped), mirroring backend.
  const preview = useMemo(() => {
    const schedEffective = schedules.filter(
      (sc) => pickedSchedules.has(sc.id) && pickedAgents.has(sc.agentId),
    ).length
    const autoEffective = automations.filter(
      (au) =>
        pickedAutomations.has(au.id) && (!au.targetAgentId || pickedAgents.has(au.targetAgentId)),
    ).length
    const resolvedName = name.trim() || ws.name
    const slug = slugify(resolvedName) || ws.id.toLowerCase()
    return {
      resolvedName,
      version: version.trim() || '1.0.0',
      packId: `workspace-${slug}`,
      willOverwrite: slug === (slugify(ws.name) || ws.id.toLowerCase()),
      items: [
        {
          icon: Boxes,
          label: t('export.preview.items.agent'),
          count: pickedAgents.size,
          on: pickedAgents.size > 0,
        },
        {
          icon: Clock,
          label: t('export.preview.items.schedule'),
          count: schedEffective,
          on: schedEffective > 0,
        },
        {
          icon: Zap,
          label: t('export.preview.items.automation'),
          count: autoEffective,
          on: autoEffective > 0,
        },
        {
          icon: Sparkles,
          label: t('export.preview.items.skill'),
          count: pickedSkills.size,
          on: pickedSkills.size > 0,
        },
        {
          icon: FileText,
          label: t('export.preview.items.instructions'),
          count: cats.instructions && hasInstructions ? 1 : 0,
          on: cats.instructions && hasInstructions,
        },
        {
          icon: FileText,
          label: t('export.preview.items.prompts'),
          count: cats.prompts ? promptsCount : 0,
          on: cats.prompts && promptsCount > 0,
        },
        {
          icon: Columns3,
          label: t('export.preview.items.boardColumn'),
          count: cats.boardColumns ? boardCount : 0,
          on: cats.boardColumns && boardCount > 0,
        },
      ],
    }
  }, [
    schedules,
    pickedSchedules,
    automations,
    pickedAutomations,
    pickedAgents,
    pickedSkills,
    cats,
    name,
    version,
    ws.name,
    ws.id,
    hasInstructions,
    promptsCount,
    boardCount,
    t,
  ])

  const doExport = async () => {
    if (!canExport) return
    setBusy(true)
    setMsg(null)
    try {
      const include: WorkspaceExportInclude = {
        agentIds: selectionToIds(pickedAgents, agentEntries),
        skillSlugs: selectionToIds(pickedSkills, skillEntries),
        scheduleIds: selectionToIds(pickedSchedules, scheduleEntries),
        automationIds: selectionToIds(pickedAutomations, automationEntries),
        instructions: cats.instructions,
        prompts: cats.prompts,
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
        text: t('export.success', { name: pack.name, id: pack.id }),
      })
    } catch (e) {
      setMsg({ ok: false, text: (e as Error).message })
    } finally {
      setBusy(false)
    }
  }

  if (loading) {
    return <div className="text-sm text-[var(--color-text-dim)]">{t('actions.loading')}</div>
  }

  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2.5 text-xs text-[var(--color-text-dim)]">
        {t('export.intro.beforeName')}
        <span className="font-medium text-[var(--color-text)]">{ws.name}</span>
        {t('export.intro.afterName')}
        <span className="font-medium text-[var(--color-text)]">{t('export.intro.pack')}</span>
        {t('export.intro.afterPack')}{' '}
        <span className="font-medium text-[var(--color-text)]">{t('export.secrets')}</span>
      </div>

      {/* Pack metadata */}
      <div className="space-y-2">
        <Field label={t('export.metadata.name')}>
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder={ws.name}
            className={inputCls}
          />
        </Field>
        <Field label={t('export.metadata.description')} hint={t('export.metadata.descriptionHint')}>
          <textarea
            value={description}
            onChange={(e) => setDescription(e.target.value)}
            rows={2}
            placeholder={t('export.metadata.descriptionPlaceholder', { name: ws.name })}
            className={`${inputCls} resize-y`}
          />
        </Field>
        <Field label={t('export.metadata.version')} hint={t('export.metadata.versionHint')}>
          <input
            value={version}
            onChange={(e) => setVersion(e.target.value)}
            placeholder="1.0.0"
            className={inputCls}
          />
        </Field>
      </div>

      {/* Item selections — agents / skills / schedules */}
      <div className="mt-2 space-y-4 border-t border-[var(--color-border)] pt-3">
        <ExportPickList
          title={t('export.sections.agents.title')}
          icon={Boxes}
          entries={agentEntries}
          picked={pickedAgents}
          setPicked={setPickedAgents}
          emptyHint={t('export.sections.agents.empty')}
        />
        <ExportPickList
          title={t('export.sections.skills.title')}
          icon={Sparkles}
          entries={skillEntries}
          picked={pickedSkills}
          setPicked={setPickedSkills}
          emptyHint={t('export.sections.skills.empty')}
          note={t('export.sections.skills.note')}
        />
        <ExportPickList
          title={t('export.sections.schedules.title')}
          icon={Clock}
          entries={scheduleEntries}
          picked={pickedSchedules}
          setPicked={setPickedSchedules}
          emptyHint={t('export.sections.schedules.empty')}
          note={t('export.sections.schedules.note')}
        />
        <ExportPickList
          title={t('export.sections.automations.title')}
          icon={Zap}
          entries={automationEntries}
          picked={pickedAutomations}
          setPicked={setPickedAutomations}
          emptyHint={t('export.sections.automations.empty')}
          note={t('export.sections.automations.note')}
        />
      </div>

      {/* File categories */}
      <div className="mt-2 border-t border-[var(--color-border)] pt-3 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        {t('export.files.title')}
      </div>
      <div className="space-y-2">
        <Toggle
          label={`${t('export.files.instructions')}${hasInstructions ? '' : ` ${t('export.files.emptySuffix')}`}`}
          hint={t('export.files.instructionsHint')}
          checked={cats.instructions}
          onChange={(v) => setCat('instructions', v)}
        />
        <Toggle
          label={`${t('export.files.prompts')}${promptsCount ? ` (${promptsCount})` : ` ${t('export.files.defaultSuffix')}`}`}
          hint={
            promptsCount
              ? t('export.files.promptsHintCustom', { readme: hasReadme ? ' + README' : '' })
              : t('export.files.promptsHintEmpty')
          }
          checked={cats.prompts}
          onChange={(v) => setCat('prompts', v)}
        />
        <Toggle
          label={`${t('export.files.boardColumns')}${boardCount ? ` (${boardCount})` : ''}`}
          hint={t('export.files.boardColumnsHint')}
          checked={cats.boardColumns}
          onChange={(v) => setCat('boardColumns', v)}
        />
      </div>

      {/* Live preview — the exact contents the current selection produces */}
      <div className="mt-2 space-y-2 rounded-lg border border-[var(--color-border)] bg-[var(--color-bg)] px-3 py-2.5">
        <div className="flex items-center justify-between gap-2">
          <span className="flex items-center gap-1.5 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            <PackageCheck size={13} className="text-[var(--color-accent)]" />{' '}
            {t('export.preview.title')}
          </span>
          <span className="truncate text-xs text-[var(--color-text-dim)]">
            <span className="font-medium text-[var(--color-text)]">{preview.resolvedName}</span>
            {t('export.preview.identityVersion', { version: preview.version })}
            <code className="rounded bg-[var(--color-surface-2)] px-1">{preview.packId}</code>
          </span>
        </div>
        <div className="flex flex-wrap gap-1.5">
          {preview.items.map((it) => (
            <span
              key={it.label}
              className={`flex items-center gap-1 rounded-md border px-2 py-1 text-xs ${
                it.on && it.count > 0
                  ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] text-[var(--color-text)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)] line-through opacity-60'
              }`}
              title={it.on ? '' : t('export.preview.excluded')}
            >
              <it.icon size={12} /> {it.count} {it.label}
            </span>
          ))}
        </div>
        <div className="text-[11px] text-[var(--color-text-dim)]">
          {preview.willOverwrite
            ? t('export.preview.overwrite', { id: preview.packId })
            : t('export.preview.newId', { id: preview.packId })}{' '}
          {t('export.secrets')}
        </div>
      </div>

      {/* Dependency warnings — excluded agents that selected schedules/automations rely on */}
      {depWarnings.length > 0 && (
        <div className="mt-2 space-y-1.5 rounded-lg border border-[color-mix(in_srgb,var(--color-warning,#f59e0b)_40%,transparent)] bg-[color-mix(in_srgb,var(--color-warning,#f59e0b)_8%,transparent)] px-3 py-2.5">
          <div className="flex items-center gap-1.5 text-xs font-semibold text-[var(--color-warning,#f59e0b)]">
            <AlertTriangle size={14} /> {t('export.warnings.title', { count: depWarnings.length })}
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
          {canExport ? t('export.action.ready') : t('export.action.selectAgent')}
        </span>
        <button
          onClick={doExport}
          disabled={busy || !canExport}
          className="flex shrink-0 items-center gap-1.5 rounded border border-[var(--color-accent)] px-3 py-1.5 text-xs text-[var(--color-accent)] transition hover:bg-[var(--color-accent-soft)] disabled:opacity-50"
        >
          <PackageCheck size={14} />{' '}
          {busy ? t('export.action.exporting') : t('export.action.export')}
        </button>
      </div>
      {msg && (
        <p
          className={`text-xs ${msg.ok ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'}`}
        >
          {msg.text}
        </p>
      )}
    </>
  )
}

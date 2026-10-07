import { useMemo, useState } from 'react'
import { Eye, Trash2, Star, Copy, RotateCcw, Power, GitBranch, Lock } from 'lucide-react'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import type { View } from '@/app/NavRail'
import type { Agent, AgentOverrideKey, AgentPatch } from '@/types'
import { AVATAR_COLORS, normalizeAvatar, resolveColor } from '@/shared/lib/avatar'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { EmojiField } from '@/shared/components/EmojiField'
import { ProviderInstanceModelSelect } from '@/shared/components/agents/ProviderInstanceModelSelect'
import { AgentToolsSection } from './AgentToolsSection'
import { AgentSkillsSection } from './AgentSkillsSection'
import { AgentContextModal } from './AgentContextModal'
import { Button, PromptEditor } from '@/shared/components'
import { FieldHint, InfoPopover } from '@/shared/components/InfoPopover'
import { OptionPills } from '@/shared/components/OptionPills'
import { CoordinatorWorkflowPicker } from '@/shared/components/CoordinatorWorkflowPicker'
import { useCatalog, thinkingOptionsForModel } from '@/shared/lib/catalog'
import { hasOverride } from '@/shared/lib/agentLineage'
import {
  BOOLEAN_OPTIONS,
  PERMISSION_OPTIONS,
  THINKING_OPTIONS,
  booleanFromOption,
} from './agentOptions'
import { SystemAgentStatusBadge } from './SystemAgentStatusBadge'
import { BuiltinPromptRevert } from './BuiltinPromptRevert'
import { AgentLineageChips } from './AgentLineageChips'
import { FieldOverrideBadge } from './FieldOverrideBadge'
import { useTranslation } from 'react-i18next'

interface Props {
  agent: Agent
  onSave: (patch: AgentPatch) => Promise<{ warning?: string } | void>
  /** Called after a successful save (e.g. close the modal). */
  onSaved?: () => void
  /** Secondary action button (e.g. Cancel in the modal). */
  onCancel?: () => void
  cancelLabel?: string
  /** Danger action: when set, a "Sil" button is shown at the footer-left. */
  onDelete?: () => void
  /** Drop every override on a derived agent (inherit everything again). */
  onRestoreDefault?: () => void
  onToggleDisabled?: () => void
  systemActionPending?: boolean
  /** Clone the agent: when set, a "Klonla" button is shown in the header. The
   * clone carries over every setting (profile + provider/model + tools + skills).
   * Resolves once the clone exists (the parent then selects it). */
  onDuplicate?: () => Promise<string | undefined>
  /** Derive a child that inherits every field. bindRole takes over the parent's
   * system role — the way a locked built-in is customised. */
  onDerive?: (opts: { bindRole: boolean }) => Promise<string | undefined>
  /** Whether this agent is the default for new chats. Drives the header star
   * toggle's filled/active state. */
  isDefault?: boolean
  defaultSaveState?: 'idle' | 'saving' | 'saved'
  /** Make this agent the default for new chats. When set, a star toggle is shown
   * in the header (mirrors the roster's ★/☆ button). Takes effect immediately —
   * independent of the form's Save. */
  onSetDefault?: () => void
  /** When set, unsaved edits are surfaced on this nav view's dirty indicator.
   * The AgentsView page passes "agents"; the modal reuse leaves it unset so it
   * does not flag the nav from an unrelated screen. */
  dirtyView?: View
  /** Inheritance context (all optional — the modal reuse passes none). */
  /** Ancestors ROOT FIRST; empty for a root agent. */
  lineage?: Agent[]
  /** The direct parent (resolved), used to show the inherited value after a
   * field reset without a round trip. */
  parent?: Agent | null
  /** Agents this one may pick as its parent (see eligibleParents). */
  parentOptions?: Agent[]
  /** Jump to another agent's settings (lineage chips). */
  onSelectAgent?: (id: string) => void
  /** Render as an inspect-only view: every field is disabled and no mutating
   * action is offered, even for an agent that is otherwise editable. The Agents
   * screen passes this for system agents, which are edited from
   * Settings → "Sistem ajanları" instead. */
  readOnly?: boolean
  /** Note shown at the top of a read-only form, saying where the agent IS
   * editable. Ignored unless `readOnly`. */
  readOnlyNote?: React.ReactNode
  /** Whether the plain "Türet" button is offered. The System agents screen sets
   * it false: there, the only sanctioned derivation is "Özelleştir", which binds
   * the copy to the built-in's role. Has no effect on "Özelleştir" itself.
   * `readOnly` suppresses "Türet" regardless of this flag. */
  allowFreeDerive?: boolean
}

// Field units of this form that can be inherited. "tools" and "allowedTools"
// are handled by the instant-save tools section.
const FORM_OVERRIDE_KEYS: AgentOverrideKey[] = [
  'soul',
  'identity',
  'provider',
  'model',
  'thinkingLevel',
  'nativeWebSearch',
  'nativeShell',
  'permissionMode',
  'avatar',
  'color',
  'skills',
  'coordinatorMode',
  'coordinatorWorkflow',
  'coordinatorPrompt',
]

// AgentSettingsForm is the editable agent profile (visual identity + core
// fields). It is reused both inside the modal (roster gear) and as the right
// pane of the two-panel Agents view. Mount with a key={agent.id} so switching
// the selected agent resets the field state.
//
// Inheritance: on a DERIVED agent (agent.parentId set) every inheritable field
// carries a badge — "devralındı" or "override" — and editing a field pins it.
// Save sends ONLY the pinned fields (plus the name) and lists the released
// ones in resetFields, so an untouched field keeps following the parent. A
// LOCKED built-in renders read-only; the way to change it is "Özelleştir".
export function AgentSettingsForm({
  agent,
  onSave,
  onSaved,
  onCancel,
  cancelLabel,
  onDelete,
  onRestoreDefault,
  onToggleDisabled,
  systemActionPending = false,
  onDuplicate,
  onDerive,
  isDefault,
  defaultSaveState = 'idle',
  onSetDefault,
  dirtyView,
  lineage = [],
  parent = null,
  parentOptions = [],
  onSelectAgent,
  readOnly = false,
  readOnlyNote,
  allowFreeDerive = true,
}: Props) {
  const { t } = useTranslation('agents')
  // `locked` gates every editor and mutating action below. It now follows
  // `readOnly` ALONE: a built-in system agent is editable in place, its edit
  // stored in the app-global layer and applied to every workspace, so it no
  // longer has to be copied to be changed. What a built-in still cannot do —
  // be deleted, disabled or re-parented — is gated on `agent.locked` at each of
  // those actions instead of by disabling the whole form.
  const locked = readOnly
  // A built-in stays fixed in the ways that are the compiled registry's business:
  // it always exists, always serves its role, and is never a free-standing agent
  // to clone. Those actions are gated on this rather than on `locked`, which now
  // only means "this screen is inspecting, not editing".
  const isBuiltin = !!agent.locked
  const isChild = !!(agent.parentId || agent.catalogParentId)
  const libraryParent = !!agent.catalogParentId && !agent.parentId
  const [name, setName] = useState(agent.name)
  // Seed with a normalized avatar so an existing mojibake value is repaired on
  // open and persisted clean when the form is saved.
  const [avatar, setAvatar] = useState(normalizeAvatar(agent.avatar) ?? '')
  const [color, setColor] = useState(agent.color ?? '')
  const [soul, setSoul] = useState(agent.soul ?? '')
  const [identity, setIdentity] = useState(agent.identity ?? '')
  const [provider, setProvider] = useState(agent.provider)
  const [providerInstanceId, setProviderInstanceId] = useState(
    agent.providerInstanceId || agent.provider,
  )
  const [model, setModel] = useState(agent.model ?? '')
  // A legacy row loaded before the boot migration may still carry '', which is no
  // longer a value the backend accepts — show (and re-save) it as the 'off' it
  // behaved as on every non-CLI provider.
  const [thinkingLevel, setThinkingLevel] = useState(agent.thinkingLevel || 'off')
  // undefined = the agent never stored the flag, which means ENABLED (see
  // Agent.nativeWebSearch) — the checkbox must start checked, not cleared.
  const [nativeWebSearch, setNativeWebSearch] = useState(agent.nativeWebSearch ?? true)
  // Opt-IN (see Agent.nativeShell): undefined means the bridged shell alone.
  const [nativeShell, setNativeShell] = useState(agent.nativeShell ?? false)
  const [permissionMode, setPermissionMode] = useState(agent.permissionMode || 'auto')
  const [skills, setSkills] = useState<string[]>(agent.skills ?? [])
  const [coordinatorMode, setCoordinatorMode] = useState(agent.coordinatorMode ?? false)
  const [coordinatorWorkflow, setCoordinatorWorkflow] = useState(agent.coordinatorWorkflow ?? '')
  const [coordinatorPrompt, setCoordinatorPrompt] = useState(agent.coordinatorPrompt ?? '')
  // Which inheritable fields this agent PINS. Seeded from the server; editing a
  // field adds its key, the per-field reset removes it. Meaningless on a root.
  const [overrides, setOverrides] = useState<Set<AgentOverrideKey>>(
    () => new Set(agent.overrides ?? []),
  )
  const [saving, setSaving] = useState(false)
  const [duplicating, setDuplicating] = useState(false)
  const [deriving, setDeriving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState(0)
  const [previewOpen, setPreviewOpen] = useState(false)
  // Remounts the tools section after a "tools" override reset (instant save).
  const [toolsKey, setToolsKey] = useState(0)
  // P1.3: warning from the backend (e.g. "model not in price table"). Cleared on
  // the next save or when the model/provider changes.
  const [saveWarn, setSaveWarn] = useState<string | null>(null)

  // Reasoning tiers + class for the chosen provider+model. Every pill stays
  // visible; no-op levels are shown greyed with a reason (e.g. "Kapalı" on the
  // always-on Fable class, "Çok yüksek"/"Maks" on legacy models that clamp them,
  // or "Orta"/"Çok yüksek" on effort-class models). tiers null = unknown/custom
  // → all enabled. Every pill value is already a backend token; the
  // currently-stored level stays selectable even if outside the set.
  const catalog = useCatalog()
  const thinkingOptions = useMemo(
    () => thinkingOptionsForModel(THINKING_OPTIONS, catalog, provider, model, thinkingLevel),
    [catalog, provider, model, thinkingLevel],
  )

  const mark = (key: AgentOverrideKey) => {
    if (!isChild) return
    setOverrides((prev) => (prev.has(key) ? prev : new Set(prev).add(key)))
  }
  const overridden = (key: AgentOverrideKey) => isChild && overrides.has(key)

  // Release an override: the field shows the parent's value again (when the
  // parent is known; otherwise the server refreshes it on save).
  const reset = (key: AgentOverrideKey) => {
    setOverrides((prev) => {
      const next = new Set(prev)
      next.delete(key)
      return next
    })
    if (!parent) return
    switch (key) {
      case 'soul':
        setSoul(parent.soul ?? '')
        break
      case 'identity':
        setIdentity(parent.identity ?? '')
        break
      case 'provider':
        setProvider(parent.provider)
        setProviderInstanceId(parent.providerInstanceId || parent.provider)
        break
      case 'model':
        setModel(parent.model ?? '')
        break
      case 'thinkingLevel':
        setThinkingLevel(parent.thinkingLevel || 'off')
        break
      case 'nativeWebSearch':
        setNativeWebSearch(parent.nativeWebSearch ?? true)
        break
      case 'nativeShell':
        setNativeShell(parent.nativeShell ?? false)
        break
      case 'permissionMode':
        setPermissionMode(parent.permissionMode || 'auto')
        break
      case 'avatar':
        setAvatar(normalizeAvatar(parent.avatar) ?? '')
        break
      case 'color':
        setColor(parent.color ?? '')
        break
      case 'skills':
        setSkills(parent.skills ?? [])
        break
      case 'coordinatorMode':
        setCoordinatorMode(parent.coordinatorMode ?? false)
        break
      case 'coordinatorWorkflow':
        setCoordinatorWorkflow(parent.coordinatorWorkflow ?? '')
        break
      case 'coordinatorPrompt':
        setCoordinatorPrompt(parent.coordinatorPrompt ?? '')
        break
      default:
        break
    }
  }

  // Unsaved-edits flag: current form fields vs the agent's persisted values.
  // (The tools section saves instantly on its own, so it is not part of this.)
  const overridesDirty = useMemo(() => {
    if (!isChild) return false
    const stored = new Set((agent.overrides ?? []).filter((k) => FORM_OVERRIDE_KEYS.includes(k)))
    const local = new Set([...overrides].filter((k) => FORM_OVERRIDE_KEYS.includes(k)))
    if (stored.size !== local.size) return true
    for (const k of stored) if (!local.has(k)) return true
    return false
  }, [agent.overrides, overrides, isChild])
  const dirty = useMemo(
    () =>
      name !== agent.name ||
      avatar !== (normalizeAvatar(agent.avatar) ?? '') ||
      color !== (agent.color ?? '') ||
      soul !== (agent.soul ?? '') ||
      identity !== (agent.identity ?? '') ||
      providerInstanceId !== (agent.providerInstanceId || agent.provider) ||
      model !== (agent.model ?? '') ||
      thinkingLevel !== (agent.thinkingLevel || 'off') ||
      nativeWebSearch !== (agent.nativeWebSearch ?? true) ||
      nativeShell !== (agent.nativeShell ?? false) ||
      permissionMode !== (agent.permissionMode || 'auto') ||
      coordinatorMode !== (agent.coordinatorMode ?? false) ||
      coordinatorWorkflow !== (agent.coordinatorWorkflow ?? '') ||
      coordinatorPrompt !== (agent.coordinatorPrompt ?? '') ||
      JSON.stringify(skills) !== JSON.stringify(agent.skills ?? []) ||
      overridesDirty,
    [
      name,
      avatar,
      color,
      soul,
      identity,
      providerInstanceId,
      model,
      thinkingLevel,
      nativeWebSearch,
      nativeShell,
      permissionMode,
      coordinatorMode,
      coordinatorWorkflow,
      coordinatorPrompt,
      skills,
      overridesDirty,
      agent,
    ],
  )
  // Surface unsaved agent edits on the nav "Ajanlar" item (page reuse only).
  useRegisterDirty(dirtyView, dirty && !locked)

  const preview: Pick<Agent, 'id' | 'name' | 'avatar' | 'color'> = {
    id: agent.id,
    name: name || agent.name,
    avatar,
    color,
  }

  // The full patch every field of a ROOT agent is written with.
  const fullPatch = (): AgentPatch => ({
    name: name.trim(),
    avatar,
    color,
    soul,
    identity,
    // AgentPatch.provider is interpreted server-side as a provider INSTANCE
    // id (_Docs/71 §5) — send the selected instance, not the derived kind.
    provider: providerInstanceId,
    model: model.trim(),
    thinkingLevel,
    nativeWebSearch,
    nativeShell,
    permissionMode,
    skills,
    coordinatorMode,
    // A recipe without coordinator mode has nothing to apply to; clear it
    // rather than persisting a setting that silently does nothing.
    coordinatorWorkflow: coordinatorMode ? coordinatorWorkflow : '',
    // Same reasoning as the recipe above: this prompt is only ever injected
    // while coordinating, so without coordinator mode it is dead text.
    coordinatorPrompt: coordinatorMode ? coordinatorPrompt : '',
  })

  // On a derived agent only the PINNED fields travel: a field not in the patch
  // keeps inheriting, and a released one is listed in resetFields.
  const childPatch = (): AgentPatch => {
    const full = fullPatch()
    const patch: AgentPatch = { name: full.name }
    const pin = <K extends keyof AgentPatch>(key: AgentOverrideKey, ...fields: K[]) => {
      if (!overrides.has(key)) return
      for (const f of fields) patch[f] = full[f]
    }
    pin('soul', 'soul')
    pin('identity', 'identity')
    pin('provider', 'provider')
    pin('model', 'model')
    pin('thinkingLevel', 'thinkingLevel')
    pin('nativeWebSearch', 'nativeWebSearch')
    pin('nativeShell', 'nativeShell')
    pin('permissionMode', 'permissionMode')
    pin('avatar', 'avatar')
    pin('color', 'color')
    pin('skills', 'skills')
    pin('coordinatorMode', 'coordinatorMode')
    pin('coordinatorWorkflow', 'coordinatorWorkflow')
    pin('coordinatorPrompt', 'coordinatorPrompt')
    const released = (agent.overrides ?? []).filter(
      (k) => FORM_OVERRIDE_KEYS.includes(k) && !overrides.has(k),
    )
    if (released.length) patch.resetFields = released
    return patch
  }

  const save = async () => {
    if (!name.trim()) {
      setErr(t('settings.nameRequired'))
      return
    }
    setSaving(true)
    setErr(null)
    setSaveWarn(null)
    try {
      const result = await onSave(isChild ? childPatch() : fullPatch())
      if (result?.warning) setSaveWarn(result.warning)
      setSavedAt((n) => n + 1)
      onSaved?.()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // Instant actions (no form Save): re-parent, and release the tools override.
  const changeParent = async (parentId: string) => {
    if (parentId === (agent.parentId ?? '')) return
    const msg = parentId
      ? t('settings.changeParentConfirm', {
          agent: agent.name,
          parent: parentOptions.find((p) => p.id === parentId)?.name ?? parentId,
        })
      : t('settings.detachParentConfirm', { agent: agent.name })
    if (!confirm(msg)) return
    setErr(null)
    try {
      await onSave({ parentId })
    } catch (e) {
      setErr((e as Error).message)
    }
  }
  const resetTools = async () => {
    setErr(null)
    try {
      await onSave({ resetFields: ['tools', 'allowedTools'] })
      setToolsKey((n) => n + 1)
    } catch (e) {
      setErr((e as Error).message)
    }
  }

  const duplicate = async () => {
    if (!onDuplicate) return
    setDuplicating(true)
    setErr(null)
    try {
      await onDuplicate()
      // The parent selects the new clone, remounting this form via key={id}; no
      // local state reset needed here.
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setDuplicating(false)
    }
  }

  const derive = async (bindRole: boolean) => {
    if (!onDerive) return
    setDeriving(true)
    setErr(null)
    try {
      await onDerive({ bindRole })
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setDeriving(false)
    }
  }

  const parentName = parent?.name
  // How inheritance works for this agent, behind an (i) beside the lineage chips.
  const inheritInfo = !isChild
    ? null
    : !agent.system
      ? t('settings.inheritNote', { parent: parentName ?? t('settings.parentFallback') })
      : !agent.locked
        ? t('settings.roleNote', { parent: parentName ?? agent.systemKey, role: agent.systemKey })
        : null
  const badge = (key: AgentOverrideKey) =>
    isChild ? (
      <FieldOverrideBadge
        field={key}
        overridden={overridden(key)}
        parentName={parentName}
        onReset={() => reset(key)}
        disabled={saving}
      />
    ) : null

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Header with live avatar preview + actions (delete / save). The
          inheritance chain gets its own full-width row underneath so a long
          chain never squeezes the name against the action buttons. */}
      <div className="space-y-2 border-b border-[var(--color-border)] px-5 py-4">
        <div className="flex flex-wrap items-center gap-3">
          <AgentAvatar agent={preview} size={44} />
          <div className="min-w-[14rem] flex-1">
            <div className="flex items-baseline gap-2">
              <h2 className="truncate text-sm font-semibold text-[var(--color-text)]">
                {name || t('settings.fallbackName')}
              </h2>
              <span
                title={t('settings.idTitle')}
                className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-text-dim)]"
              >
                {agent.id}
              </span>
              <SystemAgentStatusBadge agent={agent} />
            </div>
            <p className="text-xs text-[var(--color-text-dim)]">
              {agent.locked
                ? t('settings.builtinSummary')
                : readOnly
                  ? t('settings.readOnlySummary')
                  : isChild
                    ? t('settings.childSummary', {
                        name: parentName ?? t('settings.parentFallback'),
                      })
                    : t('settings.summary')}
            </p>
          </div>
          <div className="flex flex-1 flex-wrap items-center justify-end gap-2">
            {savedAt > 0 && !saving && (
              <span className="text-xs text-[var(--color-text-dim)]">{t('settings.saved')}</span>
            )}
            {/* A system agent serves the runtime (titling, compaction, workers) and
              is never a conversation partner, so it cannot be the default agent
              for new sessions — the backend rejects it. Hide the action entirely
              rather than offering a button that can only fail. */}
            {onSetDefault && !agent.system && (
              <button
                data-testid="agent-set-default-detail"
                onClick={() => {
                  if (!isDefault) onSetDefault()
                }}
                disabled={isDefault || defaultSaveState === 'saving'}
                title={isDefault ? t('settings.defaultTitle') : t('settings.makeDefaultTitle')}
                className={`flex items-center gap-1.5 rounded px-3 py-1.5 text-sm transition ${
                  isDefault
                    ? 'text-[var(--color-accent)]'
                    : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
                }`}
              >
                <Star size={14} className={isDefault ? 'fill-current' : ''} />
                {defaultSaveState === 'saving'
                  ? t('settings.saving')
                  : isDefault
                    ? defaultSaveState === 'saved'
                      ? t('settings.saved')
                      : t('settings.default')
                    : t('settings.makeDefault')}
              </button>
            )}
            <button
              data-testid="agent-preview-context"
              onClick={() => setPreviewOpen(true)}
              title={t('settings.contextTitle')}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
            >
              <Eye size={14} /> {t('settings.context')}
            </button>
            {onDerive && allowFreeDerive && !readOnly && (
              <button
                data-testid="agent-derive"
                onClick={() => derive(false)}
                disabled={deriving}
                title={t('settings.deriveTitle')}
                className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
              >
                <GitBranch size={14} /> {deriving ? t('settings.deriving') : t('settings.derive')}
              </button>
            )}
            {onDuplicate && !locked && !isBuiltin && (
              <button
                data-testid="agent-duplicate"
                onClick={duplicate}
                disabled={duplicating}
                title={t('settings.cloneTitle')}
                className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
              >
                <Copy size={14} /> {duplicating ? t('settings.cloning') : t('settings.clone')}
              </button>
            )}
            {onCancel && (
              <button
                data-testid="agent-cancel"
                onClick={onCancel}
                className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
              >
                {cancelLabel ?? t('settings.cancel')}
              </button>
            )}
            {onDelete && !locked && !isBuiltin && (
              <button
                data-testid="agent-delete"
                onClick={onDelete}
                className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
              >
                <Trash2 size={14} /> {t('settings.delete')}
              </button>
            )}
            {onRestoreDefault && (isChild || agent.locked) && (
              <button
                data-testid="agent-restore-default"
                onClick={onRestoreDefault}
                disabled={systemActionPending || (agent.overrides ?? []).length === 0}
                title={
                  agent.locked ? t('settings.resetBuiltinTitle') : t('settings.resetChildTitle')
                }
                className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] disabled:opacity-50"
              >
                <RotateCcw size={14} />{' '}
                {agent.locked ? t('settings.resetAll') : t('settings.inheritAll')}
              </button>
            )}
            {onToggleDisabled && !locked && !isBuiltin && (
              <button
                data-testid="agent-toggle-disabled"
                onClick={onToggleDisabled}
                disabled={systemActionPending}
                className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
              >
                <Power size={14} /> {agent.disabled ? t('settings.enable') : t('settings.disable')}
              </button>
            )}
            {!locked && (
              <Button data-testid="agent-save" onClick={save} disabled={saving}>
                {saving ? t('settings.saving') : t('settings.save')}
              </Button>
            )}
          </div>
        </div>
        {(lineage.length > 0 || inheritInfo) && (
          <div className="flex min-w-0 items-center gap-1">
            {lineage.length > 0 && (
              <AgentLineageChips lineage={lineage} self={preview} onSelectAgent={onSelectAgent} />
            )}
            {inheritInfo && <InfoPopover text={inheritInfo} />}
          </div>
        )}
      </div>

      <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
        {readOnly && readOnlyNote && (
          <div
            data-testid="agent-readonly-note"
            className="flex items-start gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]"
          >
            <Lock size={14} className="mt-0.5 shrink-0" />
            <p>{readOnlyNote}</p>
          </div>
        )}
        {agent.locked && (
          <div
            data-testid="agent-locked-note"
            className="flex items-start gap-2 rounded-md border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]"
          >
            <Lock size={14} className="mt-0.5 shrink-0" />
            <p>{t('settings.builtinNote', { role: agent.systemKey })}</p>
          </div>
        )}

        <fieldset
          disabled={locked}
          className={`m-0 min-w-0 space-y-4 border-0 p-0 ${locked ? 'opacity-80' : ''}`}
        >
          <Field label={t('settings.fields.name')}>
            <div className="flex items-center gap-2">
              {/* Icon-only emoji picker sits to the left of the name input. */}
              <EmojiField
                value={avatar}
                onChange={(v) => {
                  setAvatar(v)
                  mark('avatar')
                }}
                compact
              />
              <input
                data-testid="agent-name-input"
                value={name}
                onChange={(e) => setName(e.target.value)}
                className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)] disabled:opacity-70"
              />
            </div>
            {isChild && (
              <div className="flex items-center gap-2 pt-1">
                <span className="text-[10px] text-[var(--color-text-dim)]">
                  {t('settings.fields.avatar')}
                </span>
                {badge('avatar')}
              </div>
            )}
          </Field>

          {libraryParent && (
            <p className="text-xs text-[var(--color-text-dim)]">{t('settings.libraryParent')}</p>
          )}
          {!agent.system && !libraryParent && (parentOptions.length > 0 || isChild) && (
            <Field label={t('settings.fields.parent')} info={t('settings.parentHelp')}>
              <select
                data-testid="agent-parent-select"
                value={agent.parentId ?? ''}
                onChange={(e) => changeParent(e.target.value)}
                className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
              >
                <option value="">{t('settings.noParent')}</option>
                {parentOptions.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.locked
                      ? t('settings.parentOptionBuiltin', { name: p.name, id: p.id })
                      : t('settings.parentOption', { name: p.name, id: p.id })}
                  </option>
                ))}
              </select>
            </Field>
          )}

          <Field label={t('settings.fields.color')} trailing={badge('color')}>
            <div className="flex flex-wrap items-center gap-1.5">
              <button
                data-testid="agent-color"
                data-color=""
                onClick={() => {
                  setColor('')
                  mark('color')
                }}
                className={`flex h-7 w-7 items-center justify-center rounded-full border text-[10px] ${
                  color === ''
                    ? 'border-[var(--color-accent)] text-[var(--color-text)]'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
                }`}
                title={t('settings.autoColor')}
                style={{ background: color === '' ? resolveColor(preview) : undefined }}
              >
                {color === '' ? '' : 'A'}
              </button>
              {AVATAR_COLORS.map((c) => (
                <button
                  key={c}
                  data-testid="agent-color"
                  data-color={c}
                  onClick={() => {
                    setColor(c)
                    mark('color')
                  }}
                  className={`h-7 w-7 rounded-full ring-offset-2 ring-offset-[var(--color-surface)] ${
                    color === c ? 'ring-2 ring-[var(--color-accent)]' : ''
                  }`}
                  style={{ background: c }}
                  title={c}
                />
              ))}
            </div>
          </Field>

          {isChild && (
            <div className="flex items-center gap-3 text-[10px] text-[var(--color-text-dim)]">
              <span>{t('settings.fields.provider')}</span>
              {badge('provider')}
              <span>{t('settings.fields.model')}</span>
              {badge('model')}
            </div>
          )}
          <ProviderInstanceModelSelect
            providerInstanceId={providerInstanceId}
            model={model}
            onChange={(kindId, instanceId, m) => {
              if (instanceId !== providerInstanceId) mark('provider')
              if (m !== model) mark('model')
              setProvider(kindId)
              setProviderInstanceId(instanceId)
              setModel(m)
            }}
          />
          {catalog.find((c) => c.id === provider)?.appliesToolHooks === false && (
            <p
              data-testid="agent-hooks-not-applied-warning"
              className="-mt-2 rounded-md border border-[color-mix(in_srgb,var(--color-warning)_45%,transparent)] bg-[color-mix(in_srgb,var(--color-warning)_14%,var(--color-surface))] px-2 py-1.5 text-xs text-[var(--color-text-dim)]"
            >
              {t('settings.hooksWarning')}
            </p>
          )}

          <OptionField
            label={t('settings.fields.thinking')}
            info={
              <>
                {t('settings.thinkingHelp')}
                {provider === 'claude-cli' && thinkingLevel === 'off' && (
                  <span className="mt-2 block">⚡ {t('settings.thinkingOffHelp')}</span>
                )}
              </>
            }
            trailing={badge('thinkingLevel')}
          >
            <OptionPills
              value={thinkingLevel}
              onChange={(v) => {
                setThinkingLevel(v)
                mark('thinkingLevel')
              }}
              options={thinkingOptions}
              ariaLabel={t('settings.fields.thinking')}
              testid="agent-thinking-level"
            />
          </OptionField>

          <OptionField
            label={t('settings.fields.permission')}
            info={t('settings.permissionHelp')}
            trailing={badge('permissionMode')}
          >
            <OptionPills
              value={permissionMode}
              onChange={(v) => {
                setPermissionMode(v)
                mark('permissionMode')
              }}
              options={PERMISSION_OPTIONS}
              ariaLabel={t('settings.fields.permission')}
              testid="agent-permission-mode"
            />
          </OptionField>

          <OptionField
            label={t('settings.fields.nativeWebSearch')}
            info={t('settings.nativeWebSearchHelp')}
            trailing={badge('nativeWebSearch')}
          >
            <OptionPills
              value={nativeWebSearch ? 'on' : 'off'}
              onChange={(value) => {
                setNativeWebSearch(booleanFromOption(value))
                mark('nativeWebSearch')
              }}
              options={BOOLEAN_OPTIONS}
              ariaLabel={t('settings.fields.nativeWebSearch')}
              testid="agent-native-web-search"
            />
          </OptionField>

          <OptionField
            label={t('settings.fields.nativeShell')}
            info={t('settings.nativeShellHelp')}
            trailing={badge('nativeShell')}
          >
            <OptionPills
              value={nativeShell ? 'on' : 'off'}
              onChange={(value) => {
                setNativeShell(booleanFromOption(value))
                mark('nativeShell')
              }}
              options={BOOLEAN_OPTIONS}
              ariaLabel={t('settings.fields.nativeShell')}
              testid="agent-native-shell"
            />
          </OptionField>

          <OptionField
            label={t('settings.fields.coordinator')}
            info={t('settings.coordinatorHelp')}
            trailing={badge('coordinatorMode')}
          >
            <OptionPills
              value={coordinatorMode ? 'on' : 'off'}
              onChange={(value) => {
                setCoordinatorMode(booleanFromOption(value))
                mark('coordinatorMode')
              }}
              options={BOOLEAN_OPTIONS}
              ariaLabel={t('settings.fields.coordinator')}
              testid="agent-coordinator-mode"
            />
          </OptionField>
          {coordinatorMode && (
            <Field label={t('settings.fields.workflow')} trailing={badge('coordinatorWorkflow')}>
              <CoordinatorWorkflowPicker
                value={coordinatorWorkflow}
                onChange={(v) => {
                  setCoordinatorWorkflow(v)
                  mark('coordinatorWorkflow')
                }}
                disabled={saving || locked}
                groupName={`agent-recipe-${agent.id}`}
              />
            </Field>
          )}
          {coordinatorMode && (
            <>
              <Field
                label={t('settings.fields.coordinatorPrompt')}
                info={t('settings.coordinatorPromptHelp')}
                trailing={badge('coordinatorPrompt')}
              >
                <PromptEditor
                  data-testid="agent-coordinator-prompt-textarea"
                  value={coordinatorPrompt}
                  onChange={(v) => {
                    setCoordinatorPrompt(v)
                    mark('coordinatorPrompt')
                  }}
                  rows={3}
                />
              </Field>
            </>
          )}

          <Field
            label={t('settings.fields.soul')}
            trailing={
              <>
                {badge('soul')}
                {/* A system agent can always go back to the prompt shipped in
                    the binary for its role — the customisation keeps its other
                    settings, only the text is restored (staged, not saved). */}
                {agent.systemKey && !locked && (
                  <BuiltinPromptRevert
                    agentId={agent.id}
                    current={soul}
                    disabled={saving}
                    onError={setErr}
                    onRevert={(v) => {
                      setSoul(v)
                      mark('soul')
                    }}
                  />
                )}
              </>
            }
          >
            <PromptEditor
              data-testid="agent-soul-textarea"
              value={soul}
              onChange={(v) => {
                setSoul(v)
                mark('soul')
              }}
              rows={4}
            />
          </Field>

          <Field label={t('settings.fields.identity')} trailing={badge('identity')}>
            <PromptEditor
              data-testid="agent-identity-textarea"
              value={identity}
              onChange={(v) => {
                setIdentity(v)
                mark('identity')
              }}
              rows={2}
            />
          </Field>

          {isChild && (
            <div className="flex items-center gap-2 text-[10px] text-[var(--color-text-dim)]">
              <span>{t('settings.fields.skills')}</span>
              {badge('skills')}
            </div>
          )}
          <AgentSkillsSection
            selected={skills}
            onChange={(v) => {
              setSkills(v)
              mark('skills')
            }}
            onError={setErr}
          />

          <div className="border-t border-[var(--color-border)] pt-4">
            <div className="mb-3 flex items-center gap-2">
              <h3 className="flex items-center gap-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
                {t('settings.toolsTitle')}
                <InfoPopover
                  text={t('settings.toolsHelp', {
                    inheritance: isChild ? t('settings.toolsInheritance') : '',
                  })}
                  label={t('settings.toolsTitle')}
                />
              </h3>
              {isChild && (
                <FieldOverrideBadge
                  field="tools"
                  overridden={hasOverride(agent, 'tools') || hasOverride(agent, 'allowedTools')}
                  parentName={parentName}
                  onReset={resetTools}
                  disabled={saving}
                />
              )}
            </div>
            <AgentToolsSection
              key={toolsKey}
              agentId={agent.id}
              onError={setErr}
              locked={locked || !!agent.systemKey?.startsWith('subagent-')}
            />
          </div>
        </fieldset>

        {err && <p className="text-xs text-[var(--color-danger)]">{err}</p>}
        {saveWarn && (
          <p className="rounded-md border border-[color-mix(in_srgb,var(--color-accent)_25%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-2 py-1.5 text-xs text-[var(--color-text-dim)]">
            {t('settings.saveWarning', { warning: saveWarn })}
          </p>
        )}
      </div>

      {previewOpen && (
        <AgentContextModal
          agentId={agent.id}
          agentName={name || agent.name}
          onClose={() => setPreviewOpen(false)}
        />
      )}
    </div>
  )
}

// info is the field's explanation: shown under the control when short, behind an
// (i) next to the label when long.
function Field({
  label,
  info,
  trailing,
  children,
}: {
  label: string
  info?: React.ReactNode
  trailing?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <label className="block space-y-1">
      <span className="flex items-center gap-2 text-xs font-medium text-[var(--color-text-dim)]">
        {label}
        {info && <InfoPopover text={info} label={label} mode="long" />}
        {trailing}
      </span>
      {children}
      <FieldHint text={info} className="text-[11px]" />
    </label>
  )
}

function OptionField({
  label,
  info,
  trailing,
  children,
}: {
  label: string
  info?: React.ReactNode
  trailing?: React.ReactNode
  children: React.ReactNode
}) {
  return (
    <fieldset className="block min-w-0 space-y-1">
      <legend className="flex w-full items-center gap-2 text-xs font-medium text-[var(--color-text-dim)]">
        {label}
        {info && <InfoPopover text={info} label={label} mode="long" />}
        {trailing}
      </legend>
      {children}
      <FieldHint text={info} className="text-[11px]" />
    </fieldset>
  )
}

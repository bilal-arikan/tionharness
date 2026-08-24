import { useMemo, useState } from 'react'
import { Eye, Trash2, Star, Copy } from 'lucide-react'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import type { View } from '@/app/NavRail'
import type { Agent, AgentPatch } from '@/types'
import { AVATAR_COLORS, normalizeAvatar, resolveColor } from '@/shared/lib/avatar'
import { AgentAvatar } from '@/shared/components/agents/AgentAvatar'
import { EmojiField } from '@/shared/components/EmojiField'
import { ProviderInstanceModelSelect } from '@/shared/components/agents/ProviderInstanceModelSelect'
import { AgentToolsSection } from './AgentToolsSection'
import { AgentSkillsSection } from './AgentSkillsSection'
import { AgentContextModal } from './AgentContextModal'
import { Button, PromptEditor } from '@/shared/components'
import { OptionPills } from '@/shared/components/OptionPills'
import { CoordinatorWorkflowPicker } from '@/shared/components/CoordinatorWorkflowPicker'
import { useCatalog, thinkingInfoForModel, thinkingTierDisabledReason } from '@/shared/lib/catalog'
import { THINKING_OPTIONS, PERMISSION_OPTIONS } from './agentOptions'

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
  /** Clone the agent: when set, a "Klonla" button is shown in the header. The
   * clone carries over every setting (profile + provider/model + tools + skills).
   * Resolves once the clone exists (the parent then selects it). */
  onDuplicate?: () => Promise<string | undefined>
  /** Whether this agent is the default for new chats. Drives the header star
   * toggle's filled/active state. */
  isDefault?: boolean
  /** Make this agent the default for new chats. When set, a star toggle is shown
   * in the header (mirrors the roster's ★/☆ button). Takes effect immediately —
   * independent of the form's Save. */
  onSetDefault?: () => void
  /** When set, unsaved edits are surfaced on this nav view's dirty indicator.
   * The AgentsView page passes "agents"; the modal reuse leaves it unset so it
   * does not flag the nav from an unrelated screen. */
  dirtyView?: View
}

// AgentSettingsForm is the editable agent profile (visual identity + core
// fields). It is reused both inside the modal (roster gear) and as the right
// pane of the two-panel Agents view. Mount with a key={agent.id} so switching
// the selected agent resets the field state.
export function AgentSettingsForm({
  agent,
  onSave,
  onSaved,
  onCancel,
  cancelLabel = 'İptal',
  onDelete,
  onDuplicate,
  isDefault,
  onSetDefault,
  dirtyView,
}: Props) {
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
  const [thinkingLevel, setThinkingLevel] = useState(agent.thinkingLevel ?? '')
  const [permissionMode, setPermissionMode] = useState(agent.permissionMode || 'auto')
  const [skills, setSkills] = useState<string[]>(agent.skills ?? [])
  const [coordinatorMode, setCoordinatorMode] = useState(agent.coordinatorMode ?? false)
  const [coordinatorWorkflow, setCoordinatorWorkflow] = useState(agent.coordinatorWorkflow ?? '')
  const [coordinatorPrompt, setCoordinatorPrompt] = useState(agent.coordinatorPrompt ?? '')
  const [saving, setSaving] = useState(false)
  const [duplicating, setDuplicating] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState(0)
  const [previewOpen, setPreviewOpen] = useState(false)
  // P1.3: warning from the backend (e.g. "model not in price table"). Cleared on
  // the next save or when the model/provider changes.
  const [saveWarn, setSaveWarn] = useState<string | null>(null)

  // Reasoning tiers + class for the chosen provider+model. Every pill stays
  // visible; no-op levels are shown greyed with a reason (e.g. "Kapalı" on the
  // always-on Fable class, "Çok yüksek"/"Maks" on legacy models that clamp them,
  // or every level but off on a non-thinking model). tiers null = unknown/custom
  // → all enabled. The pill list uses '' for off (backend token "off"); the
  // currently-stored level stays selectable even if outside the set.
  const catalog = useCatalog()
  const thinkingOptions = useMemo(() => {
    const { tiers, cls } = thinkingInfoForModel(catalog, provider, model)
    if (tiers == null) return THINKING_OPTIONS
    return THINKING_OPTIONS.map((o) => {
      const token = o.value === '' ? 'off' : o.value
      const supported = o.value === thinkingLevel || tiers.includes(token)
      return supported ? o : { ...o, disabled: true, hint: thinkingTierDisabledReason(cls, token) }
    })
  }, [catalog, provider, model, thinkingLevel])

  // Unsaved-edits flag: current form fields vs the agent's persisted values.
  // (The tools section saves instantly on its own, so it is not part of this.)
  const dirty = useMemo(
    () =>
      name !== agent.name ||
      avatar !== (normalizeAvatar(agent.avatar) ?? '') ||
      color !== (agent.color ?? '') ||
      soul !== (agent.soul ?? '') ||
      identity !== (agent.identity ?? '') ||
      providerInstanceId !== (agent.providerInstanceId || agent.provider) ||
      model !== (agent.model ?? '') ||
      thinkingLevel !== (agent.thinkingLevel ?? '') ||
      permissionMode !== (agent.permissionMode || 'auto') ||
      coordinatorMode !== (agent.coordinatorMode ?? false) ||
      coordinatorWorkflow !== (agent.coordinatorWorkflow ?? '') ||
      coordinatorPrompt !== (agent.coordinatorPrompt ?? '') ||
      JSON.stringify(skills) !== JSON.stringify(agent.skills ?? []),
    [
      name,
      avatar,
      color,
      soul,
      identity,
      providerInstanceId,
      model,
      thinkingLevel,
      permissionMode,
      coordinatorMode,
      coordinatorWorkflow,
      coordinatorPrompt,
      skills,
      agent,
    ],
  )
  // Surface unsaved agent edits on the nav "Ajanlar" item (page reuse only).
  useRegisterDirty(dirtyView, dirty)

  const preview: Pick<Agent, 'id' | 'name' | 'avatar' | 'color'> = {
    id: agent.id,
    name: name || agent.name,
    avatar,
    color,
  }

  const save = async () => {
    if (!name.trim()) {
      setErr('Ajan adı boş olamaz.')
      return
    }
    setSaving(true)
    setErr(null)
    setSaveWarn(null)
    try {
      const result = await onSave({
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
      if (result?.warning) setSaveWarn(result.warning)
      setSavedAt((n) => n + 1)
      onSaved?.()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSaving(false)
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

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Header with live avatar preview + actions (delete / save). */}
      <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-4">
        <AgentAvatar agent={preview} size={44} />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <h2 className="truncate text-sm font-semibold text-[var(--color-text)]">
              {name || 'Ajan'}
            </h2>
            <span
              title="Ajan ID — disk klasörünün adı"
              className="shrink-0 rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 font-mono text-[11px] text-[var(--color-text-dim)]"
            >
              {agent.id}
            </span>
          </div>
          <p className="text-xs text-[var(--color-text-dim)]">Ajan ayarları</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {savedAt > 0 && !saving && (
            <span className="text-xs text-[var(--color-text-dim)]">Kaydedildi ✓</span>
          )}
          {onSetDefault && (
            <button
              data-testid="agent-set-default-detail"
              onClick={() => {
                if (!isDefault) onSetDefault()
              }}
              disabled={isDefault}
              title={
                isDefault
                  ? 'Bu ajan yeni sohbetler için varsayılan'
                  : 'Yeni sohbetler için varsayılan yap'
              }
              className={`flex items-center gap-1.5 rounded px-3 py-1.5 text-sm transition ${
                isDefault
                  ? 'text-[var(--color-accent)]'
                  : 'text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]'
              }`}
            >
              <Star size={14} className={isDefault ? 'fill-current' : ''} />
              {isDefault ? 'Varsayılan' : 'Varsayılan yap'}
            </button>
          )}
          <button
            data-testid="agent-preview-context"
            onClick={() => setPreviewOpen(true)}
            title="Ajanın sıfırdan aldığı bağlamı (sistem promptu + araçlar) önizle"
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <Eye size={14} /> Bağlam
          </button>
          {onDuplicate && (
            <button
              data-testid="agent-duplicate"
              onClick={duplicate}
              disabled={duplicating}
              title="Bu ajanın tüm ayarlarıyla (sağlayıcı/model, araçlar, yetenekler) bir klonunu oluştur"
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] disabled:opacity-50"
            >
              <Copy size={14} /> {duplicating ? 'Klonlanıyor…' : 'Klonla'}
            </button>
          )}
          {onCancel && (
            <button
              data-testid="agent-cancel"
              onClick={onCancel}
              className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            >
              {cancelLabel}
            </button>
          )}
          {onDelete && (
            <button
              data-testid="agent-delete"
              onClick={onDelete}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
            >
              <Trash2 size={14} /> Sil
            </button>
          )}
          <Button data-testid="agent-save" onClick={save} disabled={saving}>
            {saving ? 'Kaydediliyor…' : 'Kaydet'}
          </Button>
        </div>
      </div>

      <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
        <Field label="Ad">
          <div className="flex items-center gap-2">
            {/* Icon-only emoji picker sits to the left of the name input. */}
            <EmojiField value={avatar} onChange={setAvatar} compact />
            <input
              data-testid="agent-name-input"
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="min-w-0 flex-1 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </div>
        </Field>

        <Field label="Renk">
          <div className="flex flex-wrap items-center gap-1.5">
            <button
              data-testid="agent-color"
              data-color=""
              onClick={() => setColor('')}
              className={`flex h-7 w-7 items-center justify-center rounded-full border text-[10px] ${
                color === ''
                  ? 'border-[var(--color-accent)] text-[var(--color-text)]'
                  : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
              }`}
              title="Otomatik (id'den türet)"
              style={{ background: color === '' ? resolveColor(preview) : undefined }}
            >
              {color === '' ? '' : 'A'}
            </button>
            {AVATAR_COLORS.map((c) => (
              <button
                key={c}
                data-testid="agent-color"
                data-color={c}
                onClick={() => setColor(c)}
                className={`h-7 w-7 rounded-full ring-offset-2 ring-offset-[var(--color-surface)] ${
                  color === c ? 'ring-2 ring-[var(--color-accent)]' : ''
                }`}
                style={{ background: c }}
                title={c}
              />
            ))}
          </div>
        </Field>

        <ProviderInstanceModelSelect
          providerInstanceId={providerInstanceId}
          model={model}
          onChange={(kindId, instanceId, m) => {
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
            ⚠️ <strong>codex-cli</strong>'da PreToolUse/PostToolUse hook'ların hiçbiri çalışmaz — ne
            codex'in kendi shell/apply_patch araçları ne de MCP köprüsüyle çağrılan TionHarness
            araçları için. Bu yüzden <code>sqz</code> gibi PostToolUse token-optimizer sıkıştırması
            da bu ajanda devre dışıdır. Sebep: codex kendi araç döngüsünü ayrı bir alt süreçte
            koşturur ve hook aktarımı sunmaz.
          </p>
        )}

        <Field label="Düşünme (thinking) seviyesi">
          <OptionPills
            value={thinkingLevel}
            onChange={setThinkingLevel}
            options={thinkingOptions}
            ariaLabel="Düşünme seviyesi"
            testid="agent-thinking-level"
          />
        </Field>
        <p className="-mt-2 text-xs text-[var(--color-text-dim)]">
          Uzatılmış akıl yürütme <strong>anthropic</strong> sağlayıcıda araçsız sohbette etkilidir.{' '}
          <strong>claude-cli</strong>'da seviye alt sürece geçer: seçilen seviye CLI{' '}
          <code>effortLevel</code>'ına eşlenir (<strong>Yüksek+ / Maks</strong> derin-çalışma
          tiyerleri artık gerçekten CLI'ye ulaşır). <strong>Maks</strong>, settings.json'ın
          reddettiği tek değer olduğu için <code>CLAUDE_CODE_EFFORT_LEVEL=max</code> env
          değişkeniyle uygulanır; bu tiyerlerde thinking açık kaldığından paralel araç batch'i
          kapanır ("think XOR batch"). <strong>Kapalı</strong> thinking'i tamamen kapatır (
          <code>MAX_THINKING_TOKENS=0</code>).
        </p>
        {provider === 'claude-cli' && !thinkingLevel && (
          <p className="-mt-1 rounded-md border border-[color-mix(in_srgb,var(--color-accent)_25%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-2 py-1 text-xs text-[var(--color-text-dim)]">
            ⚡ <strong>Kapalı + claude-cli:</strong> Claude Code ≥2.1.203 thinking açıkken paralel
            araç çağrısı yapmaz ("think XOR batch"). Bu seçimle thinking kapanır ve paralel
            batch'ler (tek istekte N araç) geri gelir — daha hızlı ve belirgin şekilde daha ucuz;
            bedeli derin akıl yürütmenin olmaması.
          </p>
        )}

        <Field label="İzin modu (araç kullanımı)">
          <OptionPills
            value={permissionMode}
            onChange={setPermissionMode}
            options={PERMISSION_OPTIONS}
            ariaLabel="İzin modu"
            testid="agent-permission-mode"
          />
        </Field>
        <p className="-mt-2 text-xs text-[var(--color-text-dim)]">
          <strong>Salt-okunur</strong> yalnız okuma araçlarına izin verir. <strong>Sor</strong>{' '}
          modunda yazma/komut araçları için sohbette onay penceresi çıkar (Allow once / Always allow
          / Deny); onay verecek kimse yoksa (otonom koşu) reddedilir. claude-cli ajanlarında bu mod
          CLI izin bayrağına çevrilir (salt-okunur→<code>plan</code>, sor→<code>acceptEdits</code>,
          otomatik→<code>bypass</code>).
        </p>

        <Field label="Koordinatör">
          <label className="flex cursor-pointer items-start gap-2 text-sm text-[var(--color-text)]">
            <input
              type="checkbox"
              data-testid="agent-coordinator-mode"
              checked={coordinatorMode}
              onChange={(e) => setCoordinatorMode(e.target.checked)}
              className="mt-0.5 accent-[var(--color-accent)]"
            />
            <span>
              Bu ajanın açtığı <strong>yeni</strong> oturumlar koordinatör olarak başlasın
            </span>
          </label>
        </Field>
        <p className="-mt-2 text-xs text-[var(--color-text-dim)]">
          Açıkken ajan her yeni oturumda koordinatör el kitabını ve <code>spawn_worker</code> /{' '}
          <code>send_to_worker</code> / <code>stop_worker</code> / <code>list_workers</code>{' '}
          araçlarını hazır bulur — oturum başına elle açman gerekmez. Bu bir{' '}
          <strong>varsayılan</strong>: <em>mevcut</em> oturumlar etkilenmez, ve ajan tek-iş moduna
          dönerken kendi oturumunun modunu <code>set_coordinator_mode</code> ile kapatabilir.
        </p>
        {coordinatorMode && (
          <Field label="Varsayılan koordinasyon reçetesi">
            <CoordinatorWorkflowPicker
              value={coordinatorWorkflow}
              onChange={setCoordinatorWorkflow}
              disabled={saving}
              groupName={`agent-recipe-${agent.id}`}
            />
          </Field>
        )}
        {coordinatorMode && (
          <>
            <Field label="Koordinatör promptu">
              <PromptEditor
                data-testid="agent-coordinator-prompt-textarea"
                value={coordinatorPrompt}
                onChange={setCoordinatorPrompt}
                rows={3}
              />
            </Field>
            <p className="-mt-2 text-xs text-[var(--color-text-dim)]">
              Yalnızca oturum <strong>koordinatör modundayken</strong>, ortak koordinatör el
              kitabının hemen ardından sistem bağlamına eklenir. Bu ajana özel delegasyon yönergesi
              (hangi worker'lar açılsın, iş nasıl bölünsün) buraya yazılır — soul'a değil: mod
              kapalıyken hiç enjekte edilmez, dolayısıyla <strong>sıfır token</strong> maliyeti
              olur.
            </p>
          </>
        )}

        <Field label="Karakter / sistem promptu (soul)">
          <PromptEditor
            data-testid="agent-soul-textarea"
            value={soul}
            onChange={setSoul}
            rows={4}
          />
        </Field>

        <Field label="Kimlik (identity)">
          <PromptEditor
            data-testid="agent-identity-textarea"
            value={identity}
            onChange={setIdentity}
            rows={2}
          />
        </Field>

        <AgentSkillsSection selected={skills} onChange={setSkills} onError={setErr} />

        <div className="border-t border-[var(--color-border)] pt-4">
          <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Yasaklı Araçlar
          </h3>
          <p className="mb-3 text-xs text-[var(--color-text-dim)]">
            Bu ajan varsayılan olarak <strong>tüm</strong> workspace-aktif araçlara erişir. Burada
            yalnızca <strong>kullanmasını istemediğin</strong> araçları yasakla. Yalnız workspace'te
            aktif araçlar listelenir (aktivasyon <strong>Araçlar</strong> ekranından). Değişiklikler
            anında kaydedilir.
          </p>
          <AgentToolsSection agentId={agent.id} onError={setErr} />
        </div>

        {err && <p className="text-xs text-[var(--color-danger)]">{err}</p>}
        {saveWarn && (
          <p className="rounded-md border border-[color-mix(in_srgb,var(--color-accent)_25%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] px-2 py-1.5 text-xs text-[var(--color-text-dim)]">
            ⚠️ {saveWarn}
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

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <label className="block space-y-1">
      <span className="text-xs font-medium text-[var(--color-text-dim)]">{label}</span>
      {children}
    </label>
  )
}

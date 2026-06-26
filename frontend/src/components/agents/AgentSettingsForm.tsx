import { useState } from 'react'
import { Eye, Trash2, FolderOpen, ClipboardCopy, Check } from 'lucide-react'
import { api } from '../../api'
import type { Agent, AgentPatch } from '../../types'
import { AVATAR_COLORS, normalizeAvatar, resolveColor } from '../../lib/avatar'
import { AgentAvatar } from './AgentAvatar'
import { EmojiField } from '../common/EmojiField'
import { ProviderModelSelect } from './ProviderModelSelect'
import { AgentToolsSection } from './AgentToolsSection'
import { AgentSkillsSection } from './AgentSkillsSection'
import { AgentContextModal } from './AgentContextModal'
import { Button } from '../common'
import { OptionPills } from '../common/OptionPills'
import { PLANNING_OPTIONS, THINKING_OPTIONS, PERMISSION_OPTIONS } from './agentOptions'

interface Props {
  agent: Agent
  onSave: (patch: AgentPatch) => Promise<void>
  /** Called after a successful save (e.g. close the modal). */
  onSaved?: () => void
  /** Secondary action button (e.g. Cancel in the modal). */
  onCancel?: () => void
  cancelLabel?: string
  /** Danger action: when set, a "Sil" button is shown at the footer-left. */
  onDelete?: () => void
}

// AgentSettingsForm is the editable agent profile (visual identity + core
// fields). It is reused both inside the modal (roster gear) and as the right
// pane of the two-panel Agents view. Mount with a key={agent.id} so switching
// the selected agent resets the field state.
export function AgentSettingsForm({ agent, onSave, onSaved, onCancel, cancelLabel = 'İptal', onDelete }: Props) {
  const [name, setName] = useState(agent.name)
  // Seed with a normalized avatar so an existing mojibake value is repaired on
  // open and persisted clean when the form is saved.
  const [avatar, setAvatar] = useState(normalizeAvatar(agent.avatar) ?? '')
  const [color, setColor] = useState(agent.color ?? '')
  const [soul, setSoul] = useState(agent.soul ?? '')
  const [identity, setIdentity] = useState(agent.identity ?? '')
  const [provider, setProvider] = useState(agent.provider)
  const [model, setModel] = useState(agent.model ?? '')
  const [planningMode, setPlanningMode] = useState(agent.planningMode || 'standard')
  const [thinkingLevel, setThinkingLevel] = useState(agent.thinkingLevel ?? '')
  const [permissionMode, setPermissionMode] = useState(agent.permissionMode || 'auto')
  const [skills, setSkills] = useState<string[]>(agent.skills ?? [])
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)
  const [savedAt, setSavedAt] = useState(0)
  const [previewOpen, setPreviewOpen] = useState(false)
  const [copiedPath, setCopiedPath] = useState(false)

  // Copy the agent's on-disk JSON file path to the clipboard.
  const copyPath = async () => {
    try {
      const { path } = await api.agentPath(agent.id)
      await navigator.clipboard?.writeText(path)
      setCopiedPath(true)
      setTimeout(() => setCopiedPath(false), 1500)
    } catch (e) {
      setErr((e as Error).message)
    }
  }

  // Open the folder holding the agent's JSON file in the OS file manager.
  const revealFolder = async () => {
    try {
      await api.revealAgent(agent.id)
    } catch (e) {
      setErr((e as Error).message)
    }
  }

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
    try {
      await onSave({
        name: name.trim(),
        avatar,
        color,
        soul,
        identity,
        provider,
        model: model.trim(),
        planningMode,
        thinkingLevel,
        permissionMode,
        skills,
      })
      setSavedAt((n) => n + 1)
      onSaved?.()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex h-full flex-col overflow-hidden">
      {/* Header with live avatar preview + actions (delete / save). */}
      <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-4">
        <AgentAvatar agent={preview} size={44} />
        <div className="min-w-0 flex-1">
          <div className="flex items-baseline gap-2">
            <h2 className="truncate text-sm font-semibold text-[var(--color-text)]">{name || 'Ajan'}</h2>
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
          <button
            data-testid="agent-preview-context"
            onClick={() => setPreviewOpen(true)}
            title="Ajanın sıfırdan aldığı bağlamı (sistem promptu + araçlar) önizle"
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <Eye size={14} /> Bağlam
          </button>
          <button
            data-testid="agent-copy-path"
            onClick={copyPath}
            title="Ajanın disk üzerindeki JSON dosya yolunu kopyala"
            className="flex items-center gap-1.5 rounded px-2 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            {copiedPath ? <Check size={14} /> : <ClipboardCopy size={14} />}
            {copiedPath ? 'Kopyalandı' : 'Yolu kopyala'}
          </button>
          <button
            data-testid="agent-reveal-folder"
            onClick={revealFolder}
            title="Ajanın JSON dosyasının bulunduğu klasörü dosya yöneticisinde aç"
            className="flex items-center gap-1.5 rounded px-2 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <FolderOpen size={14} /> Klasörü aç
          </button>
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
          <input
            data-testid="agent-name-input"
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <Field label="Görsel (emoji)">
          <EmojiField
            value={avatar}
            onChange={setAvatar}
            label={(v) => (v ? 'Emojiyi değiştir' : 'Emoji seç (varsayılan: baş harf)')}
          />
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

        <ProviderModelSelect
          provider={provider}
          model={model}
          onChange={(p, m) => {
            setProvider(p)
            setModel(m)
          }}
        />

        <Field label="Planlama modu">
          <OptionPills
            value={planningMode}
            onChange={setPlanningMode}
            options={PLANNING_OPTIONS}
            ariaLabel="Planlama modu"
            testid="agent-planning-mode"
          />
        </Field>

        <Field label="Düşünme (thinking) seviyesi">
          <OptionPills
            value={thinkingLevel}
            onChange={setThinkingLevel}
            options={THINKING_OPTIONS}
            ariaLabel="Düşünme seviyesi"
            testid="agent-thinking-level"
          />
        </Field>
        <p className="-mt-2 text-xs text-[var(--color-text-dim)]">
          Uzatılmış akıl yürütme yalnız <strong>anthropic</strong> sağlayıcıda ve araçsız sohbette etkilidir.
        </p>

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
          <strong>Salt-okunur</strong> yalnız okuma araçlarına izin verir. <strong>Sor</strong> modunda yazma/komut
          araçları için sohbette onay penceresi çıkar (Allow once / Always allow / Deny); onay verecek kimse yoksa
          (otonom koşu) reddedilir. claude-cli ajanlarında bu mod CLI izin bayrağına çevrilir
          (salt-okunur→<code>plan</code>, sor→<code>acceptEdits</code>, otomatik→<code>bypass</code>).
        </p>

        <Field label="Karakter / sistem promptu (soul)">
          <textarea
            data-testid="agent-soul-textarea"
            value={soul}
            onChange={(e) => setSoul(e.target.value)}
            rows={4}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <Field label="Kimlik (identity)">
          <textarea
            data-testid="agent-identity-textarea"
            value={identity}
            onChange={(e) => setIdentity(e.target.value)}
            rows={2}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <AgentSkillsSection selected={skills} onChange={setSkills} onError={setErr} />

        <div className="border-t border-[var(--color-border)] pt-4">
          <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Yasaklı Araçlar
          </h3>
          <p className="mb-3 text-xs text-[var(--color-text-dim)]">
            Bu ajan varsayılan olarak <strong>tüm</strong> workspace-aktif araçlara erişir. Burada
            yalnızca <strong>kullanmasını istemediğin</strong> araçları yasakla. Yalnız workspace'te aktif
            araçlar listelenir (aktivasyon <strong>Araçlar</strong> ekranından). Değişiklikler anında kaydedilir.
          </p>
          <AgentToolsSection agentId={agent.id} onError={setErr} />
        </div>

        {err && <p className="text-xs text-[var(--color-danger)]">{err}</p>}
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

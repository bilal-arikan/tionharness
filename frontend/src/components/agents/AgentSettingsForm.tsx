import { useState } from 'react'
import { Eye, Trash2 } from 'lucide-react'
import type { Agent, AgentPatch } from '../../types'
import { AVATAR_COLORS, resolveColor } from '../../lib/avatar'
import { AgentAvatar } from './AgentAvatar'
import { EmojiPicker } from './EmojiPicker'
import { ProviderModelSelect } from './ProviderModelSelect'
import { AgentToolsSection } from './AgentToolsSection'
import { AgentSkillsSection } from './AgentSkillsSection'
import { AgentContextModal } from './AgentContextModal'

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
  const [avatar, setAvatar] = useState(agent.avatar ?? '')
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
  const [emojiPickerOpen, setEmojiPickerOpen] = useState(false)

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
          <h2 className="truncate text-sm font-semibold text-[var(--color-text)]">{name || 'Ajan'}</h2>
          <p className="text-xs text-[var(--color-text-dim)]">Ajan ayarları</p>
        </div>
        <div className="flex shrink-0 items-center gap-2">
          {savedAt > 0 && !saving && (
            <span className="text-xs text-[var(--color-text-dim)]">Kaydedildi ✓</span>
          )}
          <button
            onClick={() => setPreviewOpen(true)}
            title="Ajanın sıfırdan aldığı bağlamı (sistem promptu + araçlar) önizle"
            className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)]"
          >
            <Eye size={14} /> Bağlam
          </button>
          {onCancel && (
            <button
              onClick={onCancel}
              className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            >
              {cancelLabel}
            </button>
          )}
          {onDelete && (
            <button
              onClick={onDelete}
              className="flex items-center gap-1.5 rounded px-3 py-1.5 text-sm text-[var(--color-danger)] hover:bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]"
            >
              <Trash2 size={14} /> Sil
            </button>
          )}
          <button
            onClick={save}
            disabled={saving}
            className="rounded bg-[var(--color-accent)] px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {saving ? 'Kaydediliyor…' : 'Kaydet'}
          </button>
        </div>
      </div>

      <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
        <Field label="Ad">
          <input
            value={name}
            onChange={(e) => setName(e.target.value)}
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <Field label="Görsel (emoji)">
          <div className="relative inline-block">
            <button
              type="button"
              onClick={() => setEmojiPickerOpen((o) => !o)}
              className="flex items-center gap-2 rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 text-sm hover:border-[var(--color-accent)]"
              title="Emoji seç"
            >
              <span className="flex h-7 w-7 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-lg leading-none">
                {avatar || 'Aa'}
              </span>
              <span className="text-[var(--color-text-dim)]">
                {avatar ? 'Emojiyi değiştir' : 'Emoji seç (varsayılan: baş harf)'}
              </span>
            </button>
            {emojiPickerOpen && (
              <EmojiPicker
                value={avatar}
                onSelect={(emoji) => {
                  setAvatar(emoji)
                  setEmojiPickerOpen(false)
                }}
                onClose={() => setEmojiPickerOpen(false)}
              />
            )}
          </div>
        </Field>

        <Field label="Renk">
          <div className="flex flex-wrap items-center gap-1.5">
            <button
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

        <div className="grid grid-cols-2 gap-3">
          <Field label="Planlama modu">
            <select
              value={planningMode}
              onChange={(e) => setPlanningMode(e.target.value)}
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
            >
              <option value="standard">standard</option>
              <option value="deep">deep</option>
            </select>
          </Field>
          <Field label="Düşünme (thinking) seviyesi">
            <select
              value={thinkingLevel || 'off'}
              onChange={(e) => setThinkingLevel(e.target.value === 'off' ? '' : e.target.value)}
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
            >
              <option value="off">Kapalı</option>
              <option value="low">Düşük (~2K)</option>
              <option value="medium">Orta (~8K)</option>
              <option value="high">Yüksek (~16K)</option>
            </select>
          </Field>
        </div>
        <p className="-mt-2 text-xs text-[var(--color-text-dim)]">
          Uzatılmış akıl yürütme yalnız <strong>anthropic</strong> sağlayıcıda ve araçsız sohbette etkilidir.
        </p>

        <Field label="İzin modu (araç kullanımı)">
          <select
            value={permissionMode}
            onChange={(e) => setPermissionMode(e.target.value)}
            className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none"
          >
            <option value="auto">Otomatik — tüm araçlar onaysız çalışır</option>
            <option value="ask">Sor — dosya yazma/komut için onay iste</option>
            <option value="read-only">Salt-okunur — yazma/komut engellenir</option>
          </select>
        </Field>
        <p className="-mt-2 text-xs text-[var(--color-text-dim)]">
          <strong>Salt-okunur</strong> yalnız okuma araçlarına izin verir. <strong>Sor</strong> modunda yazma/komut
          araçları için sohbette onay penceresi çıkar (Allow once / Always allow / Deny); onay verecek kimse yoksa
          (otonom koşu) reddedilir. claude-cli ajanlarında bu mod CLI izin bayrağına çevrilir
          (salt-okunur→<code>plan</code>, sor→<code>acceptEdits</code>, otomatik→<code>bypass</code>).
        </p>

        <Field label="Karakter / sistem promptu (soul)">
          <textarea
            value={soul}
            onChange={(e) => setSoul(e.target.value)}
            rows={4}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <Field label="Kimlik (identity)">
          <textarea
            value={identity}
            onChange={(e) => setIdentity(e.target.value)}
            rows={2}
            className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
          />
        </Field>

        <AgentSkillsSection selected={skills} onChange={setSkills} onError={setErr} agentId={agent.id} />

        <div className="border-t border-[var(--color-border)] pt-4">
          <h3 className="mb-1 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
            Araçlar
          </h3>
          <p className="mb-3 text-xs text-[var(--color-text-dim)]">
            Bu ajanın kullanabileceği araçları seç. Yalnız workspace'te aktif olan araçlar listelenir;
            workspace aktivasyonu <strong>Araçlar</strong> ekranından yönetilir. (Değişiklikler anında kaydedilir.)
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

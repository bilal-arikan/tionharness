import { useState } from 'react'
import type { Agent, AgentPatch } from '../types'
import { AVATAR_COLORS, AVATAR_GLYPHS, resolveColor } from '../lib/avatar'
import { AgentAvatar } from './AgentAvatar'
import { ProviderModelSelect } from './ProviderModelSelect'

interface Props {
  agent: Agent
  onClose: () => void
  onSave: (patch: AgentPatch) => Promise<void>
}

// AgentSettingsModal is the per-agent editor opened from the roster's gear
// button. It edits the visual identity (emoji + color) and the core profile
// fields, then sends a partial patch to the backend.
export function AgentSettingsModal({ agent, onClose, onSave }: Props) {
  const [name, setName] = useState(agent.name)
  const [avatar, setAvatar] = useState(agent.avatar ?? '')
  const [color, setColor] = useState(agent.color ?? '')
  const [soul, setSoul] = useState(agent.soul ?? '')
  const [identity, setIdentity] = useState(agent.identity ?? '')
  const [provider, setProvider] = useState(agent.provider)
  const [model, setModel] = useState(agent.model ?? '')
  const [planningMode, setPlanningMode] = useState(agent.planningMode || 'standard')
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  // Live preview agent for the avatar at the top of the dialog.
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
      })
      onClose()
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4"
      onClick={onClose}
    >
      <div
        className="flex max-h-[88vh] w-full max-w-lg flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header with live avatar preview. */}
        <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-4">
          <AgentAvatar agent={preview} size={44} />
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold text-[var(--color-text)]">
              {name || 'Ajan'}
            </h2>
            <p className="text-xs text-[var(--color-text-dim)]">Ajan ayarları</p>
          </div>
          <button
            onClick={onClose}
            className="text-lg leading-none text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            title="Kapat"
          >
            ×
          </button>
        </div>

        <div className="space-y-4 overflow-y-auto px-5 py-4">
          {/* Name */}
          <Field label="Ad">
            <input
              value={name}
              onChange={(e) => setName(e.target.value)}
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </Field>

          {/* Emoji / glyph picker */}
          <Field label="Görsel (emoji)">
            <div className="flex flex-wrap gap-1.5">
              <button
                onClick={() => setAvatar('')}
                className={`flex h-8 w-8 items-center justify-center rounded-full border text-xs ${
                  avatar === ''
                    ? 'border-[var(--color-accent)] text-[var(--color-text)]'
                    : 'border-[var(--color-border)] text-[var(--color-text-dim)]'
                }`}
                title="Otomatik (baş harf)"
              >
                Aa
              </button>
              {AVATAR_GLYPHS.map((g) => (
                <button
                  key={g}
                  onClick={() => setAvatar(g)}
                  className={`flex h-8 w-8 items-center justify-center rounded-full border text-base ${
                    avatar === g
                      ? 'border-[var(--color-accent)]'
                      : 'border-[var(--color-border)] hover:border-[var(--color-accent)]'
                  }`}
                >
                  {g}
                </button>
              ))}
            </div>
          </Field>

          {/* Color picker */}
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

          {/* Provider + model (catalog-driven) */}
          <ProviderModelSelect
            provider={provider}
            model={model}
            onChange={(p, m) => {
              setProvider(p)
              setModel(m)
            }}
          />

          {/* Planning mode */}
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

          {/* Soul */}
          <Field label="Karakter / sistem promptu (soul)">
            <textarea
              value={soul}
              onChange={(e) => setSoul(e.target.value)}
              rows={4}
              className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </Field>

          {/* Identity */}
          <Field label="Kimlik (identity)">
            <textarea
              value={identity}
              onChange={(e) => setIdentity(e.target.value)}
              rows={2}
              className="w-full resize-y rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </Field>

          {err && <p className="text-xs text-red-500">{err}</p>}
        </div>

        {/* Footer */}
        <div className="flex justify-end gap-2 border-t border-[var(--color-border)] px-5 py-3">
          <button
            onClick={onClose}
            className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
          >
            İptal
          </button>
          <button
            onClick={save}
            disabled={saving}
            className="rounded bg-[var(--color-accent)] px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-50"
          >
            {saving ? 'Kaydediliyor…' : 'Kaydet'}
          </button>
        </div>
      </div>
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

// Per-workspace "Promptlar & Dosyalar" category: edit the workspace's config
// files (runtime prompts, instructions, README) that live under
// <workspace>/config/. Self-loading + self-saving (own Save button), since these
// files are written directly rather than through the app-settings patch flow.
import { useEffect, useMemo, useState } from 'react'
import { api } from '../../api'
import type { WorkspaceConfig, WorkspaceConfigPatch } from '../../types'
import { Field, inputCls } from './primitives'
import { CopyPathButton } from '../CopyPathButton'

interface Props {
  onError: (msg: string) => void
}

// Human labels for each runtime prompt key.
const PROMPT_LABELS: Record<string, { label: string; hint: string }> = {
  summary: { label: 'Özet promptu', hint: '/memory · /board · /flows komutlarının sistem promptu. Boş bırakırsan gömülü varsayılan kullanılır.' },
  reflect: { label: 'Yansıma promptu', hint: '/reflect (dream cycle) yansıma talimatı. Günlük kayıtları otomatik eklenir.' },
  title: { label: 'Başlık promptu', hint: 'Otomatik başlık üretimi sistem promptu.' },
}

type Draft = { prompts: Record<string, string>; instructions: string; readme: string }

function toDraft(c: WorkspaceConfig): Draft {
  return { prompts: { ...c.prompts }, instructions: c.instructions, readme: c.readme }
}

export function WorkspaceFilesPanel({ onError }: Props) {
  const [config, setConfig] = useState<WorkspaceConfig | null>(null)
  const [draft, setDraft] = useState<Draft | null>(null)
  const [original, setOriginal] = useState<Draft | null>(null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    api
      .getWorkspaceConfig()
      .then((c) => {
        setConfig(c)
        setDraft(toDraft(c))
        setOriginal(toDraft(c))
      })
      .catch((e) => onError((e as Error).message))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  const dirty = useMemo(
    () => !!draft && !!original && JSON.stringify(draft) !== JSON.stringify(original),
    [draft, original],
  )

  if (!config || !draft) {
    return <div className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</div>
  }

  const setPrompt = (key: string, val: string) =>
    setDraft((d) => (d ? { ...d, prompts: { ...d.prompts, [key]: val } } : d))

  const save = async () => {
    if (!original) return
    const patch: WorkspaceConfigPatch = {}
    const changedPrompts: Record<string, string> = {}
    for (const key of config.promptKeys) {
      if (draft.prompts[key] !== original.prompts[key]) changedPrompts[key] = draft.prompts[key]
    }
    if (Object.keys(changedPrompts).length) patch.prompts = changedPrompts
    if (draft.instructions !== original.instructions) patch.instructions = draft.instructions
    if (draft.readme !== original.readme) patch.readme = draft.readme

    setSaving(true)
    try {
      const updated = await api.updateWorkspaceConfig(patch)
      setConfig(updated)
      setDraft(toDraft(updated))
      setOriginal(toDraft(updated))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const reveal = () => api.revealWorkspaceConfig().catch((e) => onError((e as Error).message))

  return (
    <>
      <div className="flex items-center justify-between rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2">
        <span className="text-xs text-[var(--color-text-dim)]">
          Bu dosyalar <code className="rounded bg-[var(--color-bg)] px-1">{config.dir}</code> altında. Hem buradan hem doğrudan diskten düzenleyebilirsin.
        </span>
        <div className="flex shrink-0 items-center gap-1">
          <CopyPathButton path={config.dir} />
          <button
            onClick={reveal}
            className="shrink-0 rounded border border-[var(--color-border)] px-2 py-1 text-[11px] hover:border-[var(--color-accent)]"
          >
            📂 Klasörü aç
          </button>
        </div>
      </div>

      <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">Runtime promptları</div>
      {config.promptKeys.map((key) => {
        const meta = PROMPT_LABELS[key] ?? { label: key, hint: '' }
        const isDefault = draft.prompts[key].trim() === (config.defaults[key] ?? '').trim()
        return (
          <Field key={key} label={meta.label} hint={meta.hint}>
            <textarea
              value={draft.prompts[key]}
              onChange={(e) => setPrompt(key, e.target.value)}
              rows={4}
              className={`${inputCls} resize-y font-mono text-xs`}
            />
            <div className="mt-1 flex items-center gap-2">
              <button
                onClick={() => setPrompt(key, config.defaults[key] ?? '')}
                disabled={isDefault}
                className="rounded border border-[var(--color-border)] px-2 py-0.5 text-[11px] hover:border-[var(--color-accent)] disabled:opacity-30"
              >
                Varsayılana dön
              </button>
              {isDefault && <span className="text-[11px] text-[var(--color-text-dim)]">varsayılan</span>}
            </div>
          </Field>
        )
      })}

      <div className="pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">Workspace dosyaları</div>
      <Field label="Talimatlar (instructions.md)" hint="Bu workspace'teki tüm ajanlara eklenen yönergeler. 'Genel' sekmesindeki talimatlarla senkronizedir.">
        <textarea
          value={draft.instructions}
          onChange={(e) => setDraft((d) => (d ? { ...d, instructions: e.target.value } : d))}
          rows={4}
          className={`${inputCls} resize-y`}
          placeholder="Örn. Tüm cevapları Türkçe ver; commit at ama push'lama."
        />
      </Field>
      <Field label="Notlar (README.md)" hint="Bu workspace hakkında serbest notlar. Ajanlara enjekte edilmez.">
        <textarea
          value={draft.readme}
          onChange={(e) => setDraft((d) => (d ? { ...d, readme: e.target.value } : d))}
          rows={5}
          className={`${inputCls} resize-y font-mono text-xs`}
        />
      </Field>

      <div className="sticky bottom-0 flex items-center justify-end gap-3 border-t border-[var(--color-border)] bg-[var(--color-bg)] py-2">
        <span className="text-xs text-[var(--color-text-dim)]">{dirty ? 'Kaydedilmemiş değişiklik' : 'Kayıtlı'}</span>
        <button
          onClick={save}
          disabled={!dirty || saving}
          className="rounded bg-[var(--color-accent)] px-4 py-1.5 text-sm font-medium text-white hover:opacity-90 disabled:opacity-30"
        >
          {saving ? 'Kaydediliyor…' : 'Kaydet'}
        </button>
      </div>
    </>
  )
}

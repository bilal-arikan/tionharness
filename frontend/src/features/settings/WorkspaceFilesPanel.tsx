// Top-level "Promptlar" view: edit the active workspace's config
// files (runtime prompts, instructions, README) that live under
// <workspace>/config/. Self-loading + self-saving (own Save button), since these
// files are written directly rather than through the app-settings patch flow.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '@/api'
import type { WorkspaceConfig, WorkspaceConfigPatch } from '@/types'
import { Field } from './primitives'
import { PromptEditor, LoadingState, toast } from '@/shared/components'
import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { displayPath } from '@/shared/lib/paths'
import { changedEditablePrompts } from '@/shared/lib/workspacePrompts'

// FilesSaveState lets the parent (WorkspaceView) render the Save button + status
// in its top header instead of this panel showing its own.
export interface FilesSaveState {
  dirty: boolean
  saving: boolean
  save: () => void
}

interface Props {
  onError: (msg: string) => void
  onGoToAgents: () => void
  // Report dirty/saving + a stable save handler to the parent header. Optional so
  // the panel still works standalone.
  onState?: (s: FilesSaveState | null) => void
}

// Labels/hints come from the central prompt registry via the API
// (config.promptMeta); this is only the fallback for an older backend.
const FALLBACK_META = { label: '', hint: '' }

type Draft = { prompts: Record<string, string>; instructions: string; readme: string }

function toDraft(c: WorkspaceConfig): Draft {
  return { prompts: { ...c.prompts }, instructions: c.instructions, readme: c.readme }
}

export function WorkspaceFilesPanel({ onError, onGoToAgents, onState }: Props) {
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

  // Latest values for the stable save handler (so the parent-held save closure
  // never goes stale as the draft changes).
  const draftRef = useRef(draft)
  const originalRef = useRef(original)
  const configRef = useRef(config)
  // Post-commit assignment: the save closure reads these, never render.
  useEffect(() => {
    draftRef.current = draft
    originalRef.current = original
    configRef.current = config
  })

  const save = useCallback(async () => {
    const d = draftRef.current
    const o = originalRef.current
    const c = configRef.current
    if (!d || !o || !c) return
    const patch: WorkspaceConfigPatch = {}
    const changedPrompts = changedEditablePrompts(
      c.promptKeys,
      d.prompts,
      o.prompts,
      c.promptMeta ?? {},
    )
    if (Object.keys(changedPrompts).length) patch.prompts = changedPrompts
    if (d.instructions !== o.instructions) patch.instructions = d.instructions
    if (d.readme !== o.readme) patch.readme = d.readme

    setSaving(true)
    try {
      const updated = await api.updateWorkspaceConfig(patch)
      setConfig(updated)
      setDraft(toDraft(updated))
      setOriginal(toDraft(updated))
      toast.success('Kaydedildi')
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }, [onError])

  // Report save state to the parent header (Kaydet/Kayıtlı live there now).
  useEffect(() => {
    onState?.({ dirty, saving, save })
  }, [dirty, saving, save, onState])
  // Clear the parent header state on unmount (tab switch).
  useEffect(() => () => onState?.(null), [onState])

  if (!config || !draft) {
    return <LoadingState label="Yükleniyor…" />
  }

  const setPrompt = (key: string, val: string) =>
    setDraft((d) => (d ? { ...d, prompts: { ...d.prompts, [key]: val } } : d))

  return (
    <>
      <div className="flex items-center justify-between rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2">
        <span className="text-xs text-[var(--color-text-dim)]">
          Bu dosyalar{' '}
          <code className="rounded bg-[var(--color-bg)] px-1">{displayPath(config.dir)}</code>{' '}
          altında. Hem buradan hem doğrudan diskten düzenleyebilirsin.
        </span>
        <div className="flex shrink-0 items-center gap-1">
          <CopyPathButton path={config.dir} />
        </div>
      </div>

      <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Workspace dosyaları
      </div>
      <Field
        label="Talimatlar (instructions.md)"
        hint="Bu workspace'teki tüm ajanlara eklenen yönergeler. 'Genel' sekmesindeki talimatlarla senkronizedir."
      >
        <PromptEditor
          value={draft.instructions}
          onChange={(v) => setDraft((d) => (d ? { ...d, instructions: v } : d))}
          rows={4}
          autoSize
          placeholder="Örn. Tüm cevapları Türkçe ver; commit at ama push'lama."
        />
      </Field>

      <div className="pt-2 text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">
        Runtime promptları
      </div>
      <p className="text-[11px] text-[var(--color-text-dim)]">
        Sistem ajanlarının promptları (başlık, compaction, insight, subagent profilleri…) burada
        değil: her biri kendi ajanının promptudur ve <strong>Ayarlar ▸ Sistem ajanları</strong>{' '}
        ekranından düzenlenir — doğrudan ya da kalıtım alan bir özelleştirme üzerinden.{' '}
        <button
          type="button"
          onClick={onGoToAgents}
          className="text-[var(--color-accent)] underline underline-offset-2"
        >
          Ajanlar ekranını aç
        </button>
      </p>
      {config.promptKeys.map((key) => {
        const meta = config.promptMeta?.[key] ?? { ...FALLBACK_META, label: key }
        const isDefault = draft.prompts[key].trim() === (config.defaults[key] ?? '').trim()
        const placeholders = meta.placeholders ?? []
        const missing = placeholders.filter((p) => !draft.prompts[key].includes(`{{${p}}}`))
        let hint = meta.hint
        if (placeholders.length) {
          hint = `${hint} Zorunlu yer tutucular: ${placeholders.map((p) => `{{${p}}}`).join(', ')}.`
        }
        hint = `${hint} Boş bırakırsan gömülü varsayılan kullanılır.`
        return (
          <Field key={key} label={meta.label || key} hint={hint}>
            <PromptEditor
              value={draft.prompts[key]}
              onChange={(v) => setPrompt(key, v)}
              rows={4}
              mono
              autoSize
              textareaClassName="text-xs"
            />
            <div className="mt-1 flex flex-wrap items-center gap-2">
              <button
                onClick={() => setPrompt(key, config.defaults[key] ?? '')}
                disabled={isDefault}
                className="rounded border border-[var(--color-border)] px-2 py-0.5 text-[11px] hover:border-[var(--color-accent)] disabled:opacity-30"
              >
                Varsayılana dön
              </button>
              {isDefault ? (
                <span className="text-[11px] text-[var(--color-text-dim)]">varsayılan</span>
              ) : (
                <span className="text-[11px] text-[var(--color-accent)]">özelleştirildi</span>
              )}
              {meta.epochAffecting && (
                <span
                  title="Bu prompt önbelleğe alınan statik sistem prefix'ine girer; değişiklik yeni oturum/epoch'larda etkili olur."
                  className="rounded bg-[var(--color-surface-2)] px-1.5 py-0.5 text-[10px] text-[var(--color-text-dim)]"
                >
                  yeni oturumlarda etkili
                </span>
              )}
              {!isDefault && missing.length > 0 && (
                <span className="rounded bg-[var(--color-danger)]/15 px-1.5 py-0.5 text-[10px] text-[var(--color-danger)]">
                  eksik yer tutucu: {missing.map((p) => `{{${p}}}`).join(', ')} — varsayılana düşer
                </span>
              )}
            </div>
          </Field>
        )
      })}
    </>
  )
}

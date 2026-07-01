// Per-workspace "Promptlar & Dosyalar" category: edit the workspace's config
// files (runtime prompts, instructions, README) that live under
// <workspace>/config/. Self-loading + self-saving (own Save button), since these
// files are written directly rather than through the app-settings patch flow.
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api } from '../../api'
import type { WorkspaceConfig, WorkspaceConfigPatch } from '../../types'
import { Field } from './primitives'
import { PromptEditor } from '../common'
import { CopyPathButton } from '../CopyPathButton'
import { RevealButton } from '../RevealButton'
import { displayPath } from '../../lib/paths'

// FilesSaveState lets the parent (WorkspaceView) render the Save button + status
// in its top header instead of this panel showing its own.
export interface FilesSaveState {
  dirty: boolean
  saving: boolean
  save: () => void
}

interface Props {
  onError: (msg: string) => void
  // Report dirty/saving + a stable save handler to the parent header. Optional so
  // the panel still works standalone.
  onState?: (s: FilesSaveState | null) => void
}

// Human labels for each runtime prompt key.
const PROMPT_LABELS: Record<string, { label: string; hint: string }> = {
  summary: { label: 'Genel bakış promptu', hint: 'Yalnızca /memory · /board · /flows komutlarının anlık genel-bakış sistem promptu (kısa liste özeti). Konuşma özetlemesi (compaction) DEĞİL — o ayrı "Compaction promptu" alanıdır. Boş bırakırsan gömülü varsayılan kullanılır.' },
  reflect: { label: 'Yansıma promptu', hint: '/reflect (dream cycle) yansıma talimatı. Günlük kayıtları otomatik eklenir.' },
  title: { label: 'Başlık promptu', hint: 'Otomatik başlık üretimi sistem promptu.' },
  compact: { label: 'Compaction promptu', hint: 'Bağlam sınırına yaklaşınca geçmişi tek bir yapılandırılmış özete katlayan ASIL prompt (8 bölüm + anti-decay). İki %s yer tutucusu (mevcut özet, yeni mesajlar) KORUNMALI — bozarsan gömülü varsayılana düşer. Boş bırakırsan varsayılan kullanılır.' },
}

type Draft = { prompts: Record<string, string>; instructions: string; readme: string }

function toDraft(c: WorkspaceConfig): Draft {
  return { prompts: { ...c.prompts }, instructions: c.instructions, readme: c.readme }
}

export function WorkspaceFilesPanel({ onError, onState }: Props) {
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
  draftRef.current = draft
  const originalRef = useRef(original)
  originalRef.current = original
  const configRef = useRef(config)
  configRef.current = config

  const save = useCallback(async () => {
    const d = draftRef.current
    const o = originalRef.current
    const c = configRef.current
    if (!d || !o || !c) return
    const patch: WorkspaceConfigPatch = {}
    const changedPrompts: Record<string, string> = {}
    for (const key of c.promptKeys) {
      if (d.prompts[key] !== o.prompts[key]) changedPrompts[key] = d.prompts[key]
    }
    if (Object.keys(changedPrompts).length) patch.prompts = changedPrompts
    if (d.instructions !== o.instructions) patch.instructions = d.instructions
    if (d.readme !== o.readme) patch.readme = d.readme

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
  }, [onError])

  // Report save state to the parent header (Kaydet/Kayıtlı live there now).
  useEffect(() => {
    onState?.({ dirty, saving, save })
  }, [dirty, saving, save, onState])
  // Clear the parent header state on unmount (tab switch).
  useEffect(() => () => onState?.(null), [onState])

  if (!config || !draft) {
    return <div className="text-sm text-[var(--color-text-dim)]">Yükleniyor…</div>
  }

  const setPrompt = (key: string, val: string) =>
    setDraft((d) => (d ? { ...d, prompts: { ...d.prompts, [key]: val } } : d))

  const reveal = () => {
    api.revealWorkspaceConfig().catch((e) => onError((e as Error).message))
  }

  return (
    <>
      <div className="flex items-center justify-between rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2">
        <span className="text-xs text-[var(--color-text-dim)]">
          Bu dosyalar <code className="rounded bg-[var(--color-bg)] px-1">{displayPath(config.dir)}</code> altında. Hem buradan hem doğrudan diskten düzenleyebilirsin.
        </span>
        <div className="flex shrink-0 items-center gap-1">
          <CopyPathButton path={config.dir} />
          <RevealButton onReveal={reveal} label="Klasörü aç" />
        </div>
      </div>

      <div className="text-xs font-semibold uppercase tracking-wide text-[var(--color-text-dim)]">Runtime promptları</div>
      {config.promptKeys.map((key) => {
        const meta = PROMPT_LABELS[key] ?? { label: key, hint: '' }
        const isDefault = draft.prompts[key].trim() === (config.defaults[key] ?? '').trim()
        return (
          <Field key={key} label={meta.label} hint={meta.hint}>
            <PromptEditor
              value={draft.prompts[key]}
              onChange={(v) => setPrompt(key, v)}
              rows={4}
              mono
              textareaClassName="text-xs"
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
        <PromptEditor
          value={draft.instructions}
          onChange={(v) => setDraft((d) => (d ? { ...d, instructions: v } : d))}
          rows={4}
          placeholder="Örn. Tüm cevapları Türkçe ver; commit at ama push'lama."
        />
      </Field>
    </>
  )
}

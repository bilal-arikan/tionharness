import { useState } from 'react'
import type { SkillDetail, SkillInput } from '@/types'
import { api } from '@/api'
import { EmojiField } from '@/shared/components/EmojiField'
import { Button, PromptEditor, ModalOverlay } from '@/shared/components'

interface Props {
  mode: 'create' | 'edit'
  /** Existing skill when editing; undefined when creating. */
  initial?: SkillDetail
  /** Known group names (from the catalog) offered as autocomplete suggestions. */
  groups?: string[]
  onClose: () => void
  /** Called with the saved skill so the list + selection can refresh. */
  onSaved: (saved: SkillDetail) => void
}

// SkillEditor is the create/edit dialog for a skill: visual identity (emoji),
// name + slug, description, when-to-use, on-demand access, and the markdown
// body. It posts to the skill API and hands the saved skill back to the panel.
export function SkillEditor({ mode, initial, groups = [], onClose, onSaved }: Props) {
  const [name, setName] = useState(initial?.name ?? '')
  const [slug, setSlug] = useState('')
  const [icon, setIcon] = useState(initial?.icon ?? '')
  const [description, setDescription] = useState(initial?.description ?? '')
  const [whenToUse, setWhenToUse] = useState(initial?.whenToUse ?? '')
  const [group, setGroup] = useState(initial?.group ?? '')
  const [shared, setShared] = useState(initial?.shared ?? false)
  const [body, setBody] = useState(initial?.body ?? '')
  // Color has no picker here yet; preserve the existing value on edit so it is
  // not wiped by the full-field rewrite.
  const [color] = useState(initial?.color ?? '')
  const [saving, setSaving] = useState(false)
  const [err, setErr] = useState<string | null>(null)

  const save = async () => {
    if (!name.trim()) {
      setErr('Skill adı boş olamaz.')
      return
    }
    setSaving(true)
    setErr(null)
    const input: SkillInput = {
      name: name.trim(),
      description,
      whenToUse,
      icon,
      color,
      group: group.trim(),
      shared,
      body,
    }
    try {
      const saved =
        mode === 'create'
          ? await api.createSkill({ ...input, slug: slug.trim() || undefined })
          : await api.updateSkill(initial!.slug, input)
      onSaved(saved)
    } catch (e) {
      setErr((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <ModalOverlay onClose={onClose}>
      <div
        role="dialog"
        aria-modal="true"
        aria-label="Skill düzenleyici"
        data-testid="skill-editor-modal"
        className="flex max-h-[90vh] w-full max-w-2xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-surface)] shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        {/* Header */}
        <div className="flex items-center gap-3 border-b border-[var(--color-border)] px-5 py-4">
          <span className="flex h-10 w-10 items-center justify-center rounded-full bg-[var(--color-surface-2)] text-xl leading-none">
            {icon || '✨'}
          </span>
          <div className="min-w-0 flex-1">
            <h2 className="truncate text-sm font-semibold text-[var(--color-text)]">
              {mode === 'create' ? 'Yeni skill' : name || 'Skill'}
            </h2>
            <p className="text-xs text-[var(--color-text-dim)]">
              {mode === 'create'
                ? 'Workspace skill oluştur'
                : `Skill düzenle · ${initial?.slug}`}
            </p>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            <button
              onClick={onClose}
              className="rounded px-3 py-1.5 text-sm text-[var(--color-text-dim)] hover:text-[var(--color-text)]"
            >
              İptal
            </button>
            <Button onClick={save} disabled={saving}>
              {saving ? 'Kaydediliyor…' : 'Kaydet'}
            </Button>
          </div>
        </div>

        {/* Body */}
        <div className="flex-1 space-y-4 overflow-y-auto px-5 py-4">
          <div className="flex gap-4">
            <Field label="Simge (emoji)">
              <EmojiField value={icon} onChange={setIcon} clearLabel="✨" />
            </Field>
            <Field label="Ad" className="flex-1">
              <input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder="ör. Görev Planlayıcı"
                className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
              />
            </Field>
          </div>

          {mode === 'create' && (
            <Field label="Slug (opsiyonel — boşsa addan türetilir)">
              <input
                value={slug}
                onChange={(e) => setSlug(e.target.value)}
                placeholder="gorev-planlayici"
                className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 font-mono text-sm outline-none focus:border-[var(--color-accent)]"
              />
            </Field>
          )}

          <Field label="Açıklama">
            <input
              value={description}
              onChange={(e) => setDescription(e.target.value)}
              placeholder="Skill'in tek satırlık özeti (katalogda görünür)"
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </Field>

          <Field label="Ne zaman kullanılır (opsiyonel)">
            <input
              value={whenToUse}
              onChange={(e) => setWhenToUse(e.target.value)}
              placeholder="ör. yeni bir görev planlanırken"
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
          </Field>

          <Field label="Grup (opsiyonel — beceriler bu başlık altında katlanır)">
            <input
              value={group}
              onChange={(e) => setGroup(e.target.value)}
              list="skill-group-suggestions"
              placeholder="ör. Geliştirme, Araştırma, Otomasyon"
              className="w-full rounded border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1.5 text-sm outline-none focus:border-[var(--color-accent)]"
            />
            <datalist id="skill-group-suggestions">
              {groups.map((g) => (
                <option key={g} value={g} />
              ))}
            </datalist>
          </Field>

          <label className="flex items-center gap-2 text-sm text-[var(--color-text)]">
            <input
              type="checkbox"
              checked={shared}
              onChange={(e) => setShared(e.target.checked)}
              className="h-4 w-4 accent-[var(--color-accent)]"
            />
            <span>
              Gerektiğinde (paylaşımlı) — tüm ajanlar atama gerekmeden kullanabilir
            </span>
          </label>

          <Field label="İçerik (Markdown talimatları)">
            <PromptEditor
              value={body}
              onChange={setBody}
              rows={12}
              mono
              autoSizeMax={560}
              placeholder="# Skill&#10;&#10;Talimatları buraya yaz…"
            />
          </Field>

          {err && <p className="text-xs text-[var(--color-danger)]">{err}</p>}
        </div>
      </div>
    </ModalOverlay>
  )
}

function Field({
  label,
  children,
  className = '',
}: {
  label: string
  children: React.ReactNode
  className?: string
}) {
  return (
    <label className={`block space-y-1 ${className}`}>
      <span className="text-xs font-medium text-[var(--color-text-dim)]">{label}</span>
      {children}
    </label>
  )
}

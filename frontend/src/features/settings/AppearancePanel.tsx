import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { api } from '@/api'
import { THEME_COLORS, type ThemeColorVariant } from '@/shared/lib/themePresets'
import { applyAppearance, resolveAppearance, type Appearance } from '@/shared/lib/theme'
import { Field } from './primitives'
import { Button, LoadingState, toast } from '@/shared/components'

// AppearancePanel edits the ACTIVE WORKSPACE's appearance override (the theme
// preset — a color + light/dark variant). It is self-contained — it loads/saves
// the per-workspace settings directly and applies a live preview as the user
// edits — so the shared app-global Save button is hidden for this category. An
// empty preset inherits the app-global appearance, so switching workspaces
// re-themes the UI.
const FALLBACK_APPEARANCE: Appearance = { themePreset: 'violet-dark' }

export function AppearancePanel({
  onError,
  onAppearanceSaved,
}: {
  onError: (msg: string) => void
  onAppearanceSaved?: (a: Appearance) => void
}) {
  const { t } = useTranslation('settingsMain')
  // The app-global appearance acts as the inherited default for empty fields.
  const [globalAppearance, setGlobalAppearance] = useState<Appearance>(FALLBACK_APPEARANCE)
  const [draft, setDraft] = useState<Appearance>(FALLBACK_APPEARANCE)
  const [saved, setSaved] = useState<Appearance>(FALLBACK_APPEARANCE)
  const [hasOverride, setHasOverride] = useState(false)
  const [loading, setLoading] = useState(true)
  const [saving, setSaving] = useState(false)
  // The appearance to restore on unmount when the user leaves with an unsaved
  // live preview (so the previewed theme reverts to what is actually persisted).
  const revertRef = useRef<Appearance>(FALLBACK_APPEARANCE)

  // Load the app-global appearance (inherited default) and this workspace's
  // override, then seed the draft with the effective values.
  useEffect(() => {
    let cancelled = false
    Promise.all([api.getSettings(), api.getWorkspaceSettings()])
      .then(([g, w]) => {
        if (cancelled) return
        const global: Appearance = { themePreset: g.themePreset }
        const override: Partial<Appearance> = { themePreset: w.themePreset }
        const eff = resolveAppearance(override, global)
        setGlobalAppearance(global)
        setDraft(eff)
        setSaved(eff)
        setHasOverride(!!w.themePreset)
        revertRef.current = eff
      })
      .catch((e) => onError((e as Error).message))
      .finally(() => {
        if (!cancelled) setLoading(false)
      })
    return () => {
      cancelled = true
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [])

  // On unmount, revert any unsaved live preview to the persisted appearance.
  useEffect(() => {
    return () => applyAppearance(revertRef.current)
  }, [])

  // Live-preview an edit immediately, then track it in the draft.
  const update = (patch: Partial<Appearance>) =>
    setDraft((d) => {
      const next = { ...d, ...patch }
      applyAppearance(next)
      return next
    })

  const dirty = JSON.stringify(draft) !== JSON.stringify(saved)

  const save = async () => {
    setSaving(true)
    try {
      await api.updateWorkspaceSettings({ themePreset: draft.themePreset })
      setSaved(draft)
      setHasOverride(true)
      revertRef.current = draft
      onAppearanceSaved?.(draft)
      toast.success(t('appearance.saved'))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  // Clear the override so this workspace inherits the app-global appearance.
  const resetToGlobal = async () => {
    setSaving(true)
    try {
      await api.updateWorkspaceSettings({ themePreset: '' })
      setDraft(globalAppearance)
      setSaved(globalAppearance)
      setHasOverride(false)
      revertRef.current = globalAppearance
      applyAppearance(globalAppearance)
      onAppearanceSaved?.(globalAppearance)
      toast.success(t('appearance.resetSuccess'))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  if (loading) return <LoadingState label={t('shared.loading')} />

  return (
    <>
      <div className="rounded-lg border border-[var(--color-border)] bg-[var(--color-surface-2)] px-3 py-2 text-xs text-[var(--color-text-dim)]">
        {t('appearance.scope.prefix')}{' '}
        <span className="font-medium text-[var(--color-text)]">
          {t('appearance.scope.emphasis')}
        </span>{' '}
        {t('appearance.scope.suffix')}
      </div>
      <Field label={t('appearance.theme.label')} hint={t('appearance.theme.hint')}>
        <div className="space-y-2.5">
          {THEME_COLORS.map((c) => {
            const variant = (v: ThemeColorVariant, label: string) => {
              const sel = draft.themePreset === v.id
              return (
                <button
                  key={v.id}
                  type="button"
                  onClick={() => update({ themePreset: v.id })}
                  title={`${t(`appearance.colors.${c.id}`, { defaultValue: c.label })} · ${label}`}
                  className={`flex items-center gap-2.5 rounded-lg border-2 px-3.5 py-2.5 text-left transition ${
                    sel
                      ? 'border-[var(--color-accent)] bg-[var(--color-accent-soft)] ring-2 ring-[var(--color-accent)]/40 shadow-sm'
                      : 'border-[var(--color-border)] hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)]'
                  }`}
                >
                  <span
                    className="flex h-9 w-9 shrink-0 items-center justify-center overflow-hidden rounded-md border"
                    style={{ background: v.bg, borderColor: v.border }}
                  >
                    <span className="h-5 w-5 rounded-full" style={{ background: v.accent }} />
                  </span>
                  <span
                    className={`text-sm ${sel ? 'font-semibold text-[var(--color-text)]' : 'font-medium text-[var(--color-text-dim)]'}`}
                  >
                    {label}
                  </span>
                </button>
              )
            }
            return (
              <div key={c.id} className="flex items-center gap-4">
                <span className="w-20 shrink-0 text-sm font-semibold text-[var(--color-text)]">
                  {t(`appearance.colors.${c.id}`, { defaultValue: c.label })}
                </span>
                <div className="flex gap-2.5">
                  {variant(c.dark, t('appearance.dark'))}
                  {variant(c.light, t('appearance.light'))}
                </div>
              </div>
            )
          })}
        </div>
      </Field>
      <div className="flex items-center gap-3 pt-2">
        <Button size="lg" onClick={save} disabled={!dirty || saving}>
          {saving ? t('shared.saving') : t('shared.save')}
        </Button>
        <button
          onClick={resetToGlobal}
          disabled={saving || !hasOverride}
          className="rounded-lg border-2 border-[var(--color-border)] px-4 py-2 text-sm font-medium hover:border-[var(--color-accent)] hover:bg-[var(--color-surface-2)] disabled:opacity-30"
          title={t('appearance.resetTitle')}
        >
          {t('appearance.reset')}
        </button>
        <span className="text-xs text-[var(--color-text-dim)]">
          {hasOverride ? t('appearance.customStatus') : t('appearance.globalStatus')}
        </span>
      </div>
    </>
  )
}

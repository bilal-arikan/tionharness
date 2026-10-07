import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ShieldCheck, Save } from 'lucide-react'
import { api } from '@/api'
import type { AppSettings } from '@/types'
import { NumberField, NumberValidityProvider, Toggle } from '@/features/settings/primitives'
import { useNumberValidity } from '@/features/settings/numberValidity'
import { InfoPopover, LoadingState } from '@/shared/components'

interface Props {
  onError: (msg: string) => void
}

// SelfHealingTab hosts the loop-protection guardrails, the stuck-session
// threshold and the failure→lesson reflection switch. The distilled lessons
// themselves are workspace memory notes now (kind: lesson) and live on the Notes
// screen; this tab only edits the app-global settings (draft + Save).
export function SelfHealingTab({ onError }: Props) {
  const { t } = useTranslation('insight')
  const [draft, setDraft] = useState<AppSettings | null>(null)
  const [original, setOriginal] = useState<AppSettings | null>(null)
  const [saving, setSaving] = useState(false)
  // This tab owns both the NumberFields and the Save button, so it is the right
  // boundary for the validity provider (InsightPanel only picks the tab and has
  // no Save of its own). Without it a rejected number would stay on screen while
  // Save wrote the previous value back — silent data loss.
  const numberValidity = useNumberValidity()

  useEffect(() => {
    api
      .getSettings()
      .then((s) => {
        setDraft(s)
        setOriginal(s)
      })
      .catch((e) => onError((e as Error).message))
  }, [onError])

  const set = <K extends keyof AppSettings>(key: K, val: AppSettings[K]) =>
    setDraft((d) => (d ? { ...d, [key]: val } : d))

  const dirty = !!draft && !!original && JSON.stringify(draft) !== JSON.stringify(original)

  const save = async () => {
    if (!draft) return
    setSaving(true)
    try {
      const updated = await api.updateSettings({
        toolGuardWarnings: draft.toolGuardWarnings,
        toolGuardHardStop: draft.toolGuardHardStop,
        guardExactWarn: draft.guardExactWarn,
        guardExactBlock: draft.guardExactBlock,
        guardSameToolWarn: draft.guardSameToolWarn,
        guardSameToolHalt: draft.guardSameToolHalt,
        guardNoProgressWarn: draft.guardNoProgressWarn,
        guardNoProgressBlock: draft.guardNoProgressBlock,
        stuckTurnThreshold: draft.stuckTurnThreshold,
        lessonReflect: draft.lessonReflect,
      })
      setDraft(updated)
      setOriginal(updated)
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  if (!draft) {
    return <LoadingState label={t('actions.loading')} />
  }

  return (
    <NumberValidityProvider value={numberValidity}>
      <div className="max-w-2xl space-y-3">
        <div className="flex items-center justify-between">
          <h3 className="flex items-center gap-2 text-sm font-semibold">
            <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
              <ShieldCheck size={14} />
            </span>
            {t('selfHealing.title')}
            <InfoPopover
              text={
                <>
                  {t('selfHealing.descriptionBefore')} <code>56-SELF-HEALING</code>
                  {t('selfHealing.descriptionMiddle')} <code>stuck</code>{' '}
                  {t('selfHealing.descriptionAfter')}
                </>
              }
            />
          </h3>
          <div className="flex items-center gap-3">
            {numberValidity.hasInvalid && (
              <span className="text-xs text-[var(--color-danger)]">
                {t('validation.invalidNumber')}
              </span>
            )}
            <button
              onClick={save}
              data-testid="self-healing-save"
              disabled={!dirty || saving || numberValidity.hasInvalid}
              title={numberValidity.hasInvalid ? t('validation.invalidNumber') : undefined}
              className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-[var(--color-on-accent)] disabled:opacity-50"
            >
              <Save className="h-4 w-4" /> {saving ? t('actions.saving') : t('actions.save')}
            </button>
          </div>
        </div>

        <Toggle
          label={t('selfHealing.guardWarnings')}
          hint={t('selfHealing.guardWarningsHint')}
          checked={draft.toolGuardWarnings}
          onChange={(v) => set('toolGuardWarnings', v)}
        />
        <Toggle
          label={t('selfHealing.hardStop')}
          hint={t('selfHealing.hardStopHint')}
          checked={draft.toolGuardHardStop}
          onChange={(v) => set('toolGuardHardStop', v)}
        />
        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <NumberField
            label={t('selfHealing.exactWarn')}
            hint={t('selfHealing.exactWarnHint')}
            min={0}
            max={50}
            value={draft.guardExactWarn}
            onChange={(v) => set('guardExactWarn', v)}
          />
          <NumberField
            label={t('selfHealing.exactBlock')}
            hint={t('selfHealing.exactBlockHint')}
            min={0}
            max={50}
            value={draft.guardExactBlock}
            onChange={(v) => set('guardExactBlock', v)}
          />
          <NumberField
            label={t('selfHealing.sameToolWarn')}
            hint={t('selfHealing.sameToolWarnHint')}
            min={0}
            max={50}
            value={draft.guardSameToolWarn}
            onChange={(v) => set('guardSameToolWarn', v)}
          />
          <NumberField
            label={t('selfHealing.sameToolHalt')}
            hint={t('selfHealing.sameToolHaltHint')}
            min={0}
            max={50}
            value={draft.guardSameToolHalt}
            onChange={(v) => set('guardSameToolHalt', v)}
          />
          <NumberField
            label={t('selfHealing.noProgressWarn')}
            hint={t('selfHealing.noProgressWarnHint')}
            min={0}
            max={50}
            value={draft.guardNoProgressWarn}
            onChange={(v) => set('guardNoProgressWarn', v)}
          />
          <NumberField
            label={t('selfHealing.noProgressBlock')}
            hint={t('selfHealing.noProgressBlockHint')}
            min={0}
            max={50}
            value={draft.guardNoProgressBlock}
            onChange={(v) => set('guardNoProgressBlock', v)}
          />
        </div>
        <NumberField
          label={t('selfHealing.stuckThreshold')}
          hint={t('selfHealing.stuckThresholdHint')}
          min={0}
          max={20}
          value={draft.stuckTurnThreshold}
          onChange={(v) => set('stuckTurnThreshold', v)}
        />
        <Toggle
          label={t('selfHealing.reflect')}
          hint={t('selfHealing.reflectHint')}
          checked={draft.lessonReflect}
          onChange={(v) => set('lessonReflect', v)}
        />
      </div>
    </NumberValidityProvider>
  )
}

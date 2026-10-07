import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Save, Trash2 } from 'lucide-react'
import { api } from '@/api'
import { InfoPopover, toast } from '@/shared/components'
import { NumberField, NumberValidityProvider } from '@/features/settings/primitives'
import { useNumberValidity } from '@/features/settings/numberValidity'
import type { InsightSettings } from '@/types'

interface Props {
  settings: InsightSettings
  setSettings: (s: InsightSettings) => void
  onError: (msg: string) => void
  // Called after a reset so the panel can refresh its (now-empty) findings.
  onReset: () => void
}

const inputCls =
  'mt-1 rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-2 py-1 text-sm'

export function SettingsTab({ settings, setSettings, onError, onReset }: Props) {
  const { t } = useTranslation('insight')
  const [saving, setSaving] = useState(false)
  const [deep, setDeep] = useState(false)
  const [resetting, setResetting] = useState(false)
  // Same boundary as SelfHealingTab: this tab renders the numeric fields and the
  // Save button, so it owns the validity set and gates Save on it.
  const numberValidity = useNumberValidity()

  const save = async () => {
    setSaving(true)
    try {
      setSettings(await api.updateInsightSettings(settings))
      toast.success(t('actions.saved'))
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  const reset = async () => {
    const msg = t(deep ? 'settings.deepResetConfirm' : 'settings.resetConfirm')
    if (!confirm(msg)) return
    setResetting(true)
    try {
      await api.resetInsight(deep)
      onReset()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setResetting(false)
    }
  }

  return (
    <div className="max-w-xl space-y-4">
      <NumberValidityProvider value={numberValidity}>
        <div className="space-y-3 rounded-md border border-[var(--color-border)] p-3">
          <label className="block">
            <span className="text-sm">{t('settings.appFixRepoPath')}</span>
            <input
              type="text"
              value={settings.appFixRepoPath ?? ''}
              onChange={(e) => setSettings({ ...settings, appFixRepoPath: e.target.value })}
              placeholder="C:/Users/.../TionHarness"
              className={`${inputCls} w-full`}
            />
          </label>
          <NumberField
            label={t('settings.maxSessions')}
            min={0}
            value={settings.maxSessions ?? 0}
            onChange={(v) => setSettings({ ...settings, maxSessions: v })}
          />
          <NumberField
            label={t('settings.maxAnalyzed')}
            hint={t('settings.maxAnalyzedHint')}
            min={0}
            value={settings.maxAnalyzed ?? 0}
            onChange={(v) => setSettings({ ...settings, maxAnalyzed: v })}
          />
          <NumberField
            label={t('settings.scanSinceDays')}
            hint={t('settings.scanSinceDaysHint')}
            min={0}
            value={settings.scanSinceDays ?? 0}
            onChange={(v) => setSettings({ ...settings, scanSinceDays: v })}
          />
          <NumberField
            label={t('settings.autoVerifyDays')}
            hint={t('settings.autoVerifyDaysHint')}
            min={0}
            value={settings.autoVerifyDays ?? 0}
            onChange={(v) => setSettings({ ...settings, autoVerifyDays: v })}
          />
          <NumberField
            label={t('settings.pruneDays')}
            hint={t('settings.pruneDaysHint')}
            min={0}
            value={settings.pruneDays ?? 0}
            onChange={(v) => setSettings({ ...settings, pruneDays: v })}
          />
          <label className="block">
            <span className="flex items-center gap-1 text-sm">
              {t('settings.autoScanCron')}
              <InfoPopover text={t('settings.cronHint')} />
            </span>
            <input
              type="text"
              value={settings.autoScanCron ?? ''}
              onChange={(e) => setSettings({ ...settings, autoScanCron: e.target.value })}
              placeholder={t('settings.cronPlaceholder')}
              className={`${inputCls} w-full font-mono`}
            />
          </label>
          {numberValidity.hasInvalid && (
            <div className="text-xs text-[var(--color-danger)]">
              {t('validation.invalidNumber')}
            </div>
          )}
          <button
            onClick={save}
            disabled={saving || numberValidity.hasInvalid}
            title={numberValidity.hasInvalid ? t('validation.invalidNumber') : undefined}
            className="flex items-center gap-1 rounded-md bg-[var(--color-accent)] px-3 py-1 text-sm text-[var(--color-on-accent)] disabled:opacity-50"
          >
            <Save className="h-4 w-4" /> {saving ? t('actions.saving') : t('actions.save')}
          </button>
        </div>
      </NumberValidityProvider>

      {/* Danger zone: reset all insight data for this workspace. */}
      <div className="space-y-2 rounded-md border border-[var(--color-danger)]/40 p-3">
        <div className="text-sm font-semibold text-[var(--color-danger)]">
          {t('settings.dangerZone')}
        </div>
        <p className="text-xs text-[var(--color-text-dim)]">{t('settings.resetDescription')}</p>
        <label className="flex items-center gap-2 text-xs">
          <input type="checkbox" checked={deep} onChange={(e) => setDeep(e.target.checked)} />
          {t('settings.clearLedger')}
        </label>
        <button
          onClick={reset}
          disabled={resetting}
          className="flex items-center gap-1 rounded-md border border-[var(--color-danger)] px-3 py-1 text-sm text-[var(--color-danger)] hover:bg-[var(--color-danger)]/10 disabled:opacity-50"
        >
          <Trash2 className="h-4 w-4" />{' '}
          {resetting ? t('settings.resetting') : t('settings.resetAll')}
        </button>
      </div>
    </div>
  )
}

import { Button } from '@/shared/components'
import { useTranslation } from 'react-i18next'

export interface SettingsSaveBarProps {
  dirty: boolean
  saving: boolean
  invalid: boolean
  onSave: () => void
}

export function SettingsSaveBar({ dirty, saving, invalid, onSave }: SettingsSaveBarProps) {
  const { t } = useTranslation('settings')
  return (
    <div className="flex flex-wrap items-center gap-3">
      <span role="status" className="text-xs text-[var(--color-text-dim)]">
        {invalid ? t('saveBar.invalid') : dirty ? t('common.unsavedChanges') : t('common.saved')}
      </span>
      <Button onClick={onSave} disabled={!dirty || saving || invalid}>
        {saving ? t('common.saving') : t('common.save')}
      </Button>
    </div>
  )
}

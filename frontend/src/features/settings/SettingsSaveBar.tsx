import { Button } from '@/shared/components'

export interface SettingsSaveBarProps {
  dirty: boolean
  saving: boolean
  invalid: boolean
  onSave: () => void
}

export function SettingsSaveBar({ dirty, saving, invalid, onSave }: SettingsSaveBarProps) {
  return (
    <div className="flex flex-wrap items-center gap-3">
      <span role="status" className="text-xs text-[var(--color-text-dim)]">
        {invalid
          ? 'Invalid number — correct it before saving'
          : dirty
            ? 'Unsaved changes'
            : 'Saved'}
      </span>
      <Button onClick={onSave} disabled={!dirty || saving || invalid}>
        {saving ? 'Saving…' : 'Save'}
      </Button>
    </div>
  )
}

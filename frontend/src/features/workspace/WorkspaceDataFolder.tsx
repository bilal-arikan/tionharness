import { CopyPathButton } from '@/shared/components/CopyPathButton'
import { useTranslation } from 'react-i18next'

interface Props {
  path: string
}

// The data folder is assigned by the backend and cannot be edited here.
export function WorkspaceDataFolder({ path }: Props) {
  const { t } = useTranslation('workspace')
  if (!path) return null
  return (
    <div className="flex flex-col gap-1.5">
      <label
        htmlFor="workspace-data-folder"
        className="text-xs font-medium text-[var(--color-text-dim)]"
      >
        {t('dataFolder.label')}
      </label>
      <div className="flex min-w-0 items-center gap-2">
        <input
          id="workspace-data-folder"
          data-testid="workspace-data-folder"
          value={path}
          readOnly
          className="min-w-0 flex-1 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2.5 py-1.5 font-mono text-xs text-[var(--color-text)]"
        />
        <CopyPathButton path={path} testId="workspace-data-folder-copy" />
      </div>
    </div>
  )
}

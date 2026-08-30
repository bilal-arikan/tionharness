import { useState } from 'react'
import { FileText } from 'lucide-react'
import { Button } from '@/shared/components'
import { useRegisterDirty } from '@/shared/lib/dirtySignals'
import { WorkspaceFilesPanel, type FilesSaveState } from './WorkspaceFilesPanel'
import { useTranslation } from 'react-i18next'

interface Props {
  onError: (msg: string) => void
  onGoToAgents: () => void
}

export function PromptsView({ onError, onGoToAgents }: Props) {
  const { t } = useTranslation()
  const [state, setState] = useState<FilesSaveState | null>(null)
  const dirty = !!state?.dirty
  const saving = !!state?.saving

  useRegisterDirty('prompts', dirty)

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="flex items-center justify-between border-b border-[var(--color-border)] px-6 py-3">
        <span className="flex items-center gap-2 text-left text-sm font-semibold">
          <span className="flex h-6 w-6 items-center justify-center rounded-md bg-[var(--color-accent-soft)] text-[var(--color-accent)]">
            <FileText size={14} />
          </span>
          {t('navigation.promptsFiles')}
        </span>
        {state && (
          <div className="flex items-center gap-3">
            <span className="text-xs text-[var(--color-text-dim)]">
              {dirty ? t('common.unsavedChanges') : t('common.saved')}
            </span>
            <Button onClick={state.save} disabled={!dirty || saving}>
              {saving ? t('common.saving') : t('common.save')}
            </Button>
          </div>
        )}
      </div>
      <div className="min-w-0 flex-1 overflow-y-auto p-6">
        <WorkspaceFilesPanel onError={onError} onGoToAgents={onGoToAgents} onState={setState} />
      </div>
    </div>
  )
}

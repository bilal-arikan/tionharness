import { Archive, ArchiveRestore, FolderInput, Trash2 } from 'lucide-react'
import type { MultiSelect } from '@/shared/hooks/useMultiSelect'
import { SelectionBar, SelectionBarButton } from '@/shared/components'
import type { ArtifactBulkActions } from './useArtifactBulkActions'
import { useTranslation } from 'react-i18next'

interface Props {
  sel: MultiSelect
  orderedIds: string[]
  groupNames: string[]
  showArchived: boolean
  bulk: ArtifactBulkActions
}

// ArtifactBulkBar is the multi-select action bar under the list: bulk group
// assignment, bulk archive / un-archive and bulk delete.
export function ArtifactBulkBar({ sel, orderedIds, groupNames, showArchived, bulk }: Props) {
  const { t } = useTranslation('artifacts')
  const { bulkGroup, setBulkGroup, bulkGroupBusy, bulkArchiveBusy } = bulk
  return (
    <SelectionBar
      count={sel.count}
      onClear={sel.clear}
      onSelectAll={orderedIds.length ? () => sel.selectAll(orderedIds) : undefined}
    >
      {/* Bulk group: move every selected artifact into one organisation bucket. */}
      <div data-testid="artifacts-bulk-group" className="inline-flex items-center gap-1">
        <input
          list="artifacts-bulk-group-names"
          value={bulkGroup}
          onChange={(e) => setBulkGroup(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              bulk.bulkSetGroup(bulkGroup.trim())
            }
          }}
          disabled={bulkGroupBusy}
          placeholder={t('bulk.groupPlaceholder')}
          data-testid="artifacts-bulk-group-input"
          className="w-28 rounded-md border border-[var(--color-border)] bg-[var(--color-bg)] px-2 py-1 text-xs text-[var(--color-text)] outline-none focus:border-[var(--color-accent)] disabled:opacity-50"
        />
        <datalist id="artifacts-bulk-group-names">
          {groupNames.map((n) => (
            <option key={n} value={n} />
          ))}
        </datalist>
        <SelectionBarButton
          icon={<FolderInput size={13} />}
          onClick={() => bulk.bulkSetGroup(bulkGroup.trim())}
          disabled={bulkGroupBusy}
          title={
            bulkGroup.trim()
              ? t('bulk.moveToGroupTitle', { group: bulkGroup.trim() })
              : t('bulk.ungroupTitle')
          }
        >
          {bulkGroup.trim() ? t('bulk.assign') : t('group.ungrouped')}
        </SelectionBarButton>
      </div>
      {/* Bulk archive / un-archive: hides (or restores) the selection. The
          verb follows the current view — archive in the active view, restore
          in the archived view. */}
      <SelectionBarButton
        icon={showArchived ? <ArchiveRestore size={13} /> : <Archive size={13} />}
        onClick={() => bulk.bulkSetArchived(!showArchived)}
        disabled={bulkArchiveBusy}
        title={showArchived ? t('bulk.restoreTitle') : t('bulk.archiveTitle')}
      >
        {showArchived ? t('actions.restore') : t('actions.archive')}
      </SelectionBarButton>
      <SelectionBarButton icon={<Trash2 size={13} />} onClick={bulk.bulkDelete} danger>
        {t('actions.delete')}
      </SelectionBarButton>
    </SelectionBar>
  )
}

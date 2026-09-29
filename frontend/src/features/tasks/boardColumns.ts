import type { BoardColumnDef } from '@/types'
import { i18next } from '@/i18n'

// The backend returns this canonical set when a workspace has no custom board
// configuration. Keep it as storage data and translate only its display labels;
// labels changed by the user never match this table and remain untouched.
const BUILTIN_COLUMN_LABELS: Record<string, string> = {
  pbi: 'PBI',
  todo: 'Yapılacak',
  in_progress: 'Devam Eden',
  review: 'İnceleme',
  done: 'Bitti',
  failed: 'Başarısız',
  iptal: 'İptal',
}

export function fallbackBoardColumns(): BoardColumnDef[] {
  return Object.entries(BUILTIN_COLUMN_LABELS).map(([key, label]) => ({ key, label, color: '' }))
}

export function localizeBoardColumns(columns: BoardColumnDef[]): BoardColumnDef[] {
  return columns.map((column) =>
    BUILTIN_COLUMN_LABELS[column.key] === column.label
      ? { ...column, label: i18next.t(`boardStates.${column.key}`, { ns: 'tasks' }) }
      : column,
  )
}

// Convert untouched translated built-in labels back to the backend's canonical
// representation. New and user-renamed labels pass through verbatim.
export function canonicalizeBoardColumns(
  edited: BoardColumnDef[],
  stored: BoardColumnDef[],
): BoardColumnDef[] {
  return edited.map((column) => {
    const current = stored.find((candidate) => candidate.key === column.key)
    const canonical = BUILTIN_COLUMN_LABELS[column.key]
    const translated = canonical
      ? i18next.t(`boardStates.${column.key}`, { ns: 'tasks' })
      : undefined
    return current?.label === canonical && column.label === translated
      ? { ...column, label: canonical }
      : column
  })
}

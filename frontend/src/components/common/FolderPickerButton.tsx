import { useState } from 'react'
import { FolderSearch, CornerLeftUp, Check, Folder } from 'lucide-react'
import { api } from '../../api'
import { useOutsideClick } from '../../hooks/useOutsideClick'
import type { BrowseResp } from '../../types'
import { Button } from './index'

// FolderPickerButton is a compact folder icon that opens a directory-browser
// popover and calls onSelect with the chosen absolute path. Reusable across the
// settings forms (e.g. the workspace default working dir). It browses via the
// /api/fs/browse endpoint; an empty path lists the filesystem roots.
export function FolderPickerButton({
  startPath,
  onSelect,
  title = 'Klasör seç',
}: {
  startPath?: string
  onSelect: (path: string) => void
  title?: string
}) {
  const [open, setOpen] = useState(false)
  const [browse, setBrowse] = useState<BrowseResp | null>(null)
  const [busy, setBusy] = useState(false)
  const rootRef = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)

  const navigate = async (path: string) => {
    setBusy(true)
    try {
      setBrowse(await api.browseDirs(path))
    } catch {
      try {
        setBrowse(await api.browseDirs(''))
      } catch {
        setBrowse(null)
      }
    } finally {
      setBusy(false)
    }
  }

  const openPicker = () => {
    setOpen(true)
    void navigate(startPath || '')
  }

  const pick = (path: string) => {
    onSelect(path)
    setOpen(false)
  }

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        onClick={openPicker}
        title={title}
        className="flex items-center rounded-lg border border-[var(--color-border)] px-2.5 py-2 text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
      >
        <FolderSearch size={16} />
      </button>

      {open && (
        <div className="absolute right-0 z-20 mt-1 w-80 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2 shadow-xl">
          <div className="mb-1 flex items-center gap-1 px-1">
            <button
              onClick={() => navigate(browse?.parent ?? '')}
              disabled={busy || !browse?.path}
              title="Üst klasör"
              className="rounded-md p-1 text-[var(--color-text-dim)] hover:text-[var(--color-accent)] disabled:opacity-30"
            >
              <CornerLeftUp size={14} />
            </button>
            <span className="truncate text-xs text-[var(--color-text)]" title={browse?.path}>
              {browse?.path || 'Bu bilgisayar'}
            </span>
          </div>

          <div className="max-h-56 overflow-y-auto rounded-lg border border-[var(--color-border)]">
            {busy && <div className="px-2 py-3 text-xs text-[var(--color-text-dim)]">Yükleniyor…</div>}
            {!busy && (browse?.entries.length ?? 0) === 0 && (
              <div className="px-2 py-3 text-xs text-[var(--color-text-dim)]">Alt klasör yok</div>
            )}
            {!busy &&
              browse?.entries.map((e) => (
                <button
                  key={e.path}
                  onClick={() => navigate(e.path)}
                  className="flex w-full items-center gap-2 px-2 py-1.5 text-left text-sm text-[var(--color-text-dim)] hover:bg-[var(--color-surface)] hover:text-[var(--color-text)]"
                >
                  <Folder size={14} className="shrink-0 text-[var(--color-accent)]" />
                  <span className="truncate">{e.name}</span>
                </button>
              ))}
          </div>

          <Button
            onClick={() => browse?.path && pick(browse.path)}
            disabled={busy || !browse?.path}
            size="lg"
            className="mt-2 flex w-full items-center justify-center gap-1.5"
          >
            <Check size={14} /> Bu klasörü seç
          </Button>
        </div>
      )}
    </div>
  )
}

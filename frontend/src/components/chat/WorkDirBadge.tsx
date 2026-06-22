import { useEffect, useState } from 'react'
import { FolderOpen, GitBranch, CornerLeftUp, RotateCcw, Check, Folder } from 'lucide-react'
import { api } from '../../api'
import { useOutsideClick } from '../../hooks/useOutsideClick'
import type { WorkdirInfo, BrowseResp } from '../../types'
import { Button } from '../common'

// basename returns the last path segment for a compact badge label, handling
// both Windows ("\\") and POSIX ("/") separators.
function basename(p: string): string {
  const trimmed = p.replace(/[\\/]+$/, '')
  const idx = Math.max(trimmed.lastIndexOf('\\'), trimmed.lastIndexOf('/'))
  return idx >= 0 ? trimmed.slice(idx + 1) || trimmed : trimmed
}

// WorkDirBadge is the composer's working-directory selector (the external agent project-style):
// a compact button showing the current cwd folder + git branch, opening a folder
// picker to change the session's working directory mid-conversation. Changes take
// effect on the next message. Self-contained: it reads/writes the cwd via the
// session API using the sessionId prop.
export function WorkDirBadge({ sessionId }: { sessionId?: string }) {
  const [info, setInfo] = useState<WorkdirInfo | null>(null)
  const [open, setOpen] = useState(false)
  const [browse, setBrowse] = useState<BrowseResp | null>(null)
  const [busy, setBusy] = useState(false)
  // Close the picker on outside click (detached while closed).
  const rootRef = useOutsideClick<HTMLDivElement>(() => setOpen(false), open)

  // Load the current working-dir state whenever the active session changes.
  useEffect(() => {
    if (!sessionId) {
      setInfo(null)
      return
    }
    let alive = true
    api
      .getWorkdir(sessionId)
      .then((r) => alive && setInfo(r))
      .catch(() => alive && setInfo(null))
    return () => {
      alive = false
    }
  }, [sessionId])

  const openPicker = () => {
    if (!sessionId) return
    setOpen(true)
    // Start browsing from the current effective dir (show its subfolders); fall
    // back to the filesystem roots.
    void navigate(info?.effective || '')
  }

  const navigate = async (path: string) => {
    setBusy(true)
    try {
      setBrowse(await api.browseDirs(path))
    } catch {
      // A non-readable path: fall back to the roots.
      try {
        setBrowse(await api.browseDirs(''))
      } catch {
        setBrowse(null)
      }
    } finally {
      setBusy(false)
    }
  }

  const apply = async (dir: string) => {
    if (!sessionId) return
    setBusy(true)
    try {
      setInfo(await api.setWorkdir(sessionId, dir))
      setOpen(false)
    } catch {
      // keep the picker open on failure
    } finally {
      setBusy(false)
    }
  }

  const label = info?.effective ? basename(info.effective) : 'Dizin'
  const isOverride = !!info?.dir

  return (
    <div ref={rootRef} className="relative shrink-0">
      <button
        type="button"
        onClick={openPicker}
        disabled={!sessionId}
        title={
          info?.effective
            ? `Çalışma dizini: ${info.effective}${info.branch ? ` (${info.branch})` : ''}`
            : 'Çalışma dizini seç'
        }
        className={`flex max-w-[220px] items-center gap-1 rounded-xl border px-2.5 py-3 text-sm transition disabled:opacity-30 ${
          isOverride
            ? 'border-[var(--color-accent)] text-[var(--color-accent)]'
            : 'border-[var(--color-border)] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]'
        }`}
      >
        <FolderOpen size={15} className="shrink-0" />
        <span className="hidden truncate sm:inline">{label}</span>
        {info?.branch && (
          <span className="hidden items-center gap-0.5 opacity-70 md:flex">
            <GitBranch size={12} />
            <span className="max-w-[80px] truncate">{info.branch}</span>
          </span>
        )}
      </button>

      {open && (
        <div className="absolute bottom-full left-0 mb-2 w-80 rounded-xl border border-[var(--color-border)] bg-[var(--color-surface-2)] p-2 shadow-xl">
          <div className="mb-1 flex items-center justify-between px-1">
            <span className="text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
              Çalışma dizini
            </span>
            {info?.dir && (
              <button
                onClick={() => apply('')}
                disabled={busy}
                title="Workspace varsayılanına sıfırla"
                className="flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] text-[var(--color-text-dim)] hover:text-[var(--color-accent)]"
              >
                <RotateCcw size={11} /> Sıfırla
              </button>
            )}
          </div>

          {/* Current browsed path + "up" navigation. */}
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

          {/* Subdirectory list. */}
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

          {/* Commit the currently-browsed folder as the working dir. */}
          <Button
            onClick={() => browse?.path && apply(browse.path)}
            disabled={busy || !browse?.path}
            size="lg"
            className="mt-2 flex w-full items-center justify-center gap-1.5"
          >
            <Check size={14} /> Bu klasörü kullan
          </Button>
        </div>
      )}
    </div>
  )
}

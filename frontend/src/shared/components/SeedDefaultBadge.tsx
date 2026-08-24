import { useState } from 'react'
import { Lock, RotateCcw } from 'lucide-react'
import type { SeedDefaultState } from '@/types'
import { toast } from './Toast'

// SeedDefaultBadge marks a shipped file the user has edited — and ONLY that case.
// 'default' and 'tuned' both keep auto-updating (the refresh compares bodies
// separately from config), so badging them would put a chip on every row while
// saying nothing actionable. The one fact worth a pixel is "this file no longer
// receives updates".
export function SeedDefaultBadge({ state }: { state?: SeedDefaultState }) {
  if (state !== 'edited') return null
  return (
    <span
      title="Bu dosya varsayılandan farklı — TionHarness güncellemeleri buraya artık otomatik gelmez. 'Varsayılan' ile geri döndürebilirsin (yerel değişiklikler silinir)."
      className="inline-flex shrink-0 items-center gap-1 rounded px-1.5 py-0.5 text-[10px] font-medium text-[var(--color-warning)] ring-1 ring-inset ring-[color-mix(in_srgb,var(--color-warning)_35%,transparent)]"
    >
      <Lock className="h-3 w-3" />
      düzenlendi
    </span>
  )
}

interface RestoreProps {
  // Human label of the thing being restored, used in the success toast.
  label: string
  // Performs the restore (the caller owns which endpoint that is).
  onRestore: () => Promise<unknown>
  // Called after a successful restore so the caller can refetch its list.
  onDone: () => void
  onError: (msg: string) => void
}

// RestoreDefaultButton overwrites a shipped file with the version TionHarness ships.
// Destructive (local changes are gone), so it arms first — inline, turning into a
// yes/cancel pair rather than blocking on a window.confirm, which reads as a bug
// report popup in an app that otherwise never uses one.
//
// Render it only where a shipped default EXISTS (defaultState set); restoring
// something with no default is a 404 the user should never be able to trigger.
export function RestoreDefaultButton({ label, onRestore, onDone, onError }: RestoreProps) {
  const [armed, setArmed] = useState(false)
  const [busy, setBusy] = useState(false)

  const run = async () => {
    setBusy(true)
    try {
      await onRestore()
      toast.success(`${label} varsayılana döndürüldü`)
      onDone()
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setBusy(false)
      setArmed(false)
    }
  }

  if (armed) {
    return (
      <span className="flex shrink-0 items-center gap-1 text-xs">
        <span className="text-[var(--color-text-dim)]">Düzenlemeler silinsin mi?</span>
        <button
          onClick={run}
          disabled={busy}
          className="rounded px-2 py-1 text-[var(--color-danger)] hover:bg-[var(--color-surface-2)] disabled:opacity-50"
        >
          {busy ? '…' : 'Evet'}
        </button>
        <button
          onClick={() => setArmed(false)}
          className="rounded px-2 py-1 hover:bg-[var(--color-surface-2)]"
        >
          Vazgeç
        </button>
      </span>
    )
  }
  return (
    <button
      onClick={() => setArmed(true)}
      title="TionHarness ile gelen varsayılan içeriğe döndür (yerel düzenlemeler silinir). Döndürülen dosya yeniden otomatik güncellenmeye başlar."
      className="flex shrink-0 items-center gap-1 rounded px-2 py-1 text-xs hover:bg-[var(--color-surface-2)]"
    >
      <RotateCcw className="h-3.5 w-3.5" /> Varsayılan
    </button>
  )
}

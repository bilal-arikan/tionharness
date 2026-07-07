import { useState } from 'react'
import { FolderOpen, Loader2 } from 'lucide-react'
import { PATH_ACTION_CLS } from './CopyPathButton'

interface Props {
  /** Reveal the path in the OS file manager (usually an async API call). The
   * return value is ignored (the reveal endpoints resolve to a `{path}`), so any
   * handler shape is accepted. */
  onReveal: () => void | Promise<unknown>
  /** Optional inline label ("Klasörü aç"); icon-only when omitted. */
  label?: string
  /** Extra classes on the label span (e.g. "hidden sm:inline" for responsiveness). */
  labelClassName?: string
  title?: string
  testId?: string
  disabled?: boolean
}

// RevealButton opens a filesystem folder/file in the OS file manager. Shares the
// exact look of <CopyPathButton> (PATH_ACTION_CLS) so the copy/open pair reads as
// one consistent control everywhere it appears; shows a spinner while the reveal
// request is in flight.
export function RevealButton({ onReveal, label, labelClassName = '', title = 'Klasörü aç', testId, disabled }: Props) {
  const [busy, setBusy] = useState(false)

  const click = async () => {
    setBusy(true)
    try {
      await onReveal()
    } finally {
      setBusy(false)
    }
  }

  return (
    <button type="button" onClick={click} disabled={disabled || busy} data-testid={testId} title={title} className={PATH_ACTION_CLS}>
      {busy ? <Loader2 size={14} className="animate-spin" /> : <FolderOpen size={14} />}
      {label && <span className={labelClassName}>{label}</span>}
    </button>
  )
}

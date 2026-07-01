import { useState } from 'react'
import { Copy, Check } from 'lucide-react'
import { displayPath } from '../lib/paths'

// Shared visual language for the path actions (copy path / open folder) used
// across the app so they always look identical: a compact bordered icon button
// that dims to accent on hover, with an optional inline text label.
export const PATH_ACTION_CLS =
  'flex shrink-0 items-center justify-center gap-1.5 rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-50'

interface Props {
  /** A ready path string to copy. */
  path?: string
  /** Lazily fetch the path (e.g. from an API) when clicked; used when the path
   *  is only known server-side. Takes precedence over `path` when both are set. */
  getPath?: () => Promise<string>
  /** Optional inline label ("Yolu kopyala"); icon-only when omitted. */
  label?: string
  /** Extra classes on the label span (e.g. "hidden sm:inline" for responsiveness). */
  labelClassName?: string
  title?: string
  testId?: string
  onError?: (msg: string) => void
}

// CopyPathButton copies a filesystem path to the clipboard and briefly flips to a
// check on success. Standardised look shared with <RevealButton> via
// PATH_ACTION_CLS.
export function CopyPathButton({ path, getPath, label, labelClassName = '', title = 'Yolu kopyala', testId, onError }: Props) {
  const [copied, setCopied] = useState(false)
  if (!path && !getPath) return null

  const copy = async () => {
    try {
      const text = getPath ? await getPath() : path ?? ''
      if (!text) return
      await navigator.clipboard?.writeText(text)
      setCopied(true)
      setTimeout(() => setCopied(false), 1200)
    } catch (e) {
      onError?.(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <button
      type="button"
      onClick={copy}
      data-testid={testId}
      title={copied ? 'Kopyalandı' : path ? `${title}: ${displayPath(path)}` : title}
      className={PATH_ACTION_CLS}
    >
      {copied ? <Check size={14} className="text-[var(--color-success)]" /> : <Copy size={14} />}
      {label && <span className={labelClassName}>{copied ? 'Kopyalandı' : label}</span>}
    </button>
  )
}

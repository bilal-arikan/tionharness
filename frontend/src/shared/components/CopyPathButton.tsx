import { Copy } from 'lucide-react'
import { displayPath } from '@/shared/lib/paths'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { toast } from './toastStore'

// Shared visual language for the path actions used across the app so they
// always look identical: a compact bordered icon-only button that dims to
// accent on hover. The path itself lives in the tooltip, not in a label.
const PATH_ACTION_CLS =
  'flex shrink-0 items-center justify-center rounded-md border border-[var(--color-border)] px-2 py-1 text-xs text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] disabled:opacity-50'

interface Props {
  /** A ready path string to copy. */
  path?: string
  /** Lazily fetch the path (e.g. from an API) when clicked; used when the path
   *  is only known server-side. Takes precedence over `path` when both are set. */
  getPath?: () => Promise<string>
  title?: string
  testId?: string
  onError?: (msg: string) => void
}

// CopyPathButton copies a filesystem path to the clipboard, surfacing success
// through the app-wide toast (single feedback channel). Standardised look via
// PATH_ACTION_CLS.
export function CopyPathButton({ path, getPath, title = 'Yolu kopyala', testId, onError }: Props) {
  if (!path && !getPath) return null

  const copy = async () => {
    try {
      const text = getPath ? await getPath() : (path ?? '')
      if (!text) return
      // copyToClipboard falls back to a manual-copy prompt when the browser
      // blocks programmatic copy (insecure LAN/HTTP context); it returns true
      // only on a real programmatic copy, so gate the toast on it.
      if (await copyToClipboard(text, 'Yolu kopyalayın (Ctrl+C, Enter):'))
        toast.info('Panoya kopyalandı')
    } catch (e) {
      onError?.(e instanceof Error ? e.message : String(e))
    }
  }

  return (
    <button
      type="button"
      onClick={copy}
      data-testid={testId}
      title={path ? `${title}: ${displayPath(path)}` : title}
      className={PATH_ACTION_CLS}
    >
      <Copy size={14} />
    </button>
  )
}

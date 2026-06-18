import { useState } from 'react'
import { Copy, Check } from 'lucide-react'
import { displayPath } from '../lib/paths'

// CopyPathButton is a compact icon button that copies a filesystem path to the
// clipboard, shown next to a "Klasörü aç" (reveal-in-Explorer) action so the user
// can grab the path instead of opening it. Briefly flips to a check on success.
export function CopyPathButton({ path, title = 'Yolu kopyala' }: { path?: string; title?: string }) {
  const [copied, setCopied] = useState(false)
  if (!path) return null
  const copy = () => {
    navigator.clipboard
      ?.writeText(path)
      .then(() => {
        setCopied(true)
        setTimeout(() => setCopied(false), 1200)
      })
      .catch(() => {})
  }
  return (
    <button
      type="button"
      onClick={copy}
      title={copied ? 'Kopyalandı' : `${title}: ${displayPath(path)}`}
      className="flex shrink-0 items-center justify-center rounded border border-[var(--color-border)] px-1.5 py-1 text-[var(--color-text-dim)] transition hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
    >
      {copied ? <Check size={12} className="text-[var(--color-success)]" /> : <Copy size={12} />}
    </button>
  )
}

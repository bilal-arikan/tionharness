import { useEffect, useState, type ButtonHTMLAttributes, type TextareaHTMLAttributes } from 'react'
import { Check, Copy, Eye, Maximize2, Minimize2, Pencil } from 'lucide-react'
import { Markdown } from '../markdown/Markdown'

// Textarea attributes we forward verbatim (placeholder, rows, maxLength,
// onKeyDown, autoFocus, data-testid, …). value/onChange are typed explicitly.
type TextareaProps = Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'value' | 'onChange'>

interface Props extends TextareaProps {
  value: string
  onChange: (value: string) => void
  /** Render the textarea with a monospace font (prompts, templates, skill body). */
  mono?: boolean
  /** Extra classes merged onto the inner <textarea> (e.g. min-height overrides). */
  textareaClassName?: string
}

// PromptEditor is a markdown-aware textarea: a thin toolbar adds an Edit/Preview
// toggle (rendered through the shared <Markdown>), a one-click Copy button and a
// fullscreen toggle. It owns its border/background so call sites just swap a bare
// <textarea> for it.
export function PromptEditor({
  value,
  onChange,
  rows = 4,
  mono = false,
  textareaClassName = '',
  className = '',
  ...rest
}: Props) {
  const [preview, setPreview] = useState(false)
  const [copied, setCopied] = useState(false)
  const [full, setFull] = useState(false)

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(value)
      setCopied(true)
      setTimeout(() => setCopied(false), 1500)
    } catch {
      // Clipboard may be unavailable (insecure context) — fail silently.
    }
  }

  // Escape exits fullscreen; lock body scroll while the overlay is open.
  useEffect(() => {
    if (!full) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setFull(false)
    }
    document.addEventListener('keydown', onKey)
    const prev = document.body.style.overflow
    document.body.style.overflow = 'hidden'
    return () => {
      document.removeEventListener('keydown', onKey)
      document.body.style.overflow = prev
    }
  }, [full])

  // Toolbar is shared between the inline editor and the fullscreen overlay; the
  // fullscreen flag swaps the expand icon for a collapse icon.
  const toolbar = (fullscreen: boolean) => (
    <div className="flex items-center justify-end gap-0.5 border-b border-[var(--color-border)] bg-[var(--color-surface)] px-1 py-0.5">
      <ToolbarButton active={!preview} onClick={() => setPreview(false)} title="Düzenle" aria-label="Düzenle">
        <Pencil className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton active={preview} onClick={() => setPreview(true)} title="Önizleme" aria-label="Markdown önizleme">
        <Eye className="h-3.5 w-3.5" />
      </ToolbarButton>
      <span className="mx-0.5 h-3.5 w-px bg-[var(--color-border)]" />
      <ToolbarButton onClick={copy} title="Panoya kopyala" aria-label="Panoya kopyala">
        {copied ? <Check className="h-3.5 w-3.5 text-[var(--color-success,#22c55e)]" /> : <Copy className="h-3.5 w-3.5" />}
      </ToolbarButton>
      <ToolbarButton
        onClick={() => setFull(!fullscreen)}
        title={fullscreen ? 'Tam ekrandan çık (Esc)' : 'Tam ekran'}
        aria-label={fullscreen ? 'Tam ekrandan çık' : 'Tam ekran'}
      >
        {fullscreen ? <Minimize2 className="h-3.5 w-3.5" /> : <Maximize2 className="h-3.5 w-3.5" />}
      </ToolbarButton>
    </div>
  )

  // Body renders either the markdown preview or the editable textarea. In
  // fullscreen the content area grows to fill the overlay; inline it is bounded.
  const body = (fullscreen: boolean) =>
    preview ? (
      <div
        className={
          fullscreen ? 'flex-1 overflow-y-auto px-4 py-3' : 'max-h-[60vh] overflow-y-auto px-2.5 py-2'
        }
        style={fullscreen ? undefined : { minHeight: `${rows * 1.5}rem` }}
      >
        {value.trim() ? (
          <Markdown>{value}</Markdown>
        ) : (
          <p className="text-xs italic text-[var(--color-text-dim)]">Önizlenecek içerik yok.</p>
        )}
      </div>
    ) : (
      <textarea
        value={value}
        onChange={(e) => onChange(e.target.value)}
        rows={fullscreen ? undefined : rows}
        className={`w-full bg-transparent px-2.5 py-2 text-sm outline-none ${
          fullscreen ? 'flex-1 resize-none px-4 py-3' : 'resize-y'
        } ${mono ? 'font-mono' : ''} ${textareaClassName}`}
        {...rest}
      />
    )

  return (
    <>
      <div
        className={`overflow-hidden rounded border border-[var(--color-border)] bg-[var(--color-bg)] focus-within:border-[var(--color-accent)] ${className}`}
      >
        {toolbar(false)}
        {body(false)}
      </div>

      {full && (
        <div
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 sm:p-8"
          onClick={() => setFull(false)}
        >
          <div
            role="dialog"
            aria-modal="true"
            aria-label="Tam ekran düzenleyici"
            className="flex h-full w-full max-w-4xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-bg)] shadow-xl"
            onClick={(e) => e.stopPropagation()}
          >
            {toolbar(true)}
            {body(true)}
          </div>
        </div>
      )}
    </>
  )
}

function ToolbarButton({
  active = false,
  children,
  ...props
}: { active?: boolean } & ButtonHTMLAttributes<HTMLButtonElement>) {
  return (
    <button
      type="button"
      className={`rounded p-1 transition hover:bg-[var(--color-surface-2)] hover:text-[var(--color-text)] ${
        active ? 'text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'
      }`}
      {...props}
    >
      {children}
    </button>
  )
}

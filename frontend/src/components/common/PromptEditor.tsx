import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'
import { Check, Columns2, Copy, Eye, Maximize2, Minimize2, Pencil } from 'lucide-react'
import { Markdown } from '../markdown/Markdown'

// Textarea attributes we forward verbatim (placeholder, rows, maxLength,
// onKeyDown, autoFocus, data-testid, …). value/onChange are typed explicitly.
type TextareaProps = Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'value' | 'onChange'>

// Below this rendered width the split (edit | preview) view is too cramped, so it
// is not offered as a default; the user can still toggle it on manually.
const SPLIT_MIN_WIDTH = 640

type ViewMode = 'edit' | 'preview' | 'split'

interface Props extends TextareaProps {
  value: string
  onChange: (value: string) => void
  /** Render the textarea with a monospace font (prompts, templates, skill body). */
  mono?: boolean
  /** Extra classes merged onto the inner <textarea> (e.g. min-height overrides). */
  textareaClassName?: string
}

// PromptEditor is a markdown-aware textarea: a thin toolbar toggles between three
// views — Edit, Preview (rendered through the shared <Markdown>) and Split (edit
// on the left, live preview on the right) — plus a one-click Copy and a
// fullscreen toggle. When the editor renders wide enough (≥ SPLIT_MIN_WIDTH) it
// starts in Split by default; narrower instances start in Edit. It owns its
// border/background so call sites just swap a bare <textarea> for it.
export function PromptEditor({
  value,
  onChange,
  rows = 4,
  mono = false,
  textareaClassName = '',
  className = '',
  ...rest
}: Props) {
  // null until the first width measurement picks the default (edit vs split).
  const [mode, setMode] = useState<ViewMode | null>(null)
  const [copied, setCopied] = useState(false)
  const [full, setFull] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)

  // Pick the initial view from the rendered width: wide editors open in Split,
  // narrow ones in Edit. Runs once (mode stays null until then); afterwards the
  // toolbar is user-controlled and never auto-overridden.
  useLayoutEffect(() => {
    if (mode !== null) return
    const w = rootRef.current?.offsetWidth ?? 0
    setMode(w >= SPLIT_MIN_WIDTH ? 'split' : 'edit')
  }, [mode])

  const m: ViewMode = mode ?? 'edit'

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
      <ToolbarButton active={m === 'edit'} onClick={() => setMode('edit')} title="Düzenle" aria-label="Düzenle">
        <Pencil className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton active={m === 'preview'} onClick={() => setMode('preview')} title="Önizleme" aria-label="Markdown önizleme">
        <Eye className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton active={m === 'split'} onClick={() => setMode('split')} title="Böl (düzenle + önizleme)" aria-label="Bölünmüş görünüm">
        <Columns2 className="h-3.5 w-3.5" />
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

  const editArea = (fullscreen: boolean, half: boolean) => (
    <textarea
      value={value}
      onChange={(e) => onChange(e.target.value)}
      // In split ("half") and fullscreen the textarea fills the pane height
      // (flex-1) so the edit side matches the preview instead of staying at the
      // caller's short `rows`, which left it cut off next to a tall preview.
      rows={fullscreen || half ? undefined : rows}
      className={`w-full bg-transparent px-2.5 py-2 text-sm outline-none ${
        fullscreen ? 'flex-1 resize-none px-4 py-3' : half ? 'h-full flex-1 resize-none' : 'resize-y'
      } ${mono ? 'font-mono' : ''} ${textareaClassName}`}
      {...rest}
    />
  )

  // half=true → rendered inside the split row (fills the row's fixed height via
  // h-full). Single-pane preview (half=false) instead gets the SAME comfortable
  // min-height as split so clicking "Önizleme" doesn't collapse the box to the
  // content height. Fullscreen fills the overlay.
  const previewArea = (fullscreen: boolean, half: boolean) => (
    <div
      className={
        'min-w-0 break-words overflow-y-auto ' +
        (fullscreen ? 'flex-1 px-4 py-3' : half ? 'h-full px-2.5 py-2' : 'min-h-[26rem] max-h-[65vh] px-2.5 py-2')
      }
    >
      {value.trim() ? (
        <Markdown>{value}</Markdown>
      ) : (
        <p className="text-xs italic text-[var(--color-text-dim)]">Önizlenecek içerik yok.</p>
      )}
    </div>
  )

  // Body renders the active view. Split lays the editor and the live preview
  // side by side; the single-pane views fill the width.
  const body = (fullscreen: boolean) => {
    if (m === 'split') {
      return (
        // Non-fullscreen split gets an explicit, comfortable height (capped to the
        // viewport) so BOTH panes fill it and scroll internally — otherwise the
        // edit side collapsed to the short `rows` while the preview grew tall.
        <div
          className={`flex divide-x divide-[var(--color-border)] ${
            fullscreen ? 'min-h-0 flex-1' : 'h-[26rem] max-h-[65vh]'
          }`}
        >
          <div className="flex w-1/2 min-w-0 flex-col">{editArea(fullscreen, true)}</div>
          <div className="flex w-1/2 min-w-0 flex-col bg-[var(--color-surface)]/30">{previewArea(fullscreen, true)}</div>
        </div>
      )
    }
    return m === 'preview' ? previewArea(fullscreen, false) : editArea(fullscreen, false)
  }

  return (
    <>
      <div
        ref={rootRef}
        className={`w-full min-w-0 overflow-hidden rounded border border-[var(--color-border)] bg-[var(--color-bg)] focus-within:border-[var(--color-accent)] ${className}`}
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
            className="flex h-full w-full max-w-5xl flex-col overflow-hidden rounded-xl border border-[var(--color-border)] bg-[var(--color-bg)] shadow-xl"
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

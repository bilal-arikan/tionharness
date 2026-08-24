import {
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
  type ButtonHTMLAttributes,
  type TextareaHTMLAttributes,
} from 'react'
import { Columns2, Copy, Eye, Maximize2, Minimize2, Pencil } from 'lucide-react'
import { Markdown } from './markdown/Markdown'
import { copyToClipboard } from '@/shared/lib/clipboard'
import { toast } from './toastStore'

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
  /**
   * Size the editor to its CONTENT instead of a fixed box: short prompts render
   * short, long ones grow up to `autoSizeMax` and then scroll. Applies to the
   * inline Edit, Preview and Split views; fullscreen always fills the overlay.
   * ON by default (app-wide rule); pass false to restore the fixed-box layout.
   */
  autoSize?: boolean
  /** Max content-driven height in px when `autoSize` is on (default 320). */
  autoSizeMax?: number
}

// Floor for the auto-sized editor so an empty field still presents a usable
// click/typing target rather than collapsing to a single line. The effective
// floor also honors the caller's `rows` (an authoring surface asking for 12
// rows must not collapse to 72px when empty).
const AUTO_SIZE_MIN = 72

// Approximate rendered height of `rows` text-sm lines incl. vertical padding,
// used for the rows-aware autoSize floor.
const rowsFloor = (rows: number) => rows * 20 + 18

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
  autoSize = true,
  autoSizeMax = 320,
  ...rest
}: Props) {
  // null until the first width measurement picks the default (edit vs split).
  const [mode, setMode] = useState<ViewMode | null>(null)
  const [full, setFull] = useState(false)
  const rootRef = useRef<HTMLDivElement>(null)
  // autoSize measurement targets — the INLINE textarea and split row only (the
  // fullscreen overlay always fills, so it is never measured or resized).
  const taRef = useRef<HTMLTextAreaElement>(null)
  const splitRef = useRef<HTMLDivElement>(null)

  // Pick the initial view from the rendered width: wide editors open in Split,
  // narrow ones in Edit. Runs once (mode stays null until then); afterwards the
  // toolbar is user-controlled and never auto-overridden.
  useLayoutEffect(() => {
    if (mode !== null) return
    const w = rootRef.current?.offsetWidth ?? 0
    setMode(w >= SPLIT_MIN_WIDTH ? 'split' : 'edit')
  }, [mode])

  const m: ViewMode = mode ?? 'edit'

  // Content-driven height: measure the textarea's natural scrollHeight and clamp
  // it to [AUTO_SIZE_MIN, autoSizeMax]. In Edit view the height lands on the
  // textarea itself; in Split it lands on the row so both panes stay equal and
  // the taller preview scrolls internally.
  useLayoutEffect(() => {
    if (!autoSize) return
    const ta = taRef.current
    if (!ta) return
    ta.style.height = 'auto'
    const floor = Math.max(AUTO_SIZE_MIN, rowsFloor(rows))
    const h = Math.max(floor, Math.min(ta.scrollHeight + 2, Math.max(autoSizeMax, floor)))
    if (m === 'split') {
      ta.style.height = '' // height comes from the row via the h-full class
      if (splitRef.current) splitRef.current.style.height = `${h}px`
    } else {
      ta.style.height = `${h}px`
    }
  }, [autoSize, autoSizeMax, rows, value, m, full])

  const copy = async () => {
    if (await copyToClipboard(value)) toast.info('Panoya kopyalandı')
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
      <ToolbarButton
        active={m === 'edit'}
        onClick={() => setMode('edit')}
        title="Düzenle"
        aria-label="Düzenle"
      >
        <Pencil className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        active={m === 'preview'}
        onClick={() => setMode('preview')}
        title="Önizleme"
        aria-label="Markdown önizleme"
      >
        <Eye className="h-3.5 w-3.5" />
      </ToolbarButton>
      <ToolbarButton
        active={m === 'split'}
        onClick={() => setMode('split')}
        title="Böl (düzenle + önizleme)"
        aria-label="Bölünmüş görünüm"
      >
        <Columns2 className="h-3.5 w-3.5" />
      </ToolbarButton>
      <span className="mx-0.5 h-3.5 w-px bg-[var(--color-border)]" />
      <ToolbarButton onClick={copy} title="Panoya kopyala" aria-label="Panoya kopyala">
        <Copy className="h-3.5 w-3.5" />
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
      // Measurement target for autoSize — inline instance only; the fullscreen
      // overlay renders a second textarea that must never steal the ref.
      ref={fullscreen ? undefined : taRef}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      // In split ("half") and fullscreen the textarea fills the pane height
      // (flex-1) so the edit side matches the preview instead of staying at the
      // caller's short `rows`, which left it cut off next to a tall preview.
      rows={fullscreen || half ? undefined : rows}
      className={`w-full bg-transparent px-2.5 py-2 text-sm outline-none ${
        fullscreen
          ? 'flex-1 resize-none px-4 py-3'
          : half
            ? 'h-full flex-1 resize-none'
            : autoSize
              ? 'resize-none'
              : 'resize-y'
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
        (fullscreen
          ? 'flex-1 px-4 py-3'
          : half
            ? 'h-full px-2.5 py-2'
            : autoSize
              ? 'px-2.5 py-2'
              : 'min-h-[26rem] max-h-[65vh] px-2.5 py-2')
      }
      // autoSize single-pane preview: shrink-wrap the content up to the cap.
      style={!fullscreen && !half && autoSize ? { maxHeight: autoSizeMax } : undefined}
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
        // Non-fullscreen split gets an explicit height so BOTH panes fill it and
        // scroll internally — otherwise the edit side collapsed to the short
        // `rows` while the preview grew tall. With autoSize the height is set
        // imperatively from the measured content (clamped); otherwise it is the
        // fixed comfortable box.
        <div
          ref={fullscreen ? undefined : splitRef}
          className={`flex divide-x divide-[var(--color-border)] ${
            fullscreen ? 'min-h-0 flex-1' : autoSize ? '' : 'h-[26rem] max-h-[65vh]'
          }`}
        >
          <div className="flex w-1/2 min-w-0 flex-col">{editArea(fullscreen, true)}</div>
          <div className="flex w-1/2 min-w-0 flex-col bg-[var(--color-surface)]/30">
            {previewArea(fullscreen, true)}
          </div>
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
          className="fixed inset-0 z-50 flex items-center justify-center bg-black/50 p-4 sm:p-8 max-md:p-0 max-md:[&>*]:!max-w-none max-md:[&>*]:!rounded-none"
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

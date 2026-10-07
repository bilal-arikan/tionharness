import { useEffect, useRef, useState } from 'react'
import type { KeyboardEvent, MouseEvent, ReactNode } from 'react'
import { createPortal } from 'react-dom'
import { Info } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { isShortHint } from '@/shared/lib/hint'

type Mode =
  // short text inline (dim, small) right where the (ⓘ) would sit; long → (ⓘ)
  | 'auto'
  // always the (ⓘ), e.g. in tight rows where inline text would not fit
  | 'icon'
  // only a long text renders (as the (ⓘ)); the caller shows a short one itself,
  // typically as a <FieldHint> under the control
  | 'long'

// InfoPopover is the app-wide affordance for explanatory text: short notes stay
// visible, long descriptions and "how this works" notes sit behind an (ⓘ) next to
// the label/heading they explain until the user asks for them.
export function InfoPopover({
  text,
  mode = 'auto',
  ...rest
}: {
  text: ReactNode
  mode?: Mode
  label?: string
  // Which edge of the trigger the bubble aligns to.
  align?: 'left' | 'right'
  // Accepted for older call sites; every bubble is viewport-positioned now.
  fixed?: boolean
}) {
  if (mode !== 'icon' && isShortHint(text)) {
    if (mode === 'long') return null
    return (
      <span className="min-w-0 text-[11px] font-normal normal-case leading-snug tracking-normal text-[var(--color-text-dim)]">
        {text}
      </span>
    )
  }
  return <InfoButton text={text} {...rest} />
}

// FieldHint is a field's short hint as a line under its control — the
// counterpart of <InfoPopover mode="long"> in the field's label row.
export function FieldHint({
  text,
  className = 'text-xs',
}: {
  text?: ReactNode
  className?: string
}) {
  if (!isShortHint(text)) return null
  return <span className={`block text-[var(--color-text-dim)] ${className}`}>{text}</span>
}

// InfoButton is the (ⓘ) itself. Closes on blur (click-away), Escape, or when
// anything scrolls.
//
// The trigger is a focusable span (role="button"), not a <button>: it is safe to
// drop inside a <label> (a span is not a labelable control, so the label keeps
// pointing at its input) and its click never activates the surrounding label or
// row. The bubble is portalled to <body> in viewport coordinates, so scrolling or
// clipping ancestors never cut it off.
function InfoButton({
  text,
  label,
  align = 'left',
}: {
  text: ReactNode
  label?: string
  align?: 'left' | 'right'
}) {
  const { t } = useTranslation('sharedUi')
  const resolvedLabel = label ?? t('actions.info')
  const [pos, setPos] = useState<{ top: number; left: number; right: number } | null>(null)
  const ref = useRef<HTMLSpanElement>(null)
  const bubbleRef = useRef<HTMLSpanElement>(null)
  const open = pos !== null

  useEffect(() => {
    if (!open) return
    const close = () => setPos(null)
    // A long note scrolls inside its own bubble; only outside scrolls detach it.
    const onScroll = (e: Event) => {
      if (!bubbleRef.current?.contains(e.target as Node)) close()
    }
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', close)
    return () => {
      window.removeEventListener('scroll', onScroll, true)
      window.removeEventListener('resize', close)
    }
  }, [open])

  const toggle = () => {
    if (open || !ref.current) {
      setPos(null)
      return
    }
    const r = ref.current.getBoundingClientRect()
    setPos({ top: r.bottom + 6, left: r.left, right: r.right })
  }

  const onClick = (e: MouseEvent) => {
    // Inside a <label> or a clickable row the click must not reach it.
    e.preventDefault()
    e.stopPropagation()
    toggle()
  }

  const onKeyDown = (e: KeyboardEvent) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      e.stopPropagation()
      toggle()
    } else if (e.key === 'Escape' && open) {
      e.stopPropagation()
      setPos(null)
    }
  }

  // The bubble is clamped into the viewport. Its height isn't known before it is
  // positioned, so it is capped at MAX_H (scrolling past it) and the vertical
  // clamp reserves exactly that.
  const MAX_H = 260
  const WIDTH = 320
  const style = pos
    ? {
        top: Math.min(pos.top, Math.max(8, window.innerHeight - MAX_H - 8)),
        left: Math.max(
          8,
          Math.min(align === 'right' ? pos.right - WIDTH : pos.left, window.innerWidth - WIDTH - 8),
        ),
        maxHeight: MAX_H,
      }
    : undefined

  return (
    <span
      ref={ref}
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={onKeyDown}
      onBlur={() => setPos(null)}
      title={resolvedLabel}
      aria-label={resolvedLabel}
      aria-expanded={open}
      className={`inline-flex h-4 w-4 shrink-0 cursor-pointer items-center justify-center rounded-full align-middle transition hover:text-[var(--color-accent)] focus-visible:outline focus-visible:outline-1 focus-visible:outline-[var(--color-accent)] ${
        open ? 'text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'
      }`}
    >
      <Info size={13} />
      {open &&
        createPortal(
          <span
            ref={bubbleRef}
            role="tooltip"
            style={style}
            className="fixed z-[100] block w-80 max-w-[calc(100vw-16px)] overflow-y-auto whitespace-pre-line rounded-md border border-[var(--color-border)] bg-[var(--color-surface)] px-3 py-2 text-left text-[11px] font-normal normal-case leading-relaxed tracking-normal text-[var(--color-text-dim)] shadow-[var(--shadow-lg)]"
            // Keep focus on the trigger while the bubble is used (e.g. its
            // scrollbar) so onBlur doesn't close it mid-read, and stop clicks:
            // React bubbles portal events through the trigger's onClick.
            onMouseDown={(e) => e.preventDefault()}
            onClick={(e) => e.stopPropagation()}
          >
            {text}
          </span>,
          document.body,
        )}
    </span>
  )
}

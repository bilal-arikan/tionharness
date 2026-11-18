import { useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { ChevronDown, ChevronUp } from 'lucide-react'
import { useTranslation } from 'react-i18next'

// Collapsed height: about nine lines of body text, enough to show what the
// message is about without it filling the screen.
const COLLAPSED_PX = 208
// A body only a little taller than the cap is shown whole: folding away two
// lines costs a click and saves nothing.
const SLACK_PX = 64

interface Props {
  children: ReactNode
}

// CollapsibleBody clamps a long message body and adds a "show all" toggle. Used
// for agent-authored briefs, which can run to a screenful and push the actual
// reply out of view. Short bodies render untouched (no toggle, no clamp).
export function CollapsibleBody({ children }: Props) {
  const { t } = useTranslation('chat')
  const ref = useRef<HTMLDivElement>(null)
  const [overflows, setOverflows] = useState(false)
  const [open, setOpen] = useState(false)

  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => setOverflows(el.scrollHeight > COLLAPSED_PX + SLACK_PX)
    measure()
    // Markdown (images, code blocks) can grow after the first paint.
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])

  const clamped = overflows && !open
  return (
    <div>
      <div
        ref={ref}
        data-testid="collapsible-body"
        data-clamped={clamped || undefined}
        className={clamped ? 'overflow-hidden' : undefined}
        style={clamped ? { maxHeight: COLLAPSED_PX } : undefined}
      >
        {children}
      </div>
      {overflows && (
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          className="mt-1.5 flex items-center gap-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-text)]"
        >
          {open ? <ChevronUp size={13} /> : <ChevronDown size={13} />}
          {open ? t('collapsible.showLess') : t('collapsible.showAll')}
        </button>
      )}
    </div>
  )
}

import { useCallback, useEffect, useRef, useState } from 'react'
import { createPortal } from 'react-dom'
import { Info } from 'lucide-react'

interface Props {
  // Other nodes in the flow, for the {{node.<id>}} entries. Ignored in 'seed' context.
  nodeRefs: { id: string; title: string }[]
  // Called with the placeholder text to append to the target field.
  onInsert: (text: string) => void
  // 'node' = a node's prompt/template (full var set + node refs).
  // 'seed' = the flow's run input (only date/time resolve there; shows an intro note).
  context?: 'node' | 'seed'
}

const BUBBLE_W = 320

// FlowVarsButton is an ℹ️ toggle that reveals the template placeholders usable in a
// flow field inside a floating BALLOON (a fixed-positioned popover rendered via a
// portal to <body>). Using a portal means the balloon is never clipped by an
// overflow-scroll ancestor (e.g. the node-editor modal body). The balloon flips
// above/below the button depending on where it sits in the viewport.
export function FlowVarsButton({ nodeRefs, onInsert, context = 'node' }: Props) {
  const [open, setOpen] = useState(false)
  const btnRef = useRef<HTMLButtonElement>(null)
  const [pos, setPos] = useState<{ left: number; top: number; placement: 'top' | 'bottom' } | null>(
    null,
  )

  const nodeVars: { name: string; desc: string }[] = [
    { name: '{{input}}', desc: 'Akışın girdisi (RunFlow input)' },
    { name: '{{last}}', desc: 'En son çalışan node’un çıktısı' },
    { name: '{{date}}', desc: 'Geçerli tarih (2026-07-04)' },
    { name: '{{time}}', desc: 'Geçerli saat (23:43)' },
    { name: '{{datetime}}', desc: 'Tarih + saat' },
  ]
  // The run input is the seed value (bound to {{input}} in nodes); {{last}}/{{node.<id>}}
  // have no prior output at seed time, so only date/time are offered here.
  const seedVars: { name: string; desc: string }[] = [
    { name: '{{date}}', desc: 'Geçerli tarih (2026-07-04)' },
    { name: '{{time}}', desc: 'Geçerli saat (23:43)' },
    { name: '{{datetime}}', desc: 'Tarih + saat' },
  ]
  const statics = context === 'seed' ? seedVars : nodeVars
  const showNodeRefs = context === 'node' && nodeRefs.length > 0

  // Compute the balloon anchor from the button's viewport rect. Opens above the
  // button when it sits in the lower half of the screen, else below.
  const reposition = useCallback(() => {
    const el = btnRef.current
    if (!el) return
    const r = el.getBoundingClientRect()
    const placement: 'top' | 'bottom' = r.top > window.innerHeight / 2 ? 'top' : 'bottom'
    const left = Math.max(8, Math.min(r.left, window.innerWidth - BUBBLE_W - 8))
    const top = placement === 'top' ? r.top - 8 : r.bottom + 8
    setPos({ left, top, placement })
  }, [])

  // While open, keep the balloon anchored on scroll/resize; Escape closes it.
  useEffect(() => {
    if (!open) return
    reposition()
    const onScroll = () => reposition()
    const onKey = (e: KeyboardEvent) => e.key === 'Escape' && setOpen(false)
    window.addEventListener('scroll', onScroll, true)
    window.addEventListener('resize', onScroll)
    window.addEventListener('keydown', onKey)
    return () => {
      window.removeEventListener('scroll', onScroll, true)
      window.removeEventListener('resize', onScroll)
      window.removeEventListener('keydown', onKey)
    }
  }, [open, reposition])

  const insert = (name: string) => {
    onInsert(name)
    setOpen(false)
  }

  const row = (name: string, desc: string, truncate = false) => (
    <button
      key={name}
      type="button"
      onClick={() => insert(name)}
      className="flex w-full items-baseline gap-2 rounded px-1.5 py-1 text-left transition hover:bg-[var(--color-surface-2)]"
      title="Alana ekle"
    >
      <code className="shrink-0 rounded bg-[var(--color-accent-soft)] px-1 py-0.5 font-mono text-[11px] text-[var(--color-accent)]">
        {name}
      </code>
      <span className={`text-[11px] text-[var(--color-text-dim)]${truncate ? ' truncate' : ''}`}>
        {desc}
      </span>
    </button>
  )

  return (
    <>
      <button
        ref={btnRef}
        type="button"
        onClick={() => setOpen((v) => !v)}
        className={`rounded p-0.5 transition hover:text-[var(--color-accent)] ${open ? 'text-[var(--color-accent)]' : 'text-[var(--color-text-dim)]'}`}
        title="Kullanılabilir değişkenler"
        aria-label="Kullanılabilir değişkenler"
      >
        <Info size={13} />
      </button>
      {open &&
        pos &&
        createPortal(
          <>
            {/* Click-away backdrop. */}
            <div className="fixed inset-0 z-[60]" onClick={() => setOpen(false)} />
            <div
              className="fixed z-[61] w-[320px] max-w-[calc(100vw-16px)] rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)] p-2 shadow-xl"
              style={
                pos.placement === 'top'
                  ? { left: pos.left, bottom: window.innerHeight - pos.top }
                  : { left: pos.left, top: pos.top }
              }
            >
              <div className="mb-1 px-1 text-[11px] font-semibold text-[var(--color-text-dim)]">
                {context === 'seed'
                  ? 'Değişkenler (tıkla → ekle)'
                  : 'Node’lar arası değişkenler (tıkla → ekle)'}
              </div>
              {context === 'seed' && (
                <div className="mb-1 px-1 text-[11px] text-[var(--color-text-dim)]">
                  Buraya yazdığın metin akışta{' '}
                  <code className="rounded bg-[var(--color-accent-soft)] px-1 font-mono text-[var(--color-accent)]">
                    {'{{input}}'}
                  </code>{' '}
                  olur.
                </div>
              )}
              <div className="max-h-64 overflow-y-auto">
                {statics.map((v) => row(v.name, v.desc))}
                {showNodeRefs && (
                  <div className="mt-1 border-t border-[var(--color-border)] pt-1">
                    <div className="mb-0.5 px-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)] opacity-70">
                      Belirli node çıktısı
                    </div>
                    {nodeRefs.map((n) => row(`{{node.${n.id}}}`, n.title || n.id, true))}
                  </div>
                )}
              </div>
            </div>
          </>,
          document.body,
        )}
    </>
  )
}

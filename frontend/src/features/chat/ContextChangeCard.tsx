import { useState } from 'react'
import { ChevronRight, Plus, Minus } from 'lucide-react'
import type { TurnStep, ContextArea } from '@/types'
import { STEP_KIND_MAP } from '@/shared/stepKinds'

const HeaderIcon = STEP_KIND_MAP.context_change.Icon

interface Props {
  step: TurnStep
}

// ContextChangeCard renders a prompt-epoch drift notice: the session's frozen
// static context (persona / instructions / skills / tools) changed mid-session,
// but the cached prefix still ships the session-start snapshot to preserve the
// prompt cache. Collapsed to a one-line summary; expands to the per-block
// added/removed diff. The change fully applies on the next /refresh-context.
export function ContextChangeCard({ step }: Props) {
  const [open, setOpen] = useState(false)
  const areas = step.areas ?? []
  const added = step.added ?? 0
  const removed = step.removed ?? 0
  const summary = step.text?.trim() || 'Statik bağlam değişti'
  return (
    <div className="my-0.5 rounded-md border border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_7%,transparent)] text-xs">
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-[var(--color-accent)]"
      >
        <HeaderIcon size={14} className="shrink-0" />
        <span className="min-w-0 flex-1 truncate font-medium">{summary}</span>
        {added > 0 && (
          <span className="shrink-0 font-mono text-[10px] text-[var(--color-success)]">
            +{added}
          </span>
        )}
        {removed > 0 && (
          <span className="shrink-0 font-mono text-[10px] text-[var(--color-danger)]">
            -{removed}
          </span>
        )}
        {areas.length > 0 && (
          <ChevronRight
            size={13}
            className={`shrink-0 opacity-70 transition-transform ${open ? 'rotate-90' : ''}`}
          />
        )}
      </button>
      {open && areas.length > 0 && (
        <div className="border-t border-[color-mix(in_srgb,var(--color-accent)_18%,transparent)] px-2 py-1.5">
          {areas.map((a, i) => (
            <AreaRow key={i} area={a} />
          ))}
          <div className="mt-1.5 px-1 text-[10px] text-[var(--color-text-dim)]">
            Değişiklik bir sonraki bağlam yenilemesinde tam uygulanır —{' '}
            <span className="font-mono">/refresh-context</span> (veya compaction / boşta kalma).
          </div>
        </div>
      )}
    </div>
  )
}

// AreaRow is one changed block: a colored +/- header (its self-label) that
// expands to the changed paragraph body when it carries one.
function AreaRow({ area }: { area: ContextArea }) {
  const [open, setOpen] = useState(false)
  const hasBody = (area.lines?.length ?? 0) > 0
  const isAdd = area.kind === 'added'
  const tone = isAdd ? 'text-[var(--color-success)]' : 'text-[var(--color-danger)]'
  const Icon = isAdd ? Plus : Minus
  return (
    <div className="my-0.5">
      <button
        type="button"
        onClick={() => hasBody && setOpen((v) => !v)}
        className={`flex w-full items-center gap-1.5 rounded px-1 py-0.5 text-left ${hasBody ? 'hover:bg-[color-mix(in_srgb,var(--color-accent)_8%,transparent)]' : 'cursor-default'}`}
      >
        <Icon size={11} className={`shrink-0 ${tone}`} />
        <span className={`min-w-0 flex-1 truncate ${tone}`}>{area.label}</span>
        {hasBody && (
          <ChevronRight
            size={11}
            className={`shrink-0 opacity-60 transition-transform ${open ? 'rotate-90' : ''}`}
          />
        )}
      </button>
      {open && hasBody && (
        <pre
          className={`mt-0.5 overflow-x-auto rounded px-2 py-1 font-mono text-[10px] leading-snug ${
            isAdd
              ? 'bg-[color-mix(in_srgb,var(--color-success)_10%,transparent)]'
              : 'bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)]'
          }`}
        >
          {area.lines!.join('\n')}
        </pre>
      )}
    </div>
  )
}

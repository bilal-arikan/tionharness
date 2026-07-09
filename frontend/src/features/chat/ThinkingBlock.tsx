import { useState } from 'react'

interface Props {
  text: string
}

// ThinkingBlock renders the model's reasoning as a single-line, collapsible
// card — same shape as the tool ActivityCard so the turn reads consistently.
// Collapsed: icon + label + truncated one-line preview. Expanded: full,
// dimmed/italic reasoning.
export function ThinkingBlock({ text }: Props) {
  const [open, setOpen] = useState(false)
  const preview = text.replace(/\s+/g, ' ').trim()

  return (
    <div className="overflow-hidden rounded-lg bg-[var(--color-surface)] shadow-xl shadow-black/40">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 rounded-lg px-3 py-1.5 text-left text-xs hover:bg-[var(--color-surface-2)]"
      >
        <span className="shrink-0">💭</span>
        <span className="shrink-0 font-medium text-[var(--color-text)]">Düşünme</span>
        {!open && preview && (
          <span className="min-w-0 flex-1 truncate italic text-[var(--color-text-dim)]">
            {preview}
          </span>
        )}
        <span className="ml-auto shrink-0 opacity-50">{open ? '▾' : '▸'}</span>
      </button>

      {open && (
        <div className="px-3 pb-2 text-xs italic leading-relaxed whitespace-pre-wrap text-[var(--color-text-dim)]">
          {text}
        </div>
      )}
    </div>
  )
}

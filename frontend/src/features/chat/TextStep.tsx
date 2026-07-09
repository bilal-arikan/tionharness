import { useState } from 'react'
import { MessageSquare, ChevronDown, ChevronRight } from 'lucide-react'
import { Markdown } from '@/shared/components/markdown/Markdown'

interface Props {
  text: string
  onOpenFile?: (path: string) => void
}

// TextStep renders the model's intermediate narration (the "text" steps between
// tool calls) as a single-line, collapsible card — same shape as the tool
// ActivityCard and ThinkingBlock so the turn stays compact. Collapsed: a one-
// line truncated preview. Expanded: the full markdown.
export function TextStep({ text, onOpenFile }: Props) {
  const [open, setOpen] = useState(false)
  const preview = text.replace(/\s+/g, ' ').trim()

  return (
    <div className="overflow-hidden rounded-lg bg-[var(--color-surface)] shadow-xl shadow-black/40">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 rounded-lg px-3 py-1.5 text-left text-xs hover:bg-[var(--color-surface-2)]"
      >
        <MessageSquare size={14} className="shrink-0 text-[var(--color-text-dim)]" />
        <span className="shrink-0 font-medium text-[var(--color-text)]">Düşünce</span>
        {!open && preview && (
          <span className="min-w-0 flex-1 truncate text-[var(--color-text-dim)]">{preview}</span>
        )}
        <span className="ml-auto shrink-0 opacity-50">
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
      </button>

      {open && (
        <div className="px-3 pb-2 text-[var(--color-text-dim)]">
          <Markdown onOpenFile={onOpenFile}>{text}</Markdown>
        </div>
      )}
    </div>
  )
}

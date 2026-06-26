import { useState } from 'react'
import { ChevronDown, ChevronRight, Bot } from 'lucide-react'
import type { TurnStep } from '../../types'
import { TurnSteps } from './TurnSteps'

interface Props {
  step: TurnStep
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
}

// subInput is the parsed run_subagent call input (target + task) used for the
// collapsed header summary.
function subInput(input: unknown): { target: string; task: string } {
  if (input && typeof input === 'object') {
    const o = input as Record<string, unknown>
    return {
      target: typeof o.target === 'string' ? o.target : '',
      task: typeof o.task === 'string' ? o.task : '',
    }
  }
  return { target: '', task: '' }
}

// SubagentStep renders one run_subagent invocation as a collapsible nested-agent
// card: the target + task in the header, and on expand the subagent's own
// activity trace (its tool calls / thinking, gathered in an isolated context)
// plus its final reply. Mirrors the nested agent rows in External Agent chat.
export function SubagentStep({ step, onOpenFile, onOpenArtifact }: Props) {
  const [open, setOpen] = useState(false)
  const { target, task } = subInput(step.input)
  const sub = step.subSteps || []
  const reply = step.output || ''

  return (
    <div className="overflow-hidden rounded-md">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 rounded-md px-3 py-1 text-left text-xs hover:bg-[var(--color-surface-2)]"
      >
        <span className="shrink-0 text-[var(--color-accent)]">
          <Bot size={14} />
        </span>
        <span className="shrink-0 font-medium text-[var(--color-text)]">
          Alt-ajan{target ? ` · ${target}` : ''}
        </span>
        {task && (
          <span className="min-w-0 flex-1 truncate text-[var(--color-text-dim)]">{task}</span>
        )}
        {step.isError && <span className="shrink-0 text-[var(--color-danger)]">hata</span>}
        {sub.length > 0 && (
          <span className="shrink-0 text-[var(--color-text-dim)]">{sub.length} adım</span>
        )}
        <span className="ml-1 shrink-0 opacity-50">
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
      </button>

      {open && (
        <div className="space-y-2 px-3 pb-2 text-xs">
          {sub.length > 0 && (
            <div className="border-l-2 border-[var(--color-border)] pl-2">
              <TurnSteps steps={sub} onOpenFile={onOpenFile} onOpenArtifact={onOpenArtifact} />
            </div>
          )}
          {reply && (
            <div>
              <div className="mb-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                Sonuç
              </div>
              <pre
                className={`overflow-x-auto whitespace-pre-wrap rounded bg-[var(--color-bg)] p-2 ${
                  step.isError ? 'text-[var(--color-danger)]' : 'text-[var(--color-text)]'
                }`}
              >
                {reply.length > 4000 ? reply.slice(0, 4000) + '\n… (kırpıldı)' : reply}
              </pre>
            </div>
          )}
        </div>
      )}
    </div>
  )
}

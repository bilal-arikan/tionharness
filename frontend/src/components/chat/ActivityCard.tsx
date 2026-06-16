import { useState } from 'react'
import type { TurnStep } from '../../types'
import { toolMeta } from '../../lib/tools'
import { DiffView } from '../markdown/DiffView'
import { PathText } from './PathText'

interface Props {
  step: TurnStep
  onOpenFile?: (path: string) => void
}

// ActivityCard renders a single tool invocation as a compact, collapsible card:
// icon + label + one-line intent in the header, full input/output on expand.
// Edit/Write tools render their output as a diff. Mirrors the tool activity
// cards in External Agent chat.
export function ActivityCard({ step, onOpenFile }: Props) {
  const [open, setOpen] = useState(false)
  const meta = toolMeta(step.tool || '', step.input)
  const output = step.output || ''

  return (
    <div className="overflow-hidden rounded-lg border border-[var(--color-border)] bg-[var(--color-surface)]">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 px-3 py-1.5 text-left text-xs hover:bg-[var(--color-surface-2)]"
      >
        <span className="shrink-0">{meta.icon}</span>
        <span className="shrink-0 font-medium text-[var(--color-text)]">{meta.label}</span>
        {meta.summary && (
          <span className="min-w-0 flex-1 truncate text-[var(--color-text-dim)]">
            <PathText text={meta.summary} onOpenFile={onOpenFile} />
          </span>
        )}
        {step.isError && <span className="shrink-0 text-red-400">hata</span>}
        <span className="ml-auto shrink-0 opacity-50">{open ? '▾' : '▸'}</span>
      </button>

      {open && (
        <div className="space-y-2 border-t border-[var(--color-border)] px-3 py-2 text-xs">
          {step.input != null && (
            <div>
              <div className="mb-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                Girdi
              </div>
              <pre className="overflow-x-auto rounded bg-[var(--color-bg)] p-2 text-[var(--color-text-dim)]">
                {typeof step.input === 'string'
                  ? step.input
                  : JSON.stringify(step.input, null, 2)}
              </pre>
            </div>
          )}
          {output && (
            <div>
              <div className="mb-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                Çıktı
              </div>
              {meta.isDiff ? (
                <DiffView text={output} />
              ) : (
                <pre
                  className={`overflow-x-auto whitespace-pre-wrap rounded bg-[var(--color-bg)] p-2 ${
                    step.isError ? 'text-red-300' : 'text-[var(--color-text)]'
                  }`}
                >
                  {output.length > 4000 ? output.slice(0, 4000) + '\n… (kırpıldı)' : output}
                </pre>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}

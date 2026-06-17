import { useState } from 'react'
import type { ReactNode } from 'react'
import type { TurnStep } from '../../types'
import { toolMeta, isReadTool, toolBase } from '../../lib/tools'
import { parseDiff, looksLikeDiff, synthDiff } from '../../lib/diff'
import { DiffView } from '../markdown/DiffView'
import { PathText } from './PathText'

interface Props {
  step: TurnStep
  onOpenFile?: (path: string) => void
}

// headerBadge derives a compact right-aligned summary shown next to the tool
// label in the collapsed header: +added/−removed for diff-producing tools
// (edit/write) and a line count for file readers — so the at-a-glance card
// matches the native DiffCard without expanding it.
function headerBadge(step: TurnStep, diffText: string | null, output: string): ReactNode {
  if (diffText) {
    const { stats } = parseDiff(diffText)
    if (stats.added > 0 || stats.removed > 0) {
      return (
        <span className="flex shrink-0 gap-1.5 font-mono">
          <span className="text-green-400">+{stats.added}</span>
          <span className="text-red-400">−{stats.removed}</span>
        </span>
      )
    }
  }
  if (isReadTool(step.tool || '') && output.trim()) {
    const count = output.replace(/\n$/, '').split('\n').length
    return <span className="shrink-0 text-[var(--color-text-dim)]">{count} satır</span>
  }
  return null
}

// ActivityCard renders a single tool invocation as a compact, collapsible card:
// icon + label + one-line intent in the header, full input/output on expand.
// Edit/Write tools render their output as a diff. Mirrors the tool activity
// cards in External Agent chat.
export function ActivityCard({ step, onOpenFile }: Props) {
  const [open, setOpen] = useState(false)
  const meta = toolMeta(step.tool || '', step.input)
  const output = step.output || ''
  // The diff to show for edit/write tools: a real unified diff in the output if
  // present, otherwise synthesized from the tool input (claude-cli's Edit/Write
  // return only a confirmation message, so the +/- must come from old/new/content).
  const diffText = meta.isDiff
    ? looksLikeDiff(output)
      ? output
      : synthDiff(toolBase(step.tool || ''), step.input)
    : null
  const badge = headerBadge(step, diffText, output)

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
        {badge && <span className="ml-auto">{badge}</span>}
        <span className={`${badge ? 'ml-1' : 'ml-auto'} shrink-0 opacity-50`}>{open ? '▾' : '▸'}</span>
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
          {diffText ? (
            <div>
              <div className="mb-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                Değişiklik
              </div>
              <DiffView text={diffText} />
            </div>
          ) : (
            output && (
              <div>
                <div className="mb-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                  Çıktı
                </div>
                <pre
                  className={`overflow-x-auto whitespace-pre-wrap rounded bg-[var(--color-bg)] p-2 ${
                    step.isError ? 'text-red-300' : 'text-[var(--color-text)]'
                  }`}
                >
                  {output.length > 4000 ? output.slice(0, 4000) + '\n… (kırpıldı)' : output}
                </pre>
              </div>
            )
          )}
        </div>
      )}
    </div>
  )
}

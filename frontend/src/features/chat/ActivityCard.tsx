import { useState } from 'react'
import type { ReactNode } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'
import type { TurnStep } from '@/types'
import { toolMeta, isReadTool, toolBase } from './tools'
import { parseDiff, looksLikeDiff, synthDiff } from '@/shared/lib/diff'
import { DiffView } from '@/shared/components/markdown/DiffView'
import { Markdown } from '@/shared/components/markdown/Markdown'
import { PathText } from './PathText'
import { CommandProgramIcon } from './CommandProgramIcon'

interface Props {
  step: TurnStep
  onOpenFile?: (path: string) => void
}

// programHint returns a string to resolve a program brand icon from, so it can sit
// next to the tool icon: the command for a Bash/PowerShell step, or the interpreter
// language for a transform_data/run_code step (e.g. "python3" → Python). Else null.
function programHint(step: TurnStep): string | null {
  const base = toolBase(step.tool || '')
  const input = step.input as { command?: unknown; language?: unknown } | null
  if (base === 'bash' || base === 'powershell') {
    return typeof input?.command === 'string' ? input.command : null
  }
  if (base === 'transform_data' || base === 'run_code') {
    return typeof input?.language === 'string' ? input.language : null
  }
  return null
}

// headerBadge derives a compact right-aligned summary shown next to the tool
// label in the collapsed header: +added/−removed for diff-producing tools
// (edit/write) and a line count for file readers — so the at-a-glance card
// matches the native DiffCard without expanding it.
function headerBadge(step: TurnStep, diffText: string | null, output: string): ReactNode {
  if (diffText) {
    const { stats } = parseDiff(diffText)
    if (stats.added > 0 || stats.removed > 0) {
      // A failed Edit/Write still carries a synthetic +/- from its (un-applied)
      // input; painting it green/red reads as "the change went through". When
      // the step is in error, dim the counts so the user clearly sees these
      // were the intended — not the applied — line deltas.
      const addedCls = step.isError
        ? 'text-[var(--color-text-dim)]'
        : 'text-[var(--color-success)]'
      const removedCls = step.isError
        ? 'text-[var(--color-text-dim)]'
        : 'text-[var(--color-danger)]'
      return (
        <span className="flex shrink-0 gap-1.5 font-mono">
          <span className={addedCls}>+{stats.added}</span>
          <span className={removedCls}>−{stats.removed}</span>
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
  // A loaded skill's body is markdown (use_skill returns "# Skill: <slug>\n\n…").
  // Render it formatted rather than as a raw <pre> block when the card is expanded.
  const isSkill = toolBase(step.tool || '') === 'use_skill' && !step.isError
  const progHint = programHint(step)

  return (
    <div className="overflow-hidden rounded-md">
      <button
        onClick={() => setOpen((o) => !o)}
        className="flex w-full items-center gap-2 rounded-md px-3 py-1 text-left text-xs hover:bg-[var(--color-surface-2)]"
      >
        <meta.icon size={14} className="shrink-0 text-[var(--color-text-dim)]" />
        {progHint && <CommandProgramIcon command={progHint} />}
        <span className="shrink-0 font-medium text-[var(--color-text)]">{meta.label}</span>
        {meta.summary && (
          <span className="min-w-0 flex-1 truncate text-[var(--color-text-dim)]">
            <PathText text={meta.summary} onOpenFile={onOpenFile} />
          </span>
        )}
        {step.isError && <span className="shrink-0 text-[var(--color-danger)]">hata</span>}
        {badge && <span className="ml-auto">{badge}</span>}
        <span className={`${badge ? 'ml-1' : 'ml-auto'} shrink-0 opacity-50`}>
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
      </button>

      {open && (
        <div className="space-y-2 px-3 pb-2 text-xs">
          {step.input != null && !isSkill && (
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
          ) : isSkill && output.trim() ? (
            <div>
              <div className="mb-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                Skill
              </div>
              <div className="rounded bg-[var(--color-bg)] p-2">
                <Markdown onOpenFile={onOpenFile}>{output}</Markdown>
              </div>
            </div>
          ) : (
            output && (
              <div>
                <div className="mb-1 text-[10px] uppercase tracking-wide text-[var(--color-text-dim)]">
                  Çıktı
                </div>
                <pre
                  className={`overflow-x-auto whitespace-pre-wrap rounded bg-[var(--color-bg)] p-2 ${
                    step.isError ? 'text-[var(--color-danger)]' : 'text-[var(--color-text)]'
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

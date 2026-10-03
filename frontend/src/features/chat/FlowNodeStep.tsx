import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Bot, GitBranch, Puzzle, Square, Play, type LucideIcon } from 'lucide-react'
import type { TurnStep } from '@/types'
import { formatDurationMs } from '@/shared/lib/time'
import { STEP_KIND_MAP } from '@/shared/stepKinds'

interface Props {
  step: TurnStep
}

const TYPE_ICONS: Record<string, LucideIcon> = {
  input: Play,
  llm: Bot,
  route: GitBranch,
  transform: Puzzle,
  output: Square,
}

// FlowNodeStep is the stage marker of an evolving flow inside a chat turn: a
// thin accent-tinted row naming the stage, its status and duration; the
// finished card expands to the node's output excerpt. The live card (running)
// pulses until the stage ends.
export function FlowNodeStep({ step }: Props) {
  const { t } = useTranslation('chatStatus')
  const [open, setOpen] = useState(false)
  const type = step.target?.[0] ?? ''
  const nodeId = step.target?.[1] ?? ''
  const Icon = TYPE_ICONS[type] ?? STEP_KIND_MAP.flow_node.Icon
  const running = !!step.running
  const failed = !!step.isError
  const tone = failed
    ? 'border-[color-mix(in_srgb,var(--color-danger)_35%,transparent)] bg-[color-mix(in_srgb,var(--color-danger)_10%,transparent)] text-[var(--color-danger)]'
    : running
      ? 'border-[color-mix(in_srgb,var(--color-accent)_45%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_12%,transparent)] text-[var(--color-accent)]'
      : 'border-[color-mix(in_srgb,var(--color-accent)_22%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_6%,transparent)] text-[var(--color-text)]'
  const hasBody = !!step.output && !running
  return (
    <div className={`overflow-hidden rounded-md border text-xs ${tone}`}>
      <button
        type="button"
        onClick={() => hasBody && setOpen((o) => !o)}
        className={`flex w-full items-center gap-2 px-3 py-1.5 text-left ${hasBody ? 'hover:bg-[color-mix(in_srgb,var(--color-accent)_10%,transparent)]' : 'cursor-default'}`}
      >
        <Icon size={14} className={`shrink-0 ${running ? 'animate-pulse' : ''}`} />
        <span className="shrink-0 font-medium">{t('flowNode.stage')}</span>
        <span className="min-w-0 flex-1 truncate">
          {step.text || nodeId}
          {type && (
            <span className="ml-1 opacity-60">
              · {t(`flowNode.types.${type}`, { defaultValue: type })}
            </span>
          )}
          {step.reason && <span className="ml-1 opacity-60">→ {step.reason}</span>}
        </span>
        {running ? (
          <span className="shrink-0 animate-pulse text-[10px]">{t('steps.running')}</span>
        ) : (
          <span className="shrink-0 font-mono text-[10px] opacity-70">
            {failed ? t('steps.error') : formatDurationMs(step.durationMs ?? 0)}
          </span>
        )}
        {hasBody && <span className="shrink-0 opacity-50">{open ? '▾' : '▸'}</span>}
      </button>
      {open && hasBody && (
        <div className="whitespace-pre-wrap break-words px-3 pb-2 text-[11px] leading-relaxed text-[var(--color-text-dim)]">
          {step.output}
        </div>
      )}
    </div>
  )
}

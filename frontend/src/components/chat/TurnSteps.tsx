import type { TurnStep } from '../../types'
import { ThinkingBlock } from './ThinkingBlock'
import { ActivityCard } from './ActivityCard'
import { TextStep } from './TextStep'

interface Props {
  steps: TurnStep[]
  onOpenFile?: (path: string) => void
}

// TurnSteps renders an assistant turn's activity trace as compact, collapsible
// cards: thinking blocks, intermediate narration and tool activity cards — all
// single-line by default, expandable on click.
export function TurnSteps({ steps, onOpenFile }: Props) {
  if (!steps.length) return null
  return (
    <div className="mb-2 flex flex-col gap-0.5">
      {steps.map((step, i) => {
        if (step.kind === 'thinking') return <ThinkingBlock key={i} text={step.text || ''} />
        if (step.kind === 'tool') return <ActivityCard key={i} step={step} onOpenFile={onOpenFile} />
        // Intermediate text narration — collapsed to a one-line card.
        if (step.text?.trim()) {
          return <TextStep key={i} text={step.text} onOpenFile={onOpenFile} />
        }
        return null
      })}
    </div>
  )
}

// parseSteps safely decodes the JSON `steps` string persisted on a message.
export function parseSteps(json?: string): TurnStep[] {
  if (!json || json === '[]') return []
  try {
    const v = JSON.parse(json)
    return Array.isArray(v) ? (v as TurnStep[]) : []
  } catch {
    return []
  }
}

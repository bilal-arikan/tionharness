import type { TurnStep } from '../../types'
import { ThinkingBlock } from './ThinkingBlock'
import { ActivityCard } from './ActivityCard'
import { TextStep } from './TextStep'
import { TodoCard } from './TodoCard'
import { RecoveryStep } from './RecoveryStep'
import { ArtifactCard } from './ArtifactCard'

interface Props {
  steps: TurnStep[]
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
}

// Tool names the chat renders as a special artifact card instead of a generic
// activity row.
const ARTIFACT_TOOLS = new Set(['create_artifact', 'update_artifact'])

// TurnSteps renders an assistant turn's activity trace as compact, collapsible
// cards: thinking blocks, intermediate narration and tool activity cards — all
// single-line by default, expandable on click.
export function TurnSteps({ steps, onOpenFile, onOpenArtifact }: Props) {
  if (!steps.length) return null
  return (
    <div className="mb-2 flex flex-col gap-0.5">
      {steps.map((step, i) => {
        if (step.kind === 'thinking') return <ThinkingBlock key={i} text={step.text || ''} />
        if (step.kind === 'todo') return <TodoCard key={i} step={step} />
        if (step.kind === 'recovery') return <RecoveryStep key={i} step={step} />
        if (step.kind === 'tool') {
          // Back-compat: traces persisted before 'todo' was a first-class kind
          // carry the checklist as a todo_write tool step.
          if (step.tool === 'todo_write') return <TodoCard key={i} step={step} />
          // Artifact create/update → clickable card linking to the viewer.
          if (step.tool && ARTIFACT_TOOLS.has(step.tool)) {
            return <ArtifactCard key={i} step={step} onOpenArtifact={onOpenArtifact} />
          }
          return <ActivityCard key={i} step={step} onOpenFile={onOpenFile} />
        }
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

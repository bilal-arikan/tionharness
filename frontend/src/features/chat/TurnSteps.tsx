import type { ReactNode } from 'react'
import { Zap } from 'lucide-react'
import type { TurnStep } from '@/types'
import { ThinkingBlock } from './ThinkingBlock'
import { ActivityCard } from './ActivityCard'
import { TextStep } from './TextStep'
import { TodoCard } from './TodoCard'
import { RecoveryStep } from './RecoveryStep'
import { ErrorStep } from './ErrorStep'
import { SteerStep } from './SteerStep'
import { ToolDeltaStep } from './ToolDeltaStep'
import { ArtifactCard } from './ArtifactCard'
import { DiffCard } from './DiffCard'
import { HookStep } from './HookStep'
import { SubagentStep } from './SubagentStep'
import { ContextChangeCard } from './ContextChangeCard'
import { isEditToolBase, synthDiffData } from '@/shared/lib/diff'
import { toolBase } from './tools'

interface Props {
  steps: TurnStep[]
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
}

// Tool names the chat renders as a special artifact card instead of a generic
// activity row.
const ARTIFACT_TOOLS = new Set(['create_artifact', 'update_artifact'])

// renderStep maps one trace step onto its card component (null = not rendered).
function renderStep(
  step: TurnStep,
  i: number,
  onOpenFile?: (path: string) => void,
  onOpenArtifact?: (id: string) => void,
) {
  if (step.kind === 'thinking') return <ThinkingBlock key={i} text={step.text || ''} />
  if (step.kind === 'todo') return <TodoCard key={i} step={step} />
  if (step.kind === 'diff') return <DiffCard key={i} step={step} onOpenFile={onOpenFile} />
  if (step.kind === 'subagent')
    return (
      <SubagentStep
        key={i}
        step={step}
        onOpenFile={onOpenFile}
        onOpenArtifact={onOpenArtifact}
      />
    )
  if (step.kind === 'recovery') return <RecoveryStep key={i} step={step} />
  if (step.kind === 'error') return <ErrorStep key={i} step={step} />
  if (step.kind === 'steer') return <SteerStep key={i} step={step} />
  if (step.kind === 'hook') return <HookStep key={i} step={step} />
  if (step.kind === 'context_change') return <ContextChangeCard key={i} step={step} />
  if (step.kind === 'tool_delta') return <ToolDeltaStep key={i} step={step} />
  // 'tombstone' is a control signal handled before render (App.onStep); skip.
  if (step.kind === 'tombstone') return null
  if (step.kind === 'tool') {
    // Back-compat: traces persisted before 'todo' was a first-class kind
    // carry the checklist as a todo_write tool step.
    if (step.tool === 'todo_write') return <TodoCard key={i} step={step} />
    // Artifact create/update → clickable card linking to the viewer.
    if (step.tool && ARTIFACT_TOOLS.has(step.tool)) {
      return <ArtifactCard key={i} step={step} onOpenArtifact={onOpenArtifact} />
    }
    // A successful file mutation (claude-cli applies Edit/Write itself, so it
    // arrives as a generic `tool` step) → render the same prominent diff
    // panel the native tool-loop emits as a `diff` step. Errored edits stay
    // as an ActivityCard so the failure output (and the un-applied, dimmed
    // diff) is visible.
    if (
      !step.isError &&
      isEditToolBase(toolBase(step.tool || '')) &&
      synthDiffData(toolBase(step.tool || ''), step.input)
    ) {
      return <DiffCard key={i} step={step} onOpenFile={onOpenFile} />
    }
    return <ActivityCard key={i} step={step} onOpenFile={onOpenFile} />
  }
  // Intermediate text narration — collapsed to a one-line card.
  if (step.text?.trim()) {
    return <TextStep key={i} text={step.text} onOpenFile={onOpenFile} />
  }
  return null
}

// TurnSteps renders an assistant turn's activity trace as compact, collapsible
// cards: thinking blocks, intermediate narration and tool activity cards — all
// single-line by default, expandable on click. Consecutive steps sharing a
// `batch` id (one provider response that carried multiple PARALLEL tool calls)
// are clustered under a small "⚡ N paralel araç çağrısı" header so batched
// execution is visible at a glance.
export function TurnSteps({ steps, onOpenFile, onOpenArtifact }: Props) {
  if (!steps.length) return null
  const out: ReactNode[] = []
  for (let i = 0; i < steps.length; ) {
    const b = steps[i].batch ?? 0
    if (b > 0) {
      // Collect the whole consecutive run of this batch group.
      let j = i
      while (j < steps.length && (steps[j].batch ?? 0) === b) j++
      const group = steps.slice(i, j)
      const rendered = group
        .map((s, k) => renderStep(s, i + k, onOpenFile, onOpenArtifact))
        .filter(Boolean)
      if (rendered.length > 1) {
        out.push(
          <div
            key={`batch-${b}-${i}`}
            className="my-0.5 rounded-lg border border-[color-mix(in_srgb,var(--color-accent)_25%,transparent)] bg-[color-mix(in_srgb,var(--color-accent)_4%,transparent)] px-1.5 pb-0.5 pt-1"
          >
            <div className="mb-0.5 flex items-center gap-1 text-[10px] font-medium text-[var(--color-text-dim)]">
              <Zap size={10} className="text-[var(--color-accent)]" />
              {rendered.length} paralel araç çağrısı (tek istekte)
            </div>
            {rendered}
          </div>,
        )
      } else {
        // A group whose members mostly render to nothing degrades to plain rows.
        out.push(...rendered)
      }
      i = j
      continue
    }
    const node = renderStep(steps[i], i, onOpenFile, onOpenArtifact)
    if (node) out.push(node)
    i++
  }
  return <div className="mb-2 flex flex-col">{out}</div>
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

import { memo, type ReactNode } from 'react'
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
import { CacheBreakCard } from './CacheBreakCard'
import { isEditToolBase, synthDiffData } from '@/shared/lib/diff'
import { toolBase } from './tools'

interface Props {
  steps: TurnStep[]
  // The open session; only the cache-break card uses it (its "refresh context"
  // remedy targets the session). Absent renders degrade to information only.
  sessionId?: string
  onOpenFile?: (path: string) => void
  onOpenArtifact?: (id: string) => void
}

// Tool names the chat renders as a special artifact card instead of a generic
// activity row.
const ARTIFACT_TOOLS = new Set(['create_artifact', 'update_artifact'])

// stableKey returns a React key stable across array mutations. When the step
// carries a server-assigned `id` that id survives tombstone removal and
// reordering; otherwise fall back to a kind+index composite that at least keeps
// parallel-batch groups distinct. Index-based keys leak component state (open/
// closed) when a tombstone shifts the array — every step after the removed one
// would reuse the wrong instance.
function stableKey(step: TurnStep, i: number): string {
  return step.id ?? `${step.kind}-${i}`
}

// renderStep maps one trace step onto its card component (null = not rendered).
// `key` is a React key produced by stableKey, stable across step mutations.
function renderStep(
  step: TurnStep,
  key: string,
  sessionId?: string,
  onOpenFile?: (path: string) => void,
  onOpenArtifact?: (id: string) => void,
) {
  if (step.kind === 'thinking') return <ThinkingBlock key={key} text={step.text || ''} />
  if (step.kind === 'todo') return <TodoCard key={key} step={step} />
  if (step.kind === 'diff') return <DiffCard key={key} step={step} onOpenFile={onOpenFile} />
  if (step.kind === 'subagent')
    return (
      <SubagentStep key={key} step={step} onOpenFile={onOpenFile} onOpenArtifact={onOpenArtifact} />
    )
  if (step.kind === 'recovery') return <RecoveryStep key={key} step={step} />
  if (step.kind === 'error') return <ErrorStep key={key} step={step} />
  if (step.kind === 'steer') return <SteerStep key={key} step={step} />
  if (step.kind === 'hook') return <HookStep key={key} step={step} />
  if (step.kind === 'context_change') return <ContextChangeCard key={key} step={step} />
  if (step.kind === 'cache_break')
    return <CacheBreakCard key={key} step={step} sessionId={sessionId} />
  if (step.kind === 'tool_delta') return <ToolDeltaStep key={key} step={step} />
  // 'tombstone' is a control signal handled before render (App.onStep); skip.
  if (step.kind === 'tombstone') return null
  if (step.kind === 'tool') {
    // Back-compat: traces persisted before 'todo' was a first-class kind
    // carry the checklist as a todo_write tool step.
    if (step.tool === 'todo_write') return <TodoCard key={key} step={step} />
    // Artifact create/update → clickable card linking to the viewer.
    if (step.tool && ARTIFACT_TOOLS.has(step.tool)) {
      return <ArtifactCard key={key} step={step} onOpenArtifact={onOpenArtifact} />
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
      return <DiffCard key={key} step={step} onOpenFile={onOpenFile} />
    }
    return <ActivityCard key={key} step={step} onOpenFile={onOpenFile} />
  }
  // Intermediate text narration — collapsed to a one-line card.
  if (step.text?.trim()) {
    return <TextStep key={key} text={step.text} onOpenFile={onOpenFile} />
  }
  return null
}

// TurnSteps renders an assistant turn's activity trace as compact, collapsible
// cards: thinking blocks, intermediate narration and tool activity cards — all
// single-line by default, expandable on click. Consecutive steps sharing a
// `batch` id (one provider response that carried multiple PARALLEL tool calls)
// are clustered under a small "⚡ N paralel araç çağrısı" header so batched
// execution is visible at a glance.
// memo: a long transcript re-renders on every streaming delta, and re-running
// this over every finished turn's trace (each one a tree of markdown/diff cards)
// is what made scrolling a worker session stutter. The finished turns' `steps`
// arrays are referentially stable (AssistantTurn memoizes the parse), so only
// the live turn actually re-renders.
export const TurnSteps = memo(function TurnSteps({
  steps,
  sessionId,
  onOpenFile,
  onOpenArtifact,
}: Props) {
  if (!steps.length) return null
  const out: ReactNode[] = []
  for (let i = 0; i < steps.length;) {
    const b = steps[i].batch ?? 0
    if (b > 0) {
      // Collect the whole consecutive run of this batch group.
      let j = i
      while (j < steps.length && (steps[j].batch ?? 0) === b) j++
      const group = steps.slice(i, j)
      const rendered = group
        .map((s, k) => renderStep(s, stableKey(s, i + k), sessionId, onOpenFile, onOpenArtifact))
        .filter(Boolean)
      if (rendered.length > 1) {
        // Keyed by the batch id + first step's stable key so the group wrapper
        // survives a tombstone that shifts the group's position.
        const groupKey = `batch-${b}-${stableKey(group[0], i)}`
        out.push(
          <div
            key={groupKey}
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
    const node = renderStep(steps[i], stableKey(steps[i], i), sessionId, onOpenFile, onOpenArtifact)
    if (node) out.push(node)
    i++
  }
  return <div className="mb-2 flex flex-col">{out}</div>
})

// stepTruncated reports whether the server cut any of this step's payloads (or
// a nested subagent step's) on the transcript read path — i.e. showing it in
// full needs a refetch via sessionApi.getMessageSteps.
export function stepTruncated(s: TurnStep): boolean {
  return (
    !!s.outputTruncated ||
    !!s.textTruncated ||
    !!s.patchTruncated ||
    !!s.inputTruncated ||
    !!s.subSteps?.some(stepTruncated)
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

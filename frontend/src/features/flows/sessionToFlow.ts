// Reify a chat session's transcript into a flow — the session→flow half of the
// flow↔session bridge. A linear conversation becomes a vertical chain of agent
// nodes (each assistant turn is a node whose prompt is the preceding user text).
//
// sessionToFlowRun additionally synthesizes a COMPLETED run: a FlowState whose
// trace carries each node's actual output (the assistant reply), so the session
// can be viewed as an already-finished flow run (RunView shows prompts on the
// canvas AND replies in the step trace) without saving anything. Branch/parallel
// structure can't be inferred from a flat transcript, so the result is linear.
import type { Flow, FlowGraph, FlowNode, FlowRun, FlowState, FlowTraceEntry, Message, TurnStep } from '@/types'

function truncate(s: string, n: number): string {
  const t = s.trim().replace(/\s+/g, ' ')
  return t.length > n ? t.slice(0, n - 1) + '…' : t
}

// A segment that becomes one flow node: a title (from a step's bold header, if
// any) and the output text (the agent's reply for that step).
interface Segment {
  title: string
  output: string
}

// splitTitle parses a flow-transcript step's "**title**\n\n<output>" shape
// (produced by the backend's flowStateToSteps) into a title + output. Falls back
// to an empty title with the whole text as output.
function splitTitle(text: string): Segment {
  const m = text.match(/^\s*\*\*(.+?)\*\*\s*\n+([\s\S]*)$/)
  if (m) return { title: truncate(m[1], 40), output: m[2].trim() }
  return { title: '', output: text.trim() }
}

// expandAssistant turns one assistant message into one-or-more node segments. A
// flow-run session records the whole run as a SINGLE assistant turn whose `steps`
// are one text step per node; when the steps are all text (≥2), each becomes its
// own node so the reified flow mirrors the original graph. Otherwise (a normal
// chat reply, possibly with thinking/tool steps) the message stays a single node.
function expandAssistant(m: Message): Segment[] {
  let steps: TurnStep[] = []
  if (m.steps) {
    try {
      const a = JSON.parse(m.steps)
      if (Array.isArray(a)) steps = a
    } catch {
      // malformed steps → fall through to the single-node path
    }
  }
  const allText = steps.length >= 2 && steps.every((s) => s.kind === 'text' && !!(s.text ?? '').trim())
  if (allText) return steps.map((s) => splitTitle(s.text ?? ''))
  return [{ title: '', output: m.text ?? '' }]
}

// SessionFlow bundles the derived graph plus a synthetic Flow + completed FlowRun
// so the transcript can be rendered by the read-only RunView (with node outputs)
// and, optionally, saved as a real editable flow (via the graph).
export interface SessionFlow {
  graph: FlowGraph
  flow: Flow
  run: FlowRun
}

// sessionToFlowRun builds the bundle from a session's messages. Only
// user/assistant turns form the skeleton (system/tool turns are skipped);
// consecutive user turns coalesce into the next assistant node's prompt, and each
// assistant turn becomes its own node whose trace output is that reply. A trailing
// user turn with no reply becomes a final node with an empty output.
export function sessionToFlowRun(
  messages: Message[],
  fallbackAgentId: string,
  sessionId: string,
  sessionTitle: string,
): SessionFlow {
  const turns = messages.filter((m) => m.role === 'user' || m.role === 'assistant')
  const nodes: FlowNode[] = []
  const trace: FlowTraceEntry[] = []
  const outputs: Record<string, string> = {}
  let pendingUser = ''
  let idx = 0
  let prevId = ''
  let firstInput = ''
  let lastOutput = ''

  const link = (id: string) => {
    if (prevId) {
      const prev = nodes.find((n) => n.id === prevId)
      if (prev) prev.next = id
    }
    prevId = id
  }

  for (const m of turns) {
    if (m.role === 'user') {
      const text = m.text.trim()
      if (text) {
        pendingUser = pendingUser ? `${pendingUser}\n\n${text}` : text
        if (!firstInput) firstInput = text
      }
      continue
    }
    // assistant → one OR MORE nodes: a flow-run turn expands its per-node steps,
    // a normal reply stays a single node. The first node's prompt is the user's
    // question; later nodes in the same turn feed off {{last}}.
    const segments = expandAssistant(m)
    segments.forEach((seg, k) => {
      const id = `n${++idx}`
      const title = seg.title || truncate(pendingUser || seg.output || `adım ${idx}`, 40)
      nodes.push({
        id,
        type: 'agent',
        title,
        agentId: m.agentId || fallbackAgentId,
        prompt: k === 0 ? pendingUser || '{{last}}' : '{{last}}',
        next: '',
      })
      link(id)
      trace.push({ nodeId: id, type: 'agent', title, output: seg.output, at: 0 })
      outputs[id] = seg.output
      lastOutput = seg.output
    })
    pendingUser = ''
  }

  // A trailing user turn awaiting a reply becomes a final (empty-output) node.
  if (pendingUser) {
    const id = `n${++idx}`
    const title = truncate(pendingUser, 40)
    nodes.push({ id, type: 'agent', title, agentId: fallbackAgentId, prompt: pendingUser, next: '' })
    link(id)
    trace.push({ nodeId: id, type: 'agent', title, output: '', at: 0 })
    outputs[id] = ''
  }

  const graph: FlowGraph = { start: nodes[0]?.id ?? '', nodes, accumulate: true }
  const state: FlowState = { current: '', last: lastOutput, outputs, steps: nodes.length, trace }
  const now = Math.floor(Date.now() / 1000)
  const id = `session:${sessionId}`
  const flow: Flow = {
    id,
    name: `${sessionTitle || 'Oturum'} akışı`,
    graph: JSON.stringify(graph),
    createdAt: now,
    updatedAt: now,
  }
  const run: FlowRun = {
    id,
    flowId: id,
    status: 'success',
    input: firstInput,
    state: JSON.stringify(state),
    output: lastOutput,
    error: '',
    createdAt: now,
    updatedAt: now,
  }
  return { graph, flow, run }
}

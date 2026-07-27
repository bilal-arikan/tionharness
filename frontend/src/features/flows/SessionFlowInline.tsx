import { useMemo, useState } from 'react'
import { ArrowLeft, Save, Loader2 } from 'lucide-react'
import { api } from '@/api'
import type { Agent, Message } from '@/types'
import { Button } from '@/shared/components'
import { RunView } from './RunView'
import { sessionToFlowRun } from './sessionToFlow'

interface Props {
  messages: Message[]
  agents: Agent[]
  fallbackAgentId: string
  sessionId: string
  sessionTitle: string
  onBack: () => void
  onError: (msg: string) => void
}

// SessionFlowInline shows the current session as an already-COMPLETED flow run,
// inline in the chat area (not a modal): the transcript reified into a linear
// agent-node chain rendered by the read-only RunView, so node prompts show on the
// canvas AND agent replies show in the step trace — no saving required. "← Sohbete
// dön" returns to the transcript; "Flow olarak kaydet" persists the graph as a
// real editable flow (add branches/parallel/loop, then re-run).
export function SessionFlowInline({ messages, agents, fallbackAgentId, sessionId, sessionTitle, onBack, onError }: Props) {
  const { graph, flow, run } = useMemo(
    () => sessionToFlowRun(messages, fallbackAgentId, sessionId, sessionTitle),
    [messages, fallbackAgentId, sessionId, sessionTitle],
  )
  const [saving, setSaving] = useState(false)
  // The graph always carries a start node; "empty" means no real (non-start) nodes.
  const realNodes = graph.nodes.filter((n) => n.type !== 'start')
  const empty = realNodes.length === 0

  const save = async () => {
    if (empty) return
    setSaving(true)
    try {
      await api.createFlow(flow.name, graph)
      onError('') // clear
    } catch (e) {
      onError((e as Error).message)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex min-h-0 min-w-0 flex-1 flex-col">
      <div className="flex items-center justify-between gap-2 border-b border-[var(--color-border)] px-4 py-2">
        <button
          onClick={onBack}
          className="flex items-center gap-1.5 rounded-lg border border-[var(--color-border)] px-2.5 py-1 text-xs text-[var(--color-text-dim)] transition hover:text-[var(--color-accent)]"
          title="Sohbet transkriptine dön"
        >
          <ArrowLeft size={15} className="shrink-0" />
          <span>Sohbete dön</span>
        </button>
        <span className="truncate text-xs text-[var(--color-text-dim)]">
          Anlık akış görünümü ({realNodes.length} adım) — kaydedilmedi
        </span>
        <Button onClick={save} disabled={saving || empty} variant="primary">
          {saving ? <Loader2 size={14} className="animate-spin" /> : <Save size={14} />}
          <span className="ml-1">Flow olarak kaydet</span>
        </Button>
      </div>
      {empty ? (
        <div className="flex flex-1 items-center justify-center text-sm text-[var(--color-text-dim)]">
          Bu oturumda akışa dönüştürülecek mesaj yok.
        </div>
      ) : (
        <RunView run={run} flow={flow} agents={agents} hideSummary />
      )}
    </div>
  )
}

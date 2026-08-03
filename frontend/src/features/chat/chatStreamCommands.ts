// "/" slash commands for the chat composer: summary/compaction, context-reset
// handoff, flow runs and the command palette builder. Plain functions extracted
// from useChatStream; the hook's callbacks build the context and delegate here.
import type { Dispatch, SetStateAction } from 'react'
import { api } from '@/api'
import type { Attachment, Flow, Message, SlashCommand } from '@/types'
import { flowSlug } from './chatStreamHelpers'

export interface CommandContext {
  activeSessionId: string | null
  activeAgentId: string | null
  setMessages: Dispatch<SetStateAction<Message[]>>
  setError: (msg: string | null) => void
}

// Run a "/" command that posts an assistant message: summary (board/flows/
// tools) or conversation compaction.
export function performSummarize(ctx: CommandContext, kind: string): void {
  const { activeSessionId, activeAgentId, setMessages, setError } = ctx
  const sid = activeSessionId
  if (!sid) return
  const now = Math.floor(Date.now() / 1000)
  const userTmp = `cmd-u-${Date.now()}`
  const botTmp = `cmd-a-${Date.now()}`
  const busyLabel =
    kind === 'compact'
      ? '⏳ Sohbet sıkıştırılıyor…'
      : kind === 'refresh-context'
        ? "⏳ Bağlam snapshot'ı yenileniyor…"
        : '⏳ Özetleniyor…'
  const cmdBubble: Message = {
    id: userTmp,
    sessionId: sid,
    role: 'user',
    text: '/' + kind,
    createdAt: now,
  }
  const placeholder: Message = {
    id: botTmp,
    sessionId: sid,
    role: 'assistant',
    agentId: activeAgentId ?? undefined,
    text: busyLabel,
    steps: '[]',
    createdAt: now,
  }
  setMessages((prev) => [...prev, cmdBubble, placeholder])
  api
    .summarizeSession(sid, kind)
    .then(({ userMessage, replyMessage }) =>
      setMessages((prev) =>
        prev.map((m) => (m.id === userTmp ? userMessage : m.id === botTmp ? replyMessage : m)),
      ),
    )
    .catch((e) => {
      setMessages((prev) => prev.filter((m) => m.id !== userTmp && m.id !== botTmp))
      setError((e as Error).message)
    })
}

export interface HandoffContext extends CommandContext {
  refreshSessions: () => void
  selectSession: (id: string) => void
}

// Context reset (/handoff): write a handoff artifact for the current session and
// spawn a fresh one to continue in a clean window, then switch the UI to it. The
// old session keeps a tombstone linking forward; the new session opens with the
// handoff inline.
export function performHandoff(ctx: HandoffContext): void {
  const { activeSessionId, activeAgentId, setMessages, setError, refreshSessions, selectSession } =
    ctx
  const sid = activeSessionId
  if (!sid) return
  const now = Math.floor(Date.now() / 1000)
  const userTmp = `cmd-u-${Date.now()}`
  const botTmp = `cmd-a-${Date.now()}`
  const cmdBubble: Message = {
    id: userTmp,
    sessionId: sid,
    role: 'user',
    text: '/handoff',
    createdAt: now,
  }
  const placeholder: Message = {
    id: botTmp,
    sessionId: sid,
    role: 'assistant',
    agentId: activeAgentId ?? undefined,
    text: '⏳ Context reset — handoff yazılıyor ve temiz oturum başlatılıyor…',
    steps: '[]',
    createdAt: now,
  }
  setMessages((prev) => [...prev, cmdBubble, placeholder])
  api
    .handoffSession(sid)
    .then((res) => {
      // Blocked (coordinator with running workers): no fresh session was spawned.
      // Swap the optimistic placeholder for the persisted "/handoff" + notice pair
      // so the reason shows in-thread; do NOT switch sessions or raise an error.
      if (res.blocked || !res.newSessionId) {
        setMessages((prev) => {
          const kept = prev.filter((m) => m.id !== userTmp && m.id !== botTmp)
          const extra = [res.userMessage, res.replyMessage].filter(Boolean) as Message[]
          return [...kept, ...extra]
        })
        refreshSessions()
        return
      }
      // Refresh the list (old tombstone + new session) and jump to the fresh one.
      refreshSessions()
      selectSession(res.newSessionId)
    })
    .catch((e) => {
      setMessages((prev) => prev.filter((m) => m.id !== userTmp && m.id !== botTmp))
      setError((e as Error).message)
    })
}

// Run a flow from the chat composer, streaming node-by-node progress over SSE:
// an optimistic user bubble + a live assistant bubble whose transcript grows as
// each node finishes, replaced by the persisted reply when the run completes.
export function performRunFlow(
  ctx: CommandContext,
  flowId: string,
  flowName: string,
  input: string,
  attachments: Attachment[],
): void {
  const { activeSessionId, activeAgentId, setMessages, setError } = ctx
  const sid = activeSessionId
  if (!sid) return
  const now = Math.floor(Date.now() / 1000)
  const userTmp = `flow-u-${Date.now()}`
  const botTmp = `flow-a-${Date.now()}`
  const userBubble: Message = {
    id: userTmp,
    sessionId: sid,
    role: 'user',
    text: input.trim() || `🔀 ${flowName}`,
    attachments: attachments.length ? attachments : undefined,
    createdAt: now,
  }
  const placeholder: Message = {
    id: botTmp,
    sessionId: sid,
    role: 'assistant',
    agentId: activeAgentId ?? undefined,
    text: `🔀 **${flowName}**\n\n_⏳ başlatılıyor…_`,
    steps: '[]',
    createdAt: now,
  }
  setMessages((prev) => [...prev, userBubble, placeholder])

  // Live transcript: each node shows a spinner until its output arrives,
  // re-assembled on every event into the bubble's markdown. Keyed by nodeId
  // (parallel children share an execution index, so index can't be the key);
  // ordered by arrival so parallel nodes list in a stable order.
  const nodes = new Map<string, { title: string; output?: string; error?: string }>()
  const render = () => {
    let s = `🔀 **${flowName}**\n\n`
    let i = 0
    for (const n of nodes.values()) {
      i++
      const body = n.error !== undefined ? `⚠️ ${n.error}` : (n.output ?? '_⏳ çalışıyor…_')
      s += `#### ${i}. ${n.title}\n\n${body}\n\n`
    }
    return s.trim()
  }
  const setBotText = (text: string) =>
    setMessages((prev) => prev.map((m) => (m.id === botTmp ? { ...m, text } : m)))

  api
    .runFlowStream(sid, flowId, input, {
      attachments,
      onMeta: ({ userMessage }) =>
        setMessages((prev) => prev.map((m) => (m.id === userTmp ? userMessage : m))),
      onNode: (ev) => {
        const cur = nodes.get(ev.nodeId) ?? { title: ev.title }
        cur.title = ev.title
        if (ev.phase === 'done') cur.output = ev.output ?? ''
        else if (ev.phase === 'error') cur.error = ev.error ?? 'hata'
        nodes.set(ev.nodeId, cur)
        setBotText(render())
      },
      onReply: ({ replyMessage }) =>
        setMessages((prev) => prev.map((m) => (m.id === botTmp ? replyMessage : m))),
      onError: (err) => {
        setMessages((prev) => prev.filter((m) => m.id !== userTmp && m.id !== botTmp))
        setError(err)
      },
    })
    .catch((e) => {
      setMessages((prev) => prev.filter((m) => m.id !== userTmp && m.id !== botTmp))
      setError((e as Error).message)
    })
}

export interface ChatCommandDeps {
  flows: Flow[]
  summarize: (kind: string) => void
  handoff: () => void
  openRewind: () => void
  runFlow: (flowId: string, flowName: string, input: string, attachments?: Attachment[]) => void
}

// Slash commands available in the chat composer ("/" menu): built-in session
// commands plus one entry per flow (🔀, takes the rest of the line as input).
export function buildChatCommands({
  flows,
  summarize,
  handoff,
  openRewind,
  runFlow,
}: ChatCommandDeps): SlashCommand[] {
  return [
    {
      name: 'compact',
      icon: '🗜',
      description: 'Sohbeti şimdi özete sıkıştır',
      run: () => summarize('compact'),
    },
    {
      name: 'refresh-context',
      icon: '🔄',
      description:
        "Donmuş bağlam snapshot'ını yenile — araç/skill/talimat değişiklikleri sonraki turda görünür",
      run: () => summarize('refresh-context'),
    },
    {
      name: 'handoff',
      icon: '↪',
      description: 'Context reset — temiz pencerede devam et',
      run: () => handoff(),
    },
    {
      name: 'rewind',
      icon: '⟲',
      description: "Sohbeti bir checkpoint'e geri sar — mesajları geri al",
      run: () => openRewind(),
    },
    {
      name: 'tools',
      icon: '🔌',
      description: 'Kullanılabilir araçları listele',
      run: () => summarize('tools'),
    },
    {
      name: 'board',
      icon: '🗂',
      description: 'Görev panosunu özetle',
      run: () => summarize('board'),
    },
    { name: 'flows', icon: '🔀', description: 'Akışları özetle', run: () => summarize('flows') },
    ...flows.map((f): SlashCommand => ({
      name: flowSlug(f.name) || f.id.slice(0, 6),
      icon: '🔀',
      description: `${f.name} akışını çalıştır`,
      takesInput: true,
      run: (input?: string, attachments?: Attachment[]) =>
        runFlow(f.id, f.name, input ?? '', attachments ?? []),
    })),
  ]
}

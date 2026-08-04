// "/" slash commands for the chat composer: summary/compaction, context-reset
// handoff and the command palette builder. Plain functions extracted from
// useChatStream; the hook's callbacks build the context and delegate here.
import type { Dispatch, SetStateAction } from 'react'
import { api } from '@/api'
import type { Message, SlashCommand } from '@/types'

export interface CommandContext {
  activeSessionId: string | null
  activeAgentId: string | null
  setMessages: Dispatch<SetStateAction<Message[]>>
  setError: (msg: string | null) => void
}

// Run a "/" command that posts an assistant message: summary (board/flows/
// tools) or conversation compaction. The command + its result are now
// event-sourced on the session hub: the backend persists the "/kind" message and
// publishes user_message/agent_start/step/reply BEFORE + DURING the (often slow)
// op, so the live "working" bubble and the final report are rendered by the hub
// handlers — and survive a page refresh mid-op (_Docs/58). Here we only add an
// optimistic user echo prefixed "tmp-" so the hub's user_message drops it and
// swaps in the persisted message; no assistant placeholder (the hub ghost bubble
// carries the busy label).
export function performSummarize(ctx: CommandContext, kind: string): void {
  const { activeSessionId, setMessages, setError } = ctx
  const sid = activeSessionId
  if (!sid) return
  const now = Math.floor(Date.now() / 1000)
  const userTmp = `tmp-cmd-u-${Date.now()}`
  const cmdBubble: Message = {
    id: userTmp,
    sessionId: sid,
    role: 'user',
    text: '/' + kind,
    createdAt: now,
  }
  setMessages((prev) => [...prev, cmdBubble])
  api.summarizeSession(sid, kind).catch((e) => {
    // A network/pre-flight failure before the hub rendered anything: pull the
    // optimistic echo back and surface the error. (A server-side failure after the
    // user message persisted leaves it in the transcript via the hub — the toast
    // still explains what went wrong; turn_error clears the "working" bubble.)
    setMessages((prev) => prev.filter((m) => m.id !== userTmp))
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
// handoff inline. Like /compact, the command + its result are event-sourced on the
// OLD session's hub (backend persists "/handoff" and publishes user_message/
// agent_start/step/reply), so the live "working" bubble survives a refresh mid-op.
// Here we only keep a "tmp-" optimistic user echo (the hub swaps in the persisted
// message); no assistant placeholder (the hub ghost carries the busy label).
export function performHandoff(ctx: HandoffContext): void {
  const { activeSessionId, setMessages, setError, refreshSessions, selectSession } = ctx
  const sid = activeSessionId
  if (!sid) return
  const now = Math.floor(Date.now() / 1000)
  const userTmp = `tmp-cmd-u-${Date.now()}`
  const cmdBubble: Message = {
    id: userTmp,
    sessionId: sid,
    role: 'user',
    text: '/handoff',
    createdAt: now,
  }
  setMessages((prev) => [...prev, cmdBubble])
  api
    .handoffSession(sid)
    .then((res) => {
      // Blocked (coordinator with running workers): no fresh session was spawned and
      // the session stays put. The hub already rendered the "/handoff" bubble + the
      // ⚠️ notice on this (still active) session, so just refresh the list — do NOT
      // switch sessions or raise an error.
      if (res.blocked || !res.newSessionId) {
        refreshSessions()
        return
      }
      // Refresh the list (old tombstone + new session) and jump to the fresh one.
      refreshSessions()
      selectSession(res.newSessionId)
    })
    .catch((e) => {
      setMessages((prev) => prev.filter((m) => m.id !== userTmp))
      setError((e as Error).message)
    })
}

export interface ChatCommandDeps {
  summarize: (kind: string) => void
  handoff: () => void
  openRewind: () => void
}

// Slash commands available in the chat composer ("/" menu): built-in session
// commands only. Running a flow from the composer was removed — flows run from the
// Flows panel (per-flow entries no longer pollute the "/" menu); "/flows" still
// SUMMARIZES the flow list.
export function buildChatCommands({
  summarize,
  handoff,
  openRewind,
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
  ]
}

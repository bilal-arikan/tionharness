// Hub-driven rendering: the authoritative render path for the ACTIVE session.
// Every window (the one that sent the turn AND every other window viewing the
// session) subscribes to the per-session hub stream and renders the transcript
// from it — there is no "owner window" anymore (full cutover, _Docs/58). The
// send path (performSend) only RUNS the turn + keeps the runId for stop/steer;
// it no longer paints steps/reply.
import type { Dispatch, RefObject, SetStateAction } from 'react'
import type { Message, TurnStep } from '@/types'
import type { HubEvent, SessionStreamHandlers } from '@/api/sessionStream'
import { HubKind, windowClientId } from '@/api/sessionStream'
import { emitToast } from '@/shared/lib/notifyBus'
import type { PendingAsk } from './AskPrompt'
import type { PendingItem } from './PendingTray'
import { withAdded, withRemoved, withoutKey } from './chatStreamHelpers'

// Interaction ids already announced (sound + OS toast), so the SAME pending
// prompt is not re-announced when the hub replays it — a reconnect, a session
// switch back, or a second window all redeliver the retained interaction_open.
// Entries are dropped on interaction_resolved, so the set stays bounded and a
// genuinely NEW prompt (new id) always cues.
const cuedInteractions = new Set<string>()

// askCueText renders the notification title/body for a blocking prompt. The
// title names WHAT is blocked (question vs. tool approval vs. plan); the body
// carries the concrete detail the user needs to decide.
function askCueText(ask: PendingAsk): { title: string; body: string } {
  const first = ask.questions?.[0]?.question ?? ask.question
  switch (ask.kind) {
    case 'permission':
      return {
        title: 'İzin bekleniyor',
        body: ask.tool ? `${ask.tool}: ${ask.cmd ?? ask.risk ?? ''}`.trim() : (ask.cmd ?? ''),
      }
    case 'plan':
      return { title: 'Plan onayı bekleniyor', body: first || 'Ajan hazırladığı planın onayını bekliyor.' }
    default:
      return { title: 'Ajan bir soru sordu', body: first || 'Ajan yanıtını bekliyor.' }
  }
}

export interface HubApplyCtx {
  sid: string
  activeSessionIdRef: RefObject<string | null>
  setMessages: Dispatch<SetStateAction<Message[]>>
  setStreamingSessions: Dispatch<SetStateAction<ReadonlySet<string>>>
  setPendingSessions: Dispatch<SetStateAction<ReadonlySet<string>>>
  setPendingAsks: Dispatch<SetStateAction<Record<string, PendingAsk>>>
  // setQueued renders this session's WAITING backend queue (queue_update) in the
  // composer tray; setPresence surfaces "open in N windows" (Faz 4); setTyping
  // surfaces "another window is typing…".
  setQueued: (items: PendingItem[]) => void
  setPresence: (count: number) => void
  setTyping: (active: boolean) => void
  // reload pulls the authoritative transcript (listMessages) — used on a reset,
  // and on turn end to swap the live ghost for the persisted messages.
  reload: () => void
  // Master desktop-notification preference (settings.desktopNotifications), read
  // at fire time so a Settings change applies without resubscribing. Gates only
  // the OS toast for a blocking prompt; the sound cue has its own device-local
  // pref (soundEffectsEnabled).
  notifyEnabled: RefObject<boolean>
  // Bumps the shared "meter" nonce: re-fetches the Session Info panel (size,
  // message count, context usage, spend) and the session's artifact list. Called
  // when a message ENTERS the transcript — the user's turn starting and the
  // agent's turn ending — so an open panel tracks the conversation instead of
  // freezing at its mount-time snapshot.
  bumpMeter: () => void
}

// makeHubHandlers builds the SessionStreamHandlers for one active session. It
// keeps a single live "ghost" assistant bubble (grown from step + delta events)
// which the persisted reply replaces on turn end.
export function makeHubHandlers(ctx: HubApplyCtx): SessionStreamHandlers {
  const {
    sid,
    activeSessionIdRef,
    setMessages,
    setStreamingSessions,
    setPendingSessions,
    setPendingAsks,
    setQueued,
    setPresence,
    setTyping,
    reload,
    notifyEnabled,
    bumpMeter,
  } = ctx

  // Live accumulator behind the ghost bubble. A stable id per turn so upserts
  // land on the same bubble; reset when a new agent starts answering.
  const ghostId = `live-hub-${sid}`
  let steps: TurnStep[] = []
  let text = ''
  let agentId: string | undefined
  let hasGhost = false
  // Turn-start timestamp for the live bubble, stamped ONCE per turn (at AgentStart
  // or the first ghost sync) and kept stable across every step/delta upsert — so
  // the LiveTimer counts the agent's whole run, not the time since the last tool.
  // Reset on drop so the next turn re-stamps.
  let ghostStartedAt = 0

  // onSid mutates the transcript only while THIS session is still on screen (the
  // subscription outlives a fast session switch by a frame).
  const onSid = (updater: (prev: Message[]) => Message[]) =>
    setMessages((prev) => (activeSessionIdRef.current === sid ? updater(prev) : prev))

  const composeGhost = (): Message => ({
    id: ghostId,
    sessionId: sid,
    role: 'assistant',
    agentId,
    text,
    steps: JSON.stringify(steps),
    // Stable turn-start stamp (not "now"): the live timer must not reset per step.
    createdAt: ghostStartedAt || Math.floor(Date.now() / 1000),
  })

  // syncGhost UPSERTs the ghost into the transcript (append if wiped by a reload).
  const syncGhost = () => {
    hasGhost = true
    // Fallback stamp if steps arrive before AgentStart set the turn-start time.
    if (!ghostStartedAt) ghostStartedAt = Math.floor(Date.now() / 1000)
    const bubble = composeGhost()
    onSid((prev) =>
      prev.some((m) => m.id === ghostId)
        ? prev.map((m) => (m.id === ghostId ? bubble : m))
        : [...prev, bubble],
    )
  }

  const dropGhost = () => {
    if (!hasGhost) return
    hasGhost = false
    ghostStartedAt = 0 // next turn re-stamps its own start
    steps = []
    text = ''
    onSid((prev) => prev.filter((m) => m.id !== ghostId))
  }

  const applyStep = (st: TurnStep) => {
    if (st.kind === 'tombstone') {
      steps = steps.filter((s) => s.id !== st.ref)
    } else if (st.kind === 'thinking' && st.id) {
      const idx = steps.findIndex((s) => s.kind === 'thinking' && s.id === st.id)
      if (idx >= 0) {
        const merged = { ...steps[idx], text: (steps[idx].text || '') + (st.text || '') }
        steps = steps.map((s, k) => (k === idx ? merged : s))
      } else steps = [...steps, st]
    } else if (st.kind === 'tool_delta' && st.id) {
      const idx = steps.findIndex((s) => s.kind === 'tool_delta' && s.id === st.id)
      if (idx >= 0) {
        const merged = { ...steps[idx], output: (steps[idx].output || '') + (st.output || '') }
        steps = steps.map((s, k) => (k === idx ? merged : s))
      } else steps = [...steps, st]
    } else {
      steps = [...steps, st]
    }
    syncGhost()
  }

  const openInteraction = (ev: HubEvent) => {
    const p = (ev.payload ?? {}) as Record<string, unknown>
    const kind = (p.kind as PendingAsk['kind']) || 'ask'
    const ask: PendingAsk = {
      question: (p.question as string) || '',
      options: p.options as string[] | undefined,
      questions: p.questions as PendingAsk['questions'],
      kind,
      tool: p.tool as string | undefined,
      risk: p.reason as string | undefined,
      cmd: p.text as string | undefined,
      interactionId: p.id as string | undefined,
    }
    setPendingAsks((prev) => ({ ...prev, [sid]: ask }))
    // The turn is now BLOCKED on the user, so pull their attention: a distinct
    // chime (audible even with the window focused — they may be looking
    // elsewhere) plus, when this window is backgrounded, an OS toast. notify()
    // itself skips a visible window and un-granted permission. Announced once per
    // interaction id; a prompt with no id (legacy/local-only) always cues.
    const id = ask.interactionId
    if (id && cuedInteractions.has(id)) return
    if (id) cuedInteractions.add(id)
    // Route through the single funnel as the 'prompt' type: it plays the ask cue
    // (sound-effects pref) then, unless 'prompt' is muted in Settings, raises the
    // OS toast (master gate + backgrounded rule). Making it a real notify type is
    // what lets the user silence ask/permission toasts like any other.
    const { title, body } = askCueText(ask)
    emitToast({ type: 'prompt', enabled: notifyEnabled.current, title, body, tag: `ask:${sid}:${id ?? ''}` })
  }

  const resolveInteraction = (ev: HubEvent) => {
    const p = (ev.payload ?? {}) as Record<string, unknown>
    const id = p.id as string | undefined
    // Answered (here or in another window) — forget its cue so the set stays
    // bounded. A later prompt carries a fresh id and cues again.
    if (id) cuedInteractions.delete(id)
    // Close the card in EVERY window. Match the id so a stale resolve for an
    // already-replaced prompt doesn't drop a newer one.
    setPendingAsks((prev) => {
      const cur = prev[sid]
      if (!cur) return prev
      if (id && cur.interactionId && cur.interactionId !== id) return prev
      return withoutKey(prev, sid)
    })
  }

  return {
    onEvent: (ev: HubEvent) => {
      switch (ev.kind) {
        case HubKind.UserMessage: {
          const m = ev.payload as Message
          if (!m?.id) return
          // The serial worker just started this turn → mark the session busy.
          setStreamingSessions((p) => withAdded(p, sid))
          setPendingSessions((p) => withAdded(p, sid))
          // Our message just entered the transcript → refresh an open Session Info
          // panel (message count, size, context usage).
          bumpMeter()
          // Drop any local optimistic bubble (none in the queue path, but harmless)
          // and upsert the real user message in place.
          onSid((prev) => {
            const cleaned = prev.filter((x) => !(x.role === 'user' && x.id.startsWith('tmp-')))
            return cleaned.some((x) => x.id === m.id)
              ? cleaned.map((x) => (x.id === m.id ? m : x))
              : [...cleaned, m]
          })
          break
        }
        case HubKind.AgentStart: {
          const p = (ev.payload ?? {}) as Record<string, unknown>
          agentId = p.agentId as string | undefined
          steps = []
          text = ''
          ghostStartedAt = Math.floor(Date.now() / 1000) // definitive turn start
          // An AUTONOMOUS turn (coordinator/scheduler/spawn) publishes no
          // UserMessage, so mark the session busy here (and on the first Step) —
          // otherwise the "working" indicator + composer stop/steer cluster never
          // light up for a turn the user is merely watching.
          setStreamingSessions((pp) => withAdded(pp, sid))
          syncGhost()
          setPendingSessions((pp) => withRemoved(pp, sid))
          break
        }
        case HubKind.Step:
          // First live activity of an autonomous turn (no UserMessage preceded it):
          // mark busy so the session shows "working" like an interactive turn.
          setStreamingSessions((pp) => withAdded(pp, sid))
          applyStep(ev.payload as TurnStep)
          break
        case HubKind.Delta: {
          const st = ev.payload as TurnStep
          text += st.text || ''
          syncGhost()
          break
        }
        case HubKind.Reply: {
          const m = ev.payload as Message
          setPendingAsks((p) => withoutKey(p, sid))
          dropGhost()
          if (m?.id) {
            // Map in place when the reply is already in the transcript (a since=0
            // replay re-delivers completed turns); only append a genuinely new one.
            // Appending unconditionally would yank an old reply to the end and
            // scramble transcript order during replay.
            onSid((prev) => {
              const noGhost = prev.filter((x) => x.id !== ghostId)
              return noGhost.some((x) => x.id === m.id)
                ? noGhost.map((x) => (x.id === m.id ? m : x))
                : [...noGhost, m]
            })
          }
          break
        }
        case HubKind.InteractionOpen:
          openInteraction(ev)
          break
        case HubKind.InteractionResolved:
          resolveInteraction(ev)
          break
        case HubKind.QueueUpdate: {
          const p = (ev.payload ?? {}) as { queue?: { clientMsgId: string; text: string }[] }
          const items: PendingItem[] = (p.queue ?? []).map((q) => ({
            id: q.clientMsgId,
            text: q.text,
            kind: 'queue',
            sid,
          }))
          setQueued(items)
          break
        }
        case HubKind.Presence: {
          const p = (ev.payload ?? {}) as { count?: number }
          setPresence(p.count ?? 1)
          break
        }
        case HubKind.Typing: {
          const p = (ev.payload ?? {}) as { active?: boolean; clientId?: string }
          // Ignore our own echo — only OTHER windows' typing surfaces here.
          if (p.clientId && p.clientId === windowClientId) break
          setTyping(!!p.active)
          break
        }
        case HubKind.TurnDone:
        case HubKind.TurnError:
          setStreamingSessions((p) => withRemoved(p, sid))
          setPendingSessions((p) => withRemoved(p, sid))
          setPendingAsks((p) => withoutKey(p, sid))
          dropGhost()
          // Pull the authoritative transcript so the persisted turn (full trace,
          // usage, model) replaces the live ghost.
          reload()
          // The agent's reply is persisted now, so its cost/size/context figures
          // are final → refresh an open Session Info panel. Fired at turn END
          // (not per Reply) so a multi-agent turn still costs one refresh.
          bumpMeter()
          break
      }
    },
    onReset: () => {
      // Cursor unusable (server restart / ring eviction): resync from scratch.
      dropGhost()
      reload()
    },
  }
}

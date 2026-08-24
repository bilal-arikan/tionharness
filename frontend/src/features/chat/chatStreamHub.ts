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
import { noteServerTime, serverNow } from '@/shared/lib/serverClock'
import { emitToast } from '@/shared/lib/notifyBus'
import type { PendingAsk } from './AskPrompt'
import type { PendingItem } from './PendingTray'

import { withAdded, withRemoved, withoutKey } from './chatStreamHelpers'

// TurnEntry mirrors internal/turnqueue.Entry: one turn holding (or queued for) the
// session's admission slot. `kind` names the entry path, so the tray can say WHAT a
// message is waiting behind rather than just "meşgul".
interface TurnEntry {
  kind: string
  label?: string
  since: number
}

// turnKindLabel renders an admission-queue entry for the tray.
function turnKindLabel(e: TurnEntry): string {
  const byKind: Record<string, string> = {
    coordinator: 'Worker bildirimi işleniyor',
    worker: 'Worker görevi çalışıyor',
    wake: 'Zamanlanmış tur çalışıyor',
    peer: 'Ajan mesajı işleniyor',
    spawn: 'Spawn turu çalışıyor',
    automation: 'Otomasyon turu çalışıyor',
    command: 'Komut çalışıyor',
  }
  const base = byKind[e.kind] ?? 'Tur çalışıyor'
  return e.label && e.kind === 'command' ? `${e.label} çalışıyor` : base
}

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
      return {
        title: 'Plan onayı bekleniyor',
        body: first || 'Ajan hazırladığı planın onayını bekliyor.',
      }
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
  setQueued: Dispatch<SetStateAction<PendingItem[]>>
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
    createdAt: ghostStartedAt || serverNow(),
  })

  // ── Streaming coalescer ───────────────────────────────────────────────────
  // The server publishes one Delta per RAW provider chunk (no batching), and a
  // sync replaces the transcript with a new array reference. Downstream that
  // re-runs every memo keyed on `messages` — MessageList's participant pass
  // walks the WHOLE transcript — and re-parses the live bubble's entire
  // markdown body. Per token, on a growing string, that is the streaming-jank
  // hot spot. Collapsing syncs into ~20Hz frames keeps the text growth visually
  // smooth (well above the ~10Hz where reading starts to feel steppy) while
  // paying those costs once per frame instead of once per token.
  const GHOST_SYNC_MS = 50
  let ghostTimer: ReturnType<typeof setTimeout> | null = null
  let ghostDirty = false

  const upsertGhost = () => {
    const bubble = composeGhost()
    onSid((prev) =>
      prev.some((m) => m.id === ghostId)
        ? prev.map((m) => (m.id === ghostId ? bubble : m))
        : [...prev, bubble],
    )
  }

  // Cancelling is what keeps a trailing flush from RESURRECTING a ghost that was
  // just dropped (upsertGhost appends when the id is absent), so every drop path
  // must go through it.
  const cancelGhostSync = () => {
    if (ghostTimer) clearTimeout(ghostTimer)
    ghostTimer = null
    ghostDirty = false
  }

  // syncGhost UPSERTs the ghost into the transcript (append if wiped by a reload).
  // Leading-edge throttle: the first sync of a burst lands immediately — so the
  // bubble appears and the first token shows with no added latency — and the
  // rest collapse into the open window, with one trailing flush carrying the
  // accumulated text.
  const syncGhost = () => {
    hasGhost = true
    // Fallback stamp if steps arrive before AgentStart set the turn-start time.
    if (!ghostStartedAt) ghostStartedAt = serverNow()
    if (ghostTimer) {
      ghostDirty = true
      return
    }
    upsertGhost()
    ghostTimer = setTimeout(() => {
      ghostTimer = null
      if (!ghostDirty) return
      ghostDirty = false
      syncGhost() // trailing edge: flush what accumulated, reopen the window
    }, GHOST_SYNC_MS)
  }

  const dropGhost = () => {
    cancelGhostSync()
    if (!hasGhost) return
    hasGhost = false
    ghostStartedAt = 0 // next turn re-stamps its own start
    steps = []
    text = ''
    onSid((prev) => prev.filter((m) => m.id !== ghostId))
  }

  const applyStep = (st: TurnStep) => {
    // Legacy: only for previously persisted sessions. New live frames use the
    // generic ID + append/replace contract below.
    if (st.kind === 'tombstone') {
      steps = steps.filter((s) => s.id !== st.ref)
    } else if (st.kind === 'tool_delta' && st.id) {
      const idx = steps.findIndex((s) => s.kind === 'tool_delta' && s.id === st.id)
      if (idx >= 0) {
        const merged = { ...steps[idx], output: (steps[idx].output || '') + (st.output || '') }
        steps = steps.map((s, k) => (k === idx ? merged : s))
      } else steps = [...steps, st]
    } else if (st.id) {
      const idx = steps.findIndex((s) => s.id === st.id)
      if (idx < 0) {
        steps = [...steps, st]
      } else if (st.append) {
        const merged = {
          ...steps[idx],
          ...st,
          text: (steps[idx].text || '') + (st.text || ''),
          output: (steps[idx].output || '') + (st.output || ''),
        }
        steps = steps.map((s, k) => (k === idx ? merged : s))
      } else {
        steps = steps.map((s, k) => (k === idx ? st : s))
      }
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
    emitToast({
      type: 'prompt',
      enabled: notifyEnabled.current,
      title,
      body,
      tag: `ask:${sid}:${id ?? ''}`,
      // A tool-approval prompt gets its own cue so it's audibly distinct from a
      // plain question or a plan approval; both keep the type's default 'ask' cue.
      cue: ask.kind === 'permission' ? 'permission' : undefined,
    })
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
      // Every hub frame carries the server's unix seconds — feed it to the shared
      // clock so elapsed-time readings are measured against the SERVER, not the
      // browser's (possibly skewed) clock.
      noteServerTime(ev.time)
      switch (ev.kind) {
        case HubKind.UserMessage: {
          const m = ev.payload as Message
          if (!m?.id) return
          // A runtime-INJECTED note bridged from the bus (_Docs/58) does not itself
          // start an interactive turn — the autonomous turn it belongs to may be
          // deferred (coordinator turn cap) or is mid-run (auto-continue). So it must
          // NOT arm the "working" indicator here; that turn's own AgentStart/Step lights
          // it up if and when it actually runs. Covers the coordinator notes
          // (worker-note/coordination-guard) and the scheduler/auto-continue notes that
          // render as "⏰ …"/nudge cards rather than user bubbles. A genuine
          // turn-starting message (spawn/inbox opening turn, origin unset) still marks
          // the session busy immediately.
          const injectedNote =
            m.origin === 'worker-note' ||
            m.origin === 'coordination-guard' ||
            m.origin === 'auto-continue' ||
            m.origin === 'wake' ||
            m.origin === 'schedule'
          if (!injectedNote) {
            // The serial worker just started this turn → mark the session busy.
            setStreamingSessions((p) => withAdded(p, sid))
            setPendingSessions((p) => withAdded(p, sid))
          }
          // Our message just entered the transcript → refresh an open Session Info
          // panel (message count, size, context usage).
          bumpMeter()
          // The dispatched-head placeholder handed over to a real bubble: drop it,
          // so the tray shows it exactly until the transcript does.
          if (!injectedNote) {
            setQueued((prev) => prev.filter((p) => p.kind !== 'dispatching'))
          }
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
          // Definitive turn start, taken from the SERVER's stamp on this frame.
          // agent_start is a durable (ringed) event, so a window that joins or
          // reloads mid-turn replays it and gets the real start — not the moment
          // it happened to connect.
          ghostStartedAt = ev.time || serverNow()
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
        case HubKind.ToolDelta:
          // A long-running tool's output, streamed in chunks keyed by call id.
          // applyStep merges them into one card (ToolDeltaStep renders it). Unlike
          // Step this does not arm the busy latch: tool output only ever flows
          // inside a turn that already announced itself.
          applyStep(ev.payload as TurnStep)
          break
        case HubKind.Tombstone:
          // Retract a live step (typically a tool_delta placeholder). The server
          // publishes this after the streaming tool completes.
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
          const p = (ev.payload ?? {}) as {
            queue?: { clientMsgId: string; text: string }[]
            inflight?: { clientMsgId: string; text: string } | null
            turns?: { running?: TurnEntry | null; waiting?: TurnEntry[] }
          }
          const items: PendingItem[] = (p.queue ?? []).map((q) => ({
            id: q.clientMsgId,
            text: q.text,
            kind: 'queue',
            sid,
          }))
          // The dispatched head stays visible (as "gönderiliyor") until its user
          // bubble lands in the transcript: a message must never be in neither
          // place. Not cancellable — it is already running.
          if (p.inflight) {
            items.unshift({
              id: p.inflight.clientMsgId,
              text: p.inflight.text,
              kind: 'dispatching',
              sid,
            })
          }
          // The AUTONOMOUS half of the same queue (internal/turnqueue): what actually
          // holds the session and what else is queued for it. Shown only while the
          // user has something of their own waiting — otherwise the running turn is
          // already obvious from the transcript. This is what makes "my message is
          // waiting behind a worker notification" visible instead of a silent stall.
          const holder = p.turns?.running
          if (items.length > 0 && holder && holder.kind !== 'user') {
            items.unshift({
              id: `turn-${holder.kind}-${holder.since}`,
              text: turnKindLabel(holder),
              kind: 'holding',
              sid,
            })
          }
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
    // Fires on every disconnect — a transient reconnect AND the unsubscribe that
    // follows a session switch or unmount. FLUSH rather than cancel: a coalescing
    // window open at that moment holds deltas that have not been painted yet, and
    // on a transient drop no further event may arrive to carry them. Flushing
    // also leaves no timer armed past the subscription.
    onClose: () => {
      const pending = ghostDirty
      cancelGhostSync()
      if (pending) upsertGhost() // dropGhost clears the flag, so a finished turn no-ops
    },
  }
}

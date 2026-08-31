// Session-kind predicates shared by the shell (composer gating, initial-session
// pick) and the session detail panel (read-only badge). Pure logic, no React, so
// both the app layer and feature panels can import it without a cycle.

// isWritableSessionKind reports whether the user may send a new message into a
// session from the composer. Manual chats (chat/empty) plus "spawned" sessions
// qualify: spawned covers both the spawn tool and handoff (context-reset)
// children, which are single-agent linear transcripts explicitly meant for a
// human to take over and keep talking to.
//
// Task / flow transcripts are aggregate run logs produced by the orchestrator:
// they still appear in the (unified) sessions sidebar and are fully readable —
// transcript, context preview, debug panel, session info — but the composer is
// hidden for them, since a new user turn has no run to attach to. The same holds
// for an 'insight' transcript, which is the read-only audit log of one insight
// scan run.
//
// 'worker' IS writable (TSK507): a worker transcript is a live conversation its
// coordinator already injects user turns into (SendToWorker), so the human
// watching it must be able to answer, correct or add context there too rather
// than only read. Like 'schedule', the human turn and the coordinator's turn
// share the session's single turn slot on the backend and serialize.
//
// There is no 'inbox' kind any more: peer messages between agents are delivered
// into the recipient's standing 'chat' thread (internal/agent/agentmsg.go), which
// is writable because it is an ordinary chat.
//
// 'schedule' is the exception: it is not a per-run log but the agent's single
// long-lived cron thread, which every reuse-mode fire appends to. The user must be
// able to keep talking in it (answer, correct, add context) between ticks, so it
// keeps its composer. A user turn and a scheduled turn share the session's single
// turn slot on the backend, so they serialize rather than interleave.
//
// 'automation-run' and 'schedule-run' are a one-shot automation/schedule fire's
// own fresh session — a linear transcript like 'spawned', just tagged distinctly
// so the sidebar groups it under "Otomasyon" instead of "Spawn" (see
// sessionKindMeta.ts's kindChipKey).
//
// This list MIRRORS the backend source of truth, writableSessionKindList /
// IsWritableSessionKind in internal/db/models.go:252, which the API enforces in
// rejectNonWritableSession (internal/api/session_readonly.go). Change both sides
// together, or the composer and the API will disagree about the same session.
export function isWritableSessionKind(kind: string): boolean {
  return (
    kind === '' ||
    kind === 'chat' ||
    kind === 'spawned' ||
    kind === 'schedule' ||
    kind === 'automation-run' ||
    kind === 'schedule-run' ||
    kind === 'worker'
  )
}

package api

import (
	"github.com/bilal-arikan/tionswarm/internal/events"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// publishEntityChange emits a generic workspace-scoped change event for a nav
// entity (board/artifact/...). It flows through the same SSE pipeline as the
// chat/flow/schedule events, so open windows badge the relevant view + the
// workspace label and (per device prefs) raise a desktop toast — the unified
// "something changed" signal. Best-effort and nil-safe.
func publishEntityChange(wsp *workspace.Workspace, typ, title, body string, target map[string]string) {
	if wsp == nil || wsp.Runtime == nil {
		return
	}
	if target == nil {
		target = map[string]string{}
	}
	wsp.Runtime.Emit(events.Event{
		Type:   typ,
		Level:  "info",
		Title:  title,
		Body:   body,
		Target: target,
	})
}

// emitSessionChange is the per-session "session" notify every session mutation
// handler fires on success. It carries the sessionId in the target so the
// listener can decide whether to:
//   - always refresh the open session list (all op kinds).
//   - also reload the ACTIVE transcript when the changed session is the one on
//     screen (op-specific: rewind/delete_message/handoff/summary).
//   - bump the session-detail meter when the active session's own metadata
//     (title/workdir/pin) changed.
//
// `op` is a short, stable verb (create, delete, state, title, pin, workdir,
// agent, role, tags, rewind, delete_message, feedback, spawn, handoff, summary,
// message_added). Best-effort and nil-safe — callers can fire-and-forget.
func emitSessionChange(wsp *workspace.Workspace, id, op string) {
	if wsp == nil || wsp.Runtime == nil || id == "" {
		return
	}
	wsp.Runtime.Emit(events.Event{
		Type:   "session",
		Level:  "info",
		Target: map[string]string{"sessionId": id, "op": op},
	})
}

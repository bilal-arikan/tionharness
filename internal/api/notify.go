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

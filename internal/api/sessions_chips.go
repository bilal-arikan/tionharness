package api

import (
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/liveness"
)

// Server-side twin of the sidebar's chip filter
// (frontend/src/features/sessions/sessionKindMeta.ts). The sidebar pages the
// session list, so the chip selection has to narrow the list BEFORE paging:
// otherwise a page of 100 mixed sessions can contain three chats while the
// remaining chats sit far below the window and "Daha fazla yükle" appears to do
// nothing. total/hasMore are then counted over the filtered set too, so the
// footer count matches what the list can actually show.
//
// The two live-activity chips (running / awaiting-workers) are served here too
// since liveness became server state (agent.Runtime.Liveness, _Docs/77 R2): when
// ticked they NARROW the page to sessions in that state, and their badges count
// the scope's live rows, so the footer count is right for them as well.

const (
	chipRunning         = "running"
	chipAwaitingWorkers = "awaiting-workers"
)

const (
	chipWorker          = "worker"
	chipSubagent        = "subagent"
	chipArchived        = "archived"
	chipOther           = "other"
	maxSessionLookupIDs = 50
)

// parseSessionIDs supports the client's bounded exact lookup used during
// bootstrap. It keeps deep-linked and drafted sessions discoverable when they
// sit outside the first filtered page without loading the whole workspace.
func parseSessionIDs(raw string) map[string]bool {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	ids := make(map[string]bool)
	for parts := 0; raw != "" && parts < maxSessionLookupIDs; parts++ {
		part, rest, found := strings.Cut(raw, ",")
		if id := strings.TrimSpace(part); id != "" {
			ids[id] = true
		}
		if !found {
			break
		}
		raw = rest
	}
	return ids
}

// sessionChipKeys is the chip vocabulary the frontend renders, in display order.
// Kept in sync with SESSION_CHIPS; a kind that maps to none of them falls to
// chipOther so no session can become unfilterable.
var sessionChipKeys = []string{
	"chat", "task", "flow", "spawned", chipSubagent, "automation", "insight",
	"flow-coordinator", "inbox", chipOther, "running", "awaiting-workers",
	chipWorker, chipArchived,
}

func isSessionChipKey(k string) bool {
	for _, c := range sessionChipKeys {
		if c == k {
			return true
		}
	}
	return false
}

// kindChipKey maps a Session.Kind to the chip that owns it (mirrors the
// frontend helper of the same name).
func kindChipKey(kind string) string {
	switch kind {
	case "", "chat":
		return "chat"
	case "automation", "automation-run", "schedule", "schedule-run":
		return "automation"
	}
	if isSessionChipKey(kind) {
		return kind
	}
	return chipOther
}

// sessionChipKey classifies a session by its stable metadata first (category,
// then executionType), falling back to the legacy kind for older sessions.
func sessionChipKey(s db.Session) string {
	if s.Category == db.CategorySubagent {
		return chipSubagent
	}
	if s.Category != "" && isSessionChipKey(s.Category) {
		return s.Category
	}
	if s.ExecutionType == db.CategorySubagent {
		return chipSubagent
	}
	if s.ExecutionType != "" && isSessionChipKey(s.ExecutionType) {
		return s.ExecutionType
	}
	return kindChipKey(s.Kind)
}

// sessionIsWorker reports whether the session reports up to a coordinator —
// the lineage test the frontend's isWorkerSession uses.
func sessionIsWorker(s db.Session) bool {
	return s.CoordinatorSessionID != "" || s.Role == "worker"
}

// parseChips reads the `chips` query value (comma-separated selected chip keys).
// The second return is false when the parameter was absent, which means "no chip
// filter" — an EMPTY value is a real selection (nothing selected) and matches
// nothing, exactly like the sidebar with every chip unticked.
func parseChips(raw string, given bool) (map[string]bool, bool) {
	if !given {
		return nil, false
	}
	sel := make(map[string]bool)
	for _, part := range strings.Split(raw, ",") {
		if k := strings.TrimSpace(part); k != "" {
			sel[k] = true
		}
	}
	return sel, true
}

// sessionMatchesChips mirrors the sidebar predicate
// (frontend sessionKindMeta.sessionMatchesChips): the kind chip must be on, and
// an archived / worker / live session additionally needs its SCOPE chip on. The
// live chips are scope gates exactly like worker and archived — unticking
// "running" hides the sessions that are running right now and nothing else;
// an idle session is never touched by them. (The R2 cut had them the other way
// round — "ticked = show only live" — which, with the sidebar sending every chip
// by default, emptied the list down to the sessions in flight.)
//
// `running` wins over `awaiting-workers`: a coordinator that is itself
// streaming is gated by the running chip only, so a row is filtered by exactly
// one live scope (sessionLiveScope in the frontend).
func sessionMatchesChips(s db.Session, sel map[string]bool, live liveness.Snapshot) bool {
	if s.State == "archived" && !sel[chipArchived] {
		return false
	}
	if sessionIsWorker(s) && !sel[chipWorker] {
		return false
	}
	if scope := sessionLiveScope(s.ID, live); scope != "" && !sel[scope] {
		return false
	}
	return sel[sessionChipKey(s)]
}

// sessionLiveScope classifies a session on the live-activity axis: running,
// awaiting-workers, or "" when idle.
func sessionLiveScope(id string, live liveness.Snapshot) string {
	if live.Is(id, liveness.Running) {
		return chipRunning
	}
	if live.Is(id, liveness.AwaitingWorkers) {
		return chipAwaitingWorkers
	}
	return ""
}

// sessionChipCounts counts the supplied non-chip scope per chip so the sidebar
// badges keep reporting what a chip WOULD reveal even while the chip selection
// filters it out of the page. The live chips count the scope's rows in that
// liveness state.
func sessionChipCounts(sessions []db.Session, live liveness.Snapshot) map[string]int {
	counts := make(map[string]int, len(sessionChipKeys))
	for _, s := range sessions {
		key := sessionChipKey(s)
		counts[key]++
		// A worker-category session already counts under the worker chip through
		// its key; counting the scope again would double it.
		if key != chipWorker && sessionIsWorker(s) {
			counts[chipWorker]++
		}
		if s.State == "archived" {
			counts[chipArchived]++
		}
		if live.Is(s.ID, liveness.Running) {
			counts[chipRunning]++
		}
		if live.Is(s.ID, liveness.AwaitingWorkers) {
			counts[chipAwaitingWorkers]++
		}
	}
	return counts
}

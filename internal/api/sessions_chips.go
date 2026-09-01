package api

import (
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// Server-side twin of the sidebar's chip filter
// (frontend/src/features/sessions/sessionKindMeta.ts). The sidebar pages the
// session list, so the chip selection has to narrow the list BEFORE paging:
// otherwise a page of 100 mixed sessions can contain three chats while the
// remaining chats sit far below the window and "Daha fazla yükle" appears to do
// nothing. total/hasMore are then counted over the filtered set too, so the
// footer count matches what the list can actually show.
//
// The two live-activity chips (running / awaiting-workers) are deliberately NOT
// mirrored here: liveness is client state (streaming turns + the executions
// poll), unknown to this handler. They stay a client-side narrowing of the page.

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

// sessionMatchesChips mirrors the sidebar predicate: an archived or worker
// session additionally needs its scope chip, and the kind chip must be on.
func sessionMatchesChips(s db.Session, sel map[string]bool) bool {
	if s.State == "archived" && !sel[chipArchived] {
		return false
	}
	if sessionIsWorker(s) && !sel[chipWorker] {
		return false
	}
	return sel[sessionChipKey(s)]
}

// sessionChipCounts counts the supplied non-chip scope per chip so the sidebar
// badges keep reporting what a chip WOULD reveal even while the chip selection
// filters it out of the page. The live chips are absent by construction and stay
// client-computed.
func sessionChipCounts(sessions []db.Session) map[string]int {
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
	}
	return counts
}

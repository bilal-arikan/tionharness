package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// The live chips are SCOPE gates like worker/archived: ticked (the default —
// the sidebar sends every chip) leaves idle sessions alone, unticked hides the
// sessions in that liveness state. The R2 cut inverted this ("ticked = only
// live"), which emptied the sidebar down to whatever was running: every idle
// session looked lost. Regression guard for that.
func TestListSessionsLiveChipsAreScopeGates(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	idle, err := wsp.DB.CreateSession(ctx, db.Session{Title: "idle chat"})
	if err != nil {
		t.Fatalf("create idle: %v", err)
	}
	live, err := wsp.DB.CreateSession(ctx, db.Session{Title: "live chat"})
	if err != nil {
		t.Fatalf("create live: %v", err)
	}
	if server.runs == nil {
		server.runs = newChatRuns()
	}
	server.runs.register("run-live", live.ID, wsp.ID, func() {})

	const allChips = "chat,task,flow,spawned,subagent,automation,insight,flow-coordinator,inbox,other,running,awaiting-workers,worker,archived"
	all := listSessionsPage(t, server, wsp.DB, "chips="+allChips+"&limit=50")
	if all.Total != 2 {
		t.Fatalf("every chip ticked: total = %d, want 2 (idle sessions must not vanish)", all.Total)
	}
	if all.ChipCounts["running"] != 1 {
		t.Fatalf("running badge = %d, want 1", all.ChipCounts["running"])
	}

	noLive := listSessionsPage(t, server, wsp.DB, "chips=chat,worker,archived&limit=50")
	if noLive.Total != 1 || noLive.Items[0].ID != idle.ID {
		t.Fatalf("running unticked: got %+v, want only the idle chat", noLive.Items)
	}

	onlyLiveScope := listSessionsPage(t, server, wsp.DB, "chips=running&limit=50")
	if onlyLiveScope.Total != 0 {
		t.Fatalf("running without a kind chip: total = %d, want 0 (kind chip still required)", onlyLiveScope.Total)
	}

	chatAndRunning := listSessionsPage(t, server, wsp.DB, "chips=chat,running&limit=50")
	if chatAndRunning.Total != 2 {
		t.Fatalf("chat+running: total = %d, want 2", chatAndRunning.Total)
	}
}

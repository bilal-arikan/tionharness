package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// These tests pin the behaviour of the activity endpoints after the O(1)
// running-counter rewrite: the answers must be identical to what the old
// full-store scans produced, from every source, for both the idle and the busy
// case. They use the real workspace harness (newWorkspaceServer) because a live
// Runtime is one of the four sources being asserted.

// TestWorkspaceRunningIdleIsFalse: a workspace with nothing in flight must report
// idle. A false positive here pulses the switcher forever.
func TestWorkspaceRunningIdleIsFalse(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	if s.workspaceRunning(wsp) {
		t.Fatal("idle workspace reported running")
	}
}

// TestWorkspaceRunningSources checks each source lights the flag ON ITS OWN, and
// that the workspace goes quiet again once the source clears.
func TestWorkspaceRunningSources(t *testing.T) {
	ctx := context.Background()

	t.Run("streamed chat turn", func(t *testing.T) {
		s, wsp := newWorkspaceServer(t)
		s.runs.register("R1", "SES1", wsp.ID, func() {})
		if !s.workspaceRunning(wsp) {
			t.Fatal("live chat turn did not light the workspace")
		}
		s.runs.unregister("R1")
		if s.workspaceRunning(wsp) {
			t.Fatal("workspace still running after the turn unwound")
		}
	})

	t.Run("chat turn in another workspace does not leak", func(t *testing.T) {
		s, wsp := newWorkspaceServer(t)
		// The chat-run registry is SERVER-WIDE; a turn elsewhere must not pulse this one.
		s.runs.register("R1", "SES1", "SOME-OTHER-WS", func() {})
		if s.workspaceRunning(wsp) {
			t.Fatal("another workspace's turn lit this workspace")
		}
	})

	t.Run("running flow run", func(t *testing.T) {
		s, wsp := newWorkspaceServer(t)
		run, err := wsp.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1"})
		if err != nil {
			t.Fatalf("CreateFlowRun: %v", err)
		}
		if !s.workspaceRunning(wsp) {
			t.Fatal("running flow run did not light the workspace")
		}
		if err := wsp.DB.FinishFlowRun(ctx, run.ID, db.FlowSuccess, "", ""); err != nil {
			t.Fatalf("FinishFlowRun: %v", err)
		}
		if s.workspaceRunning(wsp) {
			t.Fatal("workspace still running after the flow run finished")
		}
	})
}

// TestWorkspaceRunningWaitingFlowRunIsNotRunning is the regression guard for the
// distinction the counter has to preserve: a run suspended at an await-input node
// is NOT running. It sleeps until input (boot never revives it), so pulsing the
// switcher for it would mark a workspace busy indefinitely.
func TestWorkspaceRunningWaitingFlowRunIsNotRunning(t *testing.T) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)

	run, err := wsp.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1"})
	if err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	if err := wsp.DB.MarkFlowRunWaiting(ctx, run.ID, "{}"); err != nil {
		t.Fatalf("MarkFlowRunWaiting: %v", err)
	}
	if s.workspaceRunning(wsp) {
		t.Fatal("a waiting flow run must not count as running")
	}

	// Resuming it flips the flag back on — the counter tracks both directions.
	if _, err := wsp.DB.ClaimWaitingFlowRun(ctx, run.ID); err != nil {
		t.Fatalf("ClaimWaitingFlowRun: %v", err)
	}
	if !s.workspaceRunning(wsp) {
		t.Fatal("a resumed flow run must count as running")
	}
}

// TestHandleActivityFlowFlag: the per-view endpoint must still light Akışlar and
// the unified Aktivite view for a running flow run, now that the flag comes from
// the counter rather than a scan.
func TestHandleActivityFlowFlag(t *testing.T) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)

	if st := getActivity(t, s, wsp); st.Flow || st.Executions {
		t.Fatalf("idle workspace reported flow=%v executions=%v", st.Flow, st.Executions)
	}

	if _, err := wsp.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1"}); err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}
	st := getActivity(t, s, wsp)
	if !st.Flow || !st.Executions {
		t.Fatalf("running flow run: flow=%v executions=%v, want both true", st.Flow, st.Executions)
	}
}

// TestHandleWorkspacesActivityReportsEveryWorkspace: the switcher feed must carry
// a row per workspace with the correct per-workspace flag — the cross-workspace
// isolation that made this endpoint scan every store in the first place.
func TestHandleWorkspacesActivityReportsEveryWorkspace(t *testing.T) {
	ctx := context.Background()
	s, busy := newWorkspaceServer(t)
	idle, err := s.workspaces.Create("idle", "", "test")
	if err != nil {
		t.Fatalf("create second workspace: %v", err)
	}
	if _, err := busy.DB.CreateFlowRun(ctx, db.FlowRun{FlowID: "FLW1"}); err != nil {
		t.Fatalf("CreateFlowRun: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/workspaces/activity", nil)
	rec := httptest.NewRecorder()
	s.handleWorkspacesActivity(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var got []workspaceActivityItem
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	running := map[string]bool{}
	for _, item := range got {
		running[item.ID] = item.Running
	}
	if !running[busy.ID] {
		t.Errorf("workspace with a running flow run reported idle")
	}
	if running[idle.ID] {
		t.Errorf("idle workspace reported running")
	}
}

// getActivity drives the real handler and decodes its reply.
func getActivity(t *testing.T, s *Server, wsp *workspace.Workspace) activityState {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/activity", nil)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleActivity(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var st activityState
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	return st
}

// TestCommandTurnLightsActivity: a slash command (/compact, /handoff) runs on the
// HTTP goroutine and owns neither the chat-run registry nor an autonomous invoke,
// so it published a live "working" bubble while /api/activity reported idle and
// the nav rail stayed dark. Activity now derives from the turn-admission queue,
// which EVERY turn entry path claims.
func TestCommandTurnLightsActivity(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	if s.workspaceRunning(wsp) {
		t.Fatal("idle workspace reported running")
	}
	release, err := wsp.Runtime.ClaimSessionCommandTurn(context.Background(), "SES1", "/compact")
	if err != nil {
		t.Fatalf("claim command turn: %v", err)
	}
	if !s.workspaceRunning(wsp) {
		t.Fatal("running slash command did not light the workspace")
	}
	if ids := wsp.Runtime.BusyTurnSessionIDs(); len(ids) != 1 || ids[0] != "SES1" {
		t.Fatalf("BusyTurnSessionIDs = %v, want [SES1]", ids)
	}
	release()
	if s.workspaceRunning(wsp) {
		t.Fatal("workspace still running after the command released its slot")
	}
}

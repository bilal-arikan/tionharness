package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestExecutionRuntimeMergesLiveStateWithoutStoreMutation(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	chat, err := wsp.DB.CreateSession(t.Context(), db.Session{Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	read := func() []executionRuntimeItem {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/executions/runtime", nil)
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
		resp := httptest.NewRecorder()
		s.handleListExecutionRuntime(resp, req)
		if resp.Code != http.StatusOK {
			t.Fatalf("status = %d", resp.Code)
		}
		var rows []executionRuntimeItem
		if err := json.Unmarshal(resp.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		return rows
	}
	if rows := read(); len(rows) != 0 {
		t.Fatalf("idle = %+v", rows)
	}
	gen := wsp.DB.MutationGen()
	s.runs.register("run", chat.ID, wsp.ID, func() {})
	defer s.runs.unregister("run")
	if rows := read(); len(rows) != 1 || !rows[0].Running || rows[0].SessionID != chat.ID {
		t.Fatalf("live = %+v", rows)
	}
	if gen != wsp.DB.MutationGen() {
		t.Fatal("test unexpectedly mutated store")
	}
	s.runs.unregister("run")
	if rows := read(); len(rows) != 0 {
		t.Fatalf("stopped = %+v", rows)
	}
	// A registry entry from another workspace must never enter this response.
	s.runs.register("other", chat.ID, "different-workspace", func() {})
	defer s.runs.unregister("other")
	if rows := read(); len(rows) != 0 {
		t.Fatalf("cross-workspace live state = %+v", rows)
	}
}

func TestExecutionRuntimePreservesHistoricalStatusesAndLineage(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := t.Context()
	task, err := wsp.DB.CreateTask(ctx, db.Task{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	for _, seed := range []db.Session{
		{Kind: "task", SourceID: task.ID},
		{Kind: "chat"},
		{Kind: "spawned", CoordinatorSessionID: "parent", RootCoordinatorSessionID: "root"},
	} {
		if _, err := wsp.DB.CreateSession(ctx, seed); err != nil {
			t.Fatal(err)
		}
	}
	all, _ := wsp.DB.ListSessions(ctx, "")
	want := map[string]executionRuntimeItem{}
	for _, session := range all {
		status := s.lastStatusFor(ctx, wsp, session)
		if status != "" || session.CoordinatorSessionID != "" {
			want[session.ID] = executionRuntimeItem{SessionID: session.ID, LastStatus: status, CoordinatorSessionID: session.CoordinatorSessionID, RootCoordinatorSessionID: session.RootCoordinator()}
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/executions/runtime", nil)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	response := httptest.NewRecorder()
	s.handleListExecutionRuntime(response, req)
	var rows []executionRuntimeItem
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
		t.Fatal(err)
	}
	got := map[string]executionRuntimeItem{}
	for _, row := range rows {
		got[row.SessionID] = row
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("runtime projection = %+v, want %+v", got, want)
	}
}

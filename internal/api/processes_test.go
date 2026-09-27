package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/procwatch"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// beginTestProcess registers an entry on the process-wide ledger the handlers
// read, and makes sure it is finished when the test ends so a later test never
// inherits a phantom running process.
func beginTestProcess(t *testing.T, m procwatch.Meta) *procwatch.Handle {
	t.Helper()
	h := procwatch.Default().Begin(t.Context(), m)
	t.Cleanup(func() { h.Finish(nil) })
	return h
}

func listProcesses(t *testing.T, query string) []procwatch.Entry {
	t.Helper()
	return listProcessesIn(t, &Server{}, nil, query)
}

// listProcessesIn runs the list handler with an active workspace on the request,
// which is what the owner enrichment needs (it reads that workspace's store).
func listProcessesIn(t *testing.T, s *Server, wsp *workspace.Workspace, query string) []procwatch.Entry {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/workspace/processes"+query, nil)
	if wsp != nil {
		req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	}
	rec := httptest.NewRecorder()
	s.handleListProcesses(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	var out []procwatch.Entry
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	return out
}

func TestListProcessesReturnsTheLedger(t *testing.T) {
	h := beginTestProcess(t, procwatch.Meta{Kind: procwatch.KindShell, Label: "Bash", Command: "go test ./api-list-probe"})

	var got *procwatch.Entry
	for _, e := range listProcesses(t, "") {
		if e.ID == h.ID() {
			got = &e
			break
		}
	}
	if got == nil {
		t.Fatalf("entry %q missing from the list", h.ID())
	}
	if got.Status != procwatch.StatusRunning || got.Command != "go test ./api-list-probe" {
		t.Errorf("entry = %+v, want the running probe command", got)
	}
}

func TestListProcessesFiltersByStatusAndKind(t *testing.T) {
	running := beginTestProcess(t, procwatch.Meta{Kind: procwatch.KindMCP, Command: "mcp-server --probe"})
	done := procwatch.Default().Begin(t.Context(), procwatch.Meta{Kind: procwatch.KindShell, Command: "echo done-probe"})
	done.Finish(nil)

	// Comma-separated and repeated params mean the same thing.
	for _, q := range []string{"?status=running&kind=mcp", "?status=running,killed&kind=mcp"} {
		ids := map[string]bool{}
		for _, e := range listProcesses(t, q) {
			ids[e.ID] = true
			if e.Status != procwatch.StatusRunning || e.Kind != procwatch.KindMCP {
				t.Errorf("%s returned %+v, which matches neither facet", q, e)
			}
		}
		if !ids[running.ID()] {
			t.Errorf("%s dropped the matching running mcp entry", q)
		}
		if ids[done.ID()] {
			t.Errorf("%s returned the finished shell entry", q)
		}
	}
}

func TestListProcessesResolvesTheAgentBehindASessionOnlyOwner(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := t.Context()
	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Prosesci", Provider: "claude-cli", Model: "test-model"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "t", AgentID: agentRow.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	// What the Interaction MCP bridge stamps: workspace + session, no agent.
	bridged := beginTestProcess(t, procwatch.Meta{
		Kind:    procwatch.KindShell,
		Command: "echo bridged-probe",
		Owner:   procwatch.Owner{WorkspaceID: wsp.ID, SessionID: sess.ID},
	})
	// A process whose session is already gone must still be listed.
	orphan := beginTestProcess(t, procwatch.Meta{
		Kind:    procwatch.KindShell,
		Command: "echo orphan-probe",
		Owner:   procwatch.Owner{WorkspaceID: wsp.ID, SessionID: "SES-deleted"},
	})

	found := map[string]procwatch.Owner{}
	for _, e := range listProcessesIn(t, s, wsp, "") {
		found[e.ID] = e.Owner
	}
	got, ok := found[bridged.ID()]
	if !ok {
		t.Fatalf("the bridged entry is missing from the list")
	}
	if got.AgentID != agentRow.ID || got.AgentName != "Prosesci" {
		t.Errorf("owner = %+v, want the session's agent id and name", got)
	}
	if orphanOwner, ok := found[orphan.ID()]; !ok {
		t.Error("the entry with a deleted session was dropped from the list")
	} else if orphanOwner.AgentName != "" {
		t.Errorf("orphan owner = %+v, want no agent invented for a missing session", orphanOwner)
	}
}

func TestStopProcessSignalsAndReportsOutcome(t *testing.T) {
	stopped := false
	h := procwatch.Default().Begin(t.Context(), procwatch.Meta{
		Kind: procwatch.KindShell, Command: "sleep probe", Stop: func() { stopped = true },
	})

	s := &Server{}
	call := func(id string) (int, map[string]any) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/workspace/processes/"+id+"/stop", nil)
		req.SetPathValue("id", id)
		s.handleStopProcess(rec, req)
		var body map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &body)
		return rec.Code, body
	}

	code, body := call(h.ID())
	if code != http.StatusOK || body["stopped"] != true {
		t.Fatalf("stop = (%d, %v), want (200, stopped=true)", code, body)
	}
	if !stopped {
		t.Error("the registered stop func was never called")
	}

	// A second stop, after the process is reaped, is a normal no-op — not a 500.
	h.Finish(nil)
	code, body = call(h.ID())
	if code != http.StatusOK || body["stopped"] != false || body["reason"] == nil {
		t.Errorf("stop on a finished entry = (%d, %v), want (200, stopped=false + reason)", code, body)
	}

	if code, _ := call("no-such-process"); code != http.StatusNotFound {
		t.Errorf("stop on an unknown id = %d, want 404", code)
	}
}

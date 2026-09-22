package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/procwatch"
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
	s := &Server{}
	rec := httptest.NewRecorder()
	s.handleListProcesses(rec, httptest.NewRequest(http.MethodGet, "/api/workspace/processes"+query, nil))
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

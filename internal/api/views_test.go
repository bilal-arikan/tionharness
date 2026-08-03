package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

// viewFixture stores a flow plus one run of it that has executed two nodes and is
// now sitting on a third.
func viewFixture(t *testing.T) (*db.DB, db.FlowRun) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	graph := `{"start":"s","nodes":[
		{"id":"s","type":"start","next":"a"},
		{"id":"a","type":"agent","title":"collect","agentId":"AG1","next":"b"},
		{"id":"b","type":"agent","title":"synth","agentId":"AG1"}]}`
	flow, err := database.CreateFlow(ctx, db.Flow{Name: "research-pipeline", Graph: graph})
	if err != nil {
		t.Fatalf("create flow: %v", err)
	}

	state := `{"current":"b","last":"ok","outputs":{"a":"ok"},"trace":[
		{"nodeId":"s","type":"start","title":"s","at":1000},
		{"nodeId":"a","type":"agent","title":"collect","output":"ok","at":32000}]}`
	run, err := database.CreateFlowRun(ctx, db.FlowRun{FlowID: flow.ID, Status: db.FlowRunning, State: state})
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if err := database.SetFlowRunState(ctx, run.ID, state); err != nil {
		t.Fatalf("save state: %v", err)
	}
	run, err = database.GetFlowRun(ctx, run.ID)
	if err != nil {
		t.Fatalf("reload run: %v", err)
	}
	return database, run
}

func decodeView(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode %q: %v", string(body), err)
	}
	return out
}

// TestGetViewProjectsFlowRun pins the end-to-end contract of the projection
// endpoint: the response carries the rendered text (what an agent receives and
// what the panel shows), a freshness stamp, a source revision and a cost
// estimate.
func TestGetViewProjectsFlowRun(t *testing.T) {
	database, run := viewFixture(t)

	rec := serveFlowRuns((&Server{}).handleGetView, database,
		"/api/views/flowrun/"+run.ID+"?level=card&lens=health",
		map[string]string{"kind": "flowrun", "id": run.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	got := decodeView(t, rec.Body.Bytes())
	text, _ := got["text"].(string)
	if !strings.Contains(text, "research-pipeline") {
		t.Errorf("flow name missing from projection:\n%s", text)
	}
	if !strings.Contains(text, "agent:collect✓") {
		t.Errorf("executed node missing:\n%s", text)
	}
	if !strings.Contains(text, "⚡RUNNING") {
		t.Errorf("current node missing:\n%s", text)
	}
	if got["source"] == "" || got["source"] == nil {
		t.Error("source revision not reported")
	}
	if n, _ := got["tokens"].(float64); n <= 0 {
		t.Errorf("token estimate not reported: %v", got["tokens"])
	}
}

// TestGetViewRejectsUnknownKind: an unsupported projection is a 400, never an
// empty 200. A blank summary reads like a healthy empty entity and would hide the
// caller's mistake.
func TestGetViewRejectsUnknownKind(t *testing.T) {
	database, run := viewFixture(t)

	rec := serveFlowRuns((&Server{}).handleGetView, database,
		"/api/views/galaxy/"+run.ID,
		map[string]string{"kind": "galaxy", "id": run.ID})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d (want 400): %s", rec.Code, rec.Body.String())
	}
}

// TestGetViewMissingRunIs404 keeps a deleted/mistyped run distinguishable from a
// run that simply has not started.
func TestGetViewMissingRunIs404(t *testing.T) {
	database, _ := viewFixture(t)

	rec := serveFlowRuns((&Server{}).handleGetView, database,
		"/api/views/flowrun/RUNnope",
		map[string]string{"kind": "flowrun", "id": "RUNnope"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d (want 404): %s", rec.Code, rec.Body.String())
	}
}

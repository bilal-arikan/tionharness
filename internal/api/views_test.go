package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/logbuf"
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

// TestGetViewProjectsBoard covers the board kind end to end: the store's tasks
// reach the projection and come back rendered.
//
// Staleness/blocked/overdue signals are NOT asserted here — the store stamps
// UpdatedAt on every write, so a test cannot age a card through the public API.
// Those rules are covered in internal/view/board_test.go, where the fixture
// controls the clock. (An earlier version of this test did assert staleness and
// passed only because of a seconds-vs-millis bug that made every card look
// years old.)
func TestGetViewProjectsBoard(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.CreateTask(ctx, db.Task{Title: "aktif", BoardState: db.BoardTodo}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := database.CreateTask(ctx, db.Task{Title: "devam", BoardState: db.BoardInProgress}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	rec := serveFlowRuns((&Server{}).handleGetView, database,
		"/api/views/board/board",
		map[string]string{"kind": "board", "id": "board"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	got := decodeView(t, rec.Body.Bytes())
	text, _ := got["text"].(string)
	if !strings.Contains(text, "2 kart") {
		t.Errorf("card count missing:\n%s", text)
	}
	if !strings.Contains(text, "todo 1 | in_progress 1") {
		t.Errorf("histogram missing:\n%s", text)
	}
	// Freshly created cards must read as recent, not ancient — the shape a unit
	// mix-up breaks first.
	if !strings.Contains(text, "Δ24s") {
		t.Errorf("freshly created cards not seen as recent:\n%s", text)
	}
	if unit, _ := got["elidedUnit"].(string); unit != "kart" {
		t.Errorf("elidedUnit = %q, want kart", unit)
	}
}

// TestGetViewProjectsSession pins that a session view answers from the header +
// tail without the caller having to read the transcript.
func TestGetViewProjectsSession(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	sess, err := database.CreateSession(ctx, db.Session{AgentID: "builder", Title: "auth refactor"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := database.AddMessage(ctx, db.Message{
		SessionID: sess.ID, Role: "assistant", Text: "ilerliyorum",
		Steps: `[{"kind":"todo","todos":[{"content":"a","status":"completed"},{"content":"b","status":"in_progress"}]}]`,
	}); err != nil {
		t.Fatalf("add message: %v", err)
	}

	rec := serveFlowRuns((&Server{}).handleGetView, database,
		"/api/views/session/"+sess.ID,
		map[string]string{"kind": "session", "id": sess.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}

	got := decodeView(t, rec.Body.Bytes())
	text, _ := got["text"].(string)
	if !strings.Contains(text, "auth refactor") || !strings.Contains(text, "agent:builder") {
		t.Errorf("session header wrong:\n%s", text)
	}
	if !strings.Contains(text, "todo: 1/2 tamam") {
		t.Errorf("checklist rollup missing:\n%s", text)
	}
}

// TestGetViewChildrenWalksTheMap covers the Explorer drill-down endpoint end to
// end: the workspace root returns its eleven structural children, and expanding the
// board returns one column node per column that exists.
func TestGetViewChildrenWalksTheMap(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	if _, err := database.CreateTask(ctx, db.Task{Title: "a", BoardState: db.BoardTodo}); err != nil {
		t.Fatalf("create task: %v", err)
	}
	if _, err := database.CreateTask(ctx, db.Task{Title: "b", BoardState: db.BoardInProgress}); err != nil {
		t.Fatalf("create task: %v", err)
	}

	rec := serveFlowRuns((&Server{}).handleGetViewChildren, database,
		"/api/views/workspace/workspace/children",
		map[string]string{"kind": "workspace", "id": "workspace"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeView(t, rec.Body.Bytes())
	children, _ := got["children"].([]any)
	if len(children) != 11 {
		t.Fatalf("workspace children = %d, want 11:\n%s", len(children), rec.Body.String())
	}

	rec = serveFlowRuns((&Server{}).handleGetViewChildren, database,
		"/api/views/board/board/children",
		map[string]string{"kind": "board", "id": "board"})
	if rec.Code != http.StatusOK {
		t.Fatalf("board children status %d: %s", rec.Code, rec.Body.String())
	}
	got = decodeView(t, rec.Body.Bytes())
	children, _ = got["children"].([]any)
	if len(children) != 2 {
		t.Errorf("board should expand into 2 column nodes, got %d:\n%s", len(children), rec.Body.String())
	}
}

// TestGetViewChildrenRejectsUnknownKind: expanding a kind with no children
// defined is a 400, never an empty 200 that would read like a real leaf.
func TestGetViewChildrenRejectsUnknownKind(t *testing.T) {
	database, _ := viewFixture(t)

	rec := serveFlowRuns((&Server{}).handleGetViewChildren, database,
		"/api/views/galaxy/x/children",
		map[string]string{"kind": "galaxy", "id": "x"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d (want 400): %s", rec.Code, rec.Body.String())
	}
}

// TestGetViewProjectsArtifactAndAutomation pins the TSK66 leaves end to end: a
// saved artifact and an automation rule render their metadata through the same
// projection endpoint as every other kind.
func TestGetViewProjectsArtifactAndAutomation(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	art, err := database.CreateArtifact(ctx, db.Artifact{
		Title: "görev raporu", Kind: db.ArtifactMarkdown, Origin: "tool", AgentID: "AG1",
	})
	if err != nil {
		t.Fatalf("create artifact: %v", err)
	}
	aut, err := database.CreateAutomation(ctx, db.Automation{
		Name: "todo→review", TriggerTag: "done", TargetAgentID: "AG2",
	})
	if err != nil {
		t.Fatalf("create automation: %v", err)
	}

	rec := serveFlowRuns((&Server{}).handleGetView, database,
		"/api/views/artifact/"+art.ID,
		map[string]string{"kind": "artifact", "id": art.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("artifact status %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeView(t, rec.Body.Bytes())
	text, _ := got["text"].(string)
	if !strings.Contains(text, "görev raporu") || !strings.Contains(text, "origin: tool") {
		t.Errorf("artifact projection wrong:\n%s", text)
	}

	rec = serveFlowRuns((&Server{}).handleGetView, database,
		"/api/views/automation/"+aut.ID,
		map[string]string{"kind": "automation", "id": aut.ID})
	if rec.Code != http.StatusOK {
		t.Fatalf("automation status %d: %s", rec.Code, rec.Body.String())
	}
	got = decodeView(t, rec.Body.Bytes())
	text, _ = got["text"].(string)
	if !strings.Contains(text, "todo→review") || !strings.Contains(text, "tetik: tag:done") {
		t.Errorf("automation projection wrong:\n%s", text)
	}
}

// TestGetViewProjectsLogs covers the logs leaf with a live ring buffer: entries
// captured through the slog handler must come back rendered.
func TestGetViewProjectsLogs(t *testing.T) {
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	logs := logbuf.New(50)
	logger := slog.New(logs.Handler(slog.NewTextHandler(io.Discard, nil)))
	logger.Info("turn ok")
	logger.Error("provider 429")

	srv := &Server{logs: logs}
	rec := serveFlowRuns(srv.handleGetView, database,
		"/api/views/logs/logs",
		map[string]string{"kind": "logs", "id": "logs"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
	}
	got := decodeView(t, rec.Body.Bytes())
	text, _ := got["text"].(string)
	if !strings.Contains(text, "provider 429") || !strings.Contains(text, "turn ok") {
		t.Errorf("logs projection wrong:\n%s", text)
	}
	if !strings.Contains(text, "LOGS · 2 kayıt") {
		t.Errorf("logs header wrong:\n%s", text)
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

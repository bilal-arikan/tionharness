package api

import (
	"context"
	"io"
	"log/slog"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// TestSessionSummaryFailureIsDurable pins the WS24/SES34 regression: a slash
// command that fails (there, the codex CLI reporting a revoked refresh token)
// used to leave NO durable trace — no debug-journal record and no persisted
// reply, so a refresh showed a "/compact" bubble with no answer. The failure
// must now be journaled AND persisted, while still returning 500.
func TestSessionSummaryFailureIsDurable(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	database, err := db.Open(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	// An unknown provider id makes compactSession fail on providers.Get — the same
	// shape as a provider that cannot authenticate.
	rt := agent.NewRuntime(database, providers.NewRegistry(), agent.NewTunables(), root, root, nil, nil, "WS1", "test", nil, logger)
	agentRow, err := database.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "no-such-provider"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := database.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	s := newTestServer()
	s.providers = providers.NewRegistry()
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "test"}, DB: database, Runtime: rt}

	req := httptest.NewRequest("POST", "/api/sessions/"+sess.ID+"/summary", strings.NewReader(`{"kind":"compact"}`))
	req.SetPathValue("id", sess.ID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleSessionSummary(rec, req)

	// (b) the endpoint still reports the failure to the caller.
	if rec.Code != 500 {
		t.Fatalf("expected 500 for a failed summary command, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "no-such-provider") {
		t.Fatalf("500 body must carry the provider error, got %s", rec.Body.String())
	}

	// (a) a durable assistant message carrying the provider error verbatim.
	msgs, err := database.ListMessages(ctx, sess.ID)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	var failure *db.Message
	for i := range msgs {
		if msgs[i].Role == providers.RoleAssistant {
			failure = &msgs[i]
		}
	}
	if failure == nil {
		t.Fatalf("failed command left no persisted assistant message (%d messages)", len(msgs))
	}
	if !strings.Contains(failure.Text, "no-such-provider") {
		t.Fatalf("failure message must quote the provider error verbatim, got:\n%s", failure.Text)
	}
	if !strings.Contains(failure.Text, "/compact") {
		t.Fatalf("failure message must name the command, got:\n%s", failure.Text)
	}

	// …and the debug journal records a safe classified summary. The caller and
	// durable assistant message above keep the actionable provider diagnostic;
	// debug.jsonl must not duplicate arbitrary provider text.
	evs, err := database.ReadDebugEvents(ctx, sess.ID, db.DebugError, 0)
	if err != nil {
		t.Fatalf("read debug events: %v", err)
	}
	if len(evs) == 0 {
		t.Fatal("failed command wrote no debug.jsonl error record")
	}
	last := evs[len(evs)-1]
	if !last.Err || last.Name != "/compact" || !strings.Contains(last.Detail, "[redacted]") || strings.Contains(last.Detail, "no-such-provider") {
		t.Fatalf("unexpected debug record: %+v", last)
	}
}

package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// waitForNextWallSecond blocks until the wall clock crosses into the next
// second. Store timestamps have unix-second precision, so without this a title
// written in the same second as the last message would look "unchanged" even if
// the store did bump it.
func waitForNextWallSecond() {
	start := time.Now().Unix()
	for time.Now().Unix() == start {
		time.Sleep(2 * time.Millisecond)
	}
}

// TestGenerateSessionTitleKeepsActivityWindow covers the rota bug end to end at
// the handler level: naming a session after the fact must not extend its
// activity window (CreatedAt→UpdatedAt), which is what the rota screen draws as
// the session's bar.
func TestGenerateSessionTitleKeepsActivityWindow(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()

	agent, err := database.CreateAgent(ctx, db.Agent{Name: "A", Provider: "anthropic"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := database.CreateSession(ctx, db.Session{AgentID: agent.ID})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	if _, err := database.AddMessage(ctx, db.Message{SessionID: sess.ID, Role: "user", Text: "iş bitti"}); err != nil {
		t.Fatalf("add message: %v", err)
	}
	before, err := database.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}

	waitForNextWallSecond()

	body, _ := json.Marshal(generateTitleReq{Title: "adı sonradan kondu"})
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/"+sess.ID+"/title", bytes.NewReader(body))
	req.SetPathValue("id", sess.ID)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, &workspace.Workspace{DB: database}))
	rec := httptest.NewRecorder()
	newTestServer().handleGenerateSessionTitle(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %q", rec.Code, rec.Body.String())
	}
	after, err := database.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("get session after: %v", err)
	}
	if after.Title != "adı sonradan kondu" {
		t.Fatalf("Title = %q, want the renamed one", after.Title)
	}
	if after.UpdatedAt != before.UpdatedAt {
		t.Fatalf("UpdatedAt moved from %d to %d: titling must not count as activity",
			before.UpdatedAt, after.UpdatedAt)
	}
}

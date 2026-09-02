package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestExecutionsExposeCoordinatorLineageAdditively(t *testing.T) {
	ctx := context.Background()
	s, wsp := newWorkspaceServer(t)

	root, err := wsp.DB.CreateSession(ctx, db.Session{
		Kind:            "chat",
		Title:           "root",
		CoordinatorMode: true,
	})
	if err != nil {
		t.Fatalf("CreateSession root: %v", err)
	}
	mid, err := wsp.DB.CreateSession(ctx, db.Session{
		Kind:                     "spawned",
		Title:                    "mid",
		CoordinatorMode:          true,
		CoordinatorSessionID:     root.ID,
		RootCoordinatorSessionID: root.ID,
	})
	if err != nil {
		t.Fatalf("CreateSession mid: %v", err)
	}
	leaf, err := wsp.DB.CreateSession(ctx, db.Session{
		Kind:                     "spawned",
		Title:                    "leaf",
		CoordinatorSessionID:     mid.ID,
		RootCoordinatorSessionID: root.ID,
	})
	if err != nil {
		t.Fatalf("CreateSession leaf: %v", err)
	}
	if _, err := wsp.DB.CreateSession(ctx, db.Session{Kind: "chat", Title: "plain"}); err != nil {
		t.Fatalf("CreateSession plain: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/executions", nil)
	req = req.WithContext(context.WithValue(req.Context(), workspaceCtxKey, wsp))
	rec := httptest.NewRecorder()
	s.handleListExecutions(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}

	var rows []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode: %v (body %s)", err, rec.Body.String())
	}
	byTitle := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		title, ok := row["title"].(string)
		if !ok {
			t.Fatalf("execution row missing string title: %#v", row)
		}
		byTitle[title] = row
	}

	assertExecutionWireValue(t, byTitle["root"], "sessionId", root.ID)
	assertExecutionWireValue(t, byTitle["root"], "kind", "chat")
	assertExecutionWireValue(t, byTitle["root"], "rootCoordinatorSessionId", root.ID)
	if _, exists := byTitle["root"]["coordinatorSessionId"]; exists {
		t.Error("root execution unexpectedly exposes coordinatorSessionId")
	}
	assertExecutionWireValue(t, byTitle["mid"], "coordinatorSessionId", root.ID)
	assertExecutionWireValue(t, byTitle["mid"], "rootCoordinatorSessionId", root.ID)
	assertExecutionWireValue(t, byTitle["leaf"], "sessionId", leaf.ID)
	assertExecutionWireValue(t, byTitle["leaf"], "coordinatorSessionId", mid.ID)
	assertExecutionWireValue(t, byTitle["leaf"], "rootCoordinatorSessionId", root.ID)
	if _, exists := byTitle["plain"]["coordinatorSessionId"]; exists {
		t.Error("plain execution unexpectedly exposes coordinatorSessionId")
	}
	if _, exists := byTitle["plain"]["rootCoordinatorSessionId"]; exists {
		t.Error("plain execution unexpectedly exposes rootCoordinatorSessionId")
	}
}

func assertExecutionWireValue(t *testing.T, row map[string]any, key, want string) {
	t.Helper()
	if row == nil {
		t.Fatalf("execution row is missing while checking %s", key)
	}
	if got := row[key]; got != want {
		t.Errorf("%s = %#v, want %q", key, got, want)
	}
}

package api

import (
	"context"
	"encoding/json"
	"github.com/bilal-arikan/tionharness/internal/db"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMessagePageBoundsAndPreview(t *testing.T) {
	server, workspace := newWorkspaceServer(t)
	ctx := context.Background()
	session, err := workspace.DB.CreateSession(ctx, db.Session{Title: "paged"})
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	for i := range 5 {
		steps, _ := json.Marshal([]map[string]any{{"kind": "tool", "output": strings.Repeat("x", 4000+i)}})
		message, err := workspace.DB.AddMessage(ctx, db.Message{SessionID: session.ID, Role: "assistant", Text: "reply", Steps: string(steps)})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, message.ID)
	}
	for _, test := range []struct {
		query               string
		code, count, offset int
	}{
		{"?limit=2", 200, 2, 3},
		{"?limit=2&before=" + ids[3], 200, 2, 1},
		{"?limit=2&after=" + ids[3], 200, 1, 4},
		{"?limit=0", 400, 0, 0},
		{"?limit=201", 400, 0, 0},
		{"?limit=2&before=missing", 404, 0, 0},
		{"?limit=2&before=x&after=y", 400, 0, 0},
	} {
		r := withTestWS(httptest.NewRequest("GET", "/api/sessions/"+session.ID+"/messages"+test.query, nil), workspace.DB)
		r.SetPathValue("id", session.ID)
		w := httptest.NewRecorder()
		server.handleListMessages(w, r)
		if w.Code != test.code {
			t.Fatalf("%s: %d %s", test.query, w.Code, w.Body.String())
		}
		if w.Code != 200 {
			continue
		}
		var page db.MessagePage
		if err := json.Unmarshal(w.Body.Bytes(), &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != test.count || page.Offset != test.offset || page.Total != 5 {
			t.Fatalf("%s: %+v", test.query, page)
		}
		if !strings.Contains(page.Items[0].Steps, "outputTruncated") {
			t.Fatal("page contains full trace")
		}
	}
	full, err := workspace.DB.FindMessage(ctx, session.ID, ids[0])
	if err != nil || strings.Contains(full.Steps, "outputTruncated") {
		t.Fatal("preview changed canonical trace")
	}
}

func TestPreviewCacheTracksContentAndBudget(t *testing.T) {
	first := `[{"kind":"tool","output":"` + strings.Repeat("a", 6000) + `"}]`
	second := strings.ReplaceAll(first, "a", "b")
	if trimStepsJSON(first) != trimStepsJSONUncached(first) {
		t.Fatal("preview mismatch")
	}
	if trimStepsJSON(first) == trimStepsJSON(second) {
		t.Fatal("changed trace reused an old preview")
	}
	stepsPreviews.Lock()
	defer stepsPreviews.Unlock()
	if stepsPreviews.bytes > stepsPreviewCacheBytes {
		t.Fatal("preview cache exceeded budget")
	}
}

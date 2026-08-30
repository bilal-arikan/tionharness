package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func TestListSessionsFiltersExecutionClassification(t *testing.T) {
	server, wsp := newWorkspaceServer(t)
	ctx := context.Background()
	for _, session := range []db.Session{
		{Title: "chat"},
		{Kind: "subagent", Title: "child", Category: db.CategorySubagent, ExecutionType: db.ExecutionSubagent},
		{Kind: "worker", Title: "worker", Category: db.CategoryWorker, ExecutionType: db.ExecutionWorker},
	} {
		if _, err := wsp.DB.CreateSession(ctx, session); err != nil {
			t.Fatalf("create session: %v", err)
		}
	}

	for _, test := range []struct {
		name  string
		query string
		want  string
	}{
		{name: "category", query: "category=subagent", want: "child"},
		{name: "execution type", query: "executionType=subagent", want: "child"},
		{name: "combined", query: "category=subagent&executionType=subagent", want: "child"},
	} {
		t.Run(test.name, func(t *testing.T) {
			req := withTestWS(httptest.NewRequest("GET", "/api/sessions?"+test.query, nil), wsp.DB)
			rec := httptest.NewRecorder()
			server.handleListSessions(rec, req)
			if rec.Code != 200 {
				t.Fatalf("status %d: %s", rec.Code, rec.Body.String())
			}
			var got []db.Session
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if len(got) != 1 || got[0].Title != test.want {
				t.Fatalf("sessions = %+v, want only %q", got, test.want)
			}
		})
	}
}

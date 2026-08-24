package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// listparams_test.go — TSK68 API surface: the list endpoints accept the same
// limit/offset/sort contract as the list_* tools. With no listing query they
// keep the legacy behavior (full, unwrapped array) so existing UI clients are
// untouched; with one they return the {items,total,offset,limit,hasMore}
// envelope. Malformed values are 400s, never silently ignored.

func TestListQueryParams(t *testing.T) {
	parse := func(qs string) (limit, offset int, field string, asc, listing bool, err error) {
		q, e := url.ParseQuery(qs)
		if e != nil {
			t.Fatalf("parse %q: %v", qs, e)
		}
		return listQueryParams(q)
	}

	// No params → legacy mode.
	if _, _, _, _, listing, err := parse(""); err != nil || listing {
		t.Fatalf("empty query: listing=%v err=%v, want false/nil", listing, err)
	}

	limit, offset, field, asc, listing, err := parse("limit=5&offset=10&sort=name_asc")
	if err != nil || !listing || limit != 5 || offset != 10 || field != "name" || !asc {
		t.Fatalf("full params = (%d,%d,%q,%v,%v,%v)", limit, offset, field, asc, listing, err)
	}

	// limit only → default offset, capped at max, and the tool-layer default
	// sort (updated_desc) so the API pages like the list_* tools.
	limit, _, field, asc, _, err = parse("limit=999")
	if err != nil || limit != 100 || field != "updated" || asc {
		t.Fatalf("limit cap = %d field %q asc %v err %v, want 100 updated false", limit, field, asc, err)
	}

	// Malformed values are errors.
	for _, qs := range []string{"limit=abc", "limit=-1", "limit=", "offset=abc", "offset=-2", "sort=bogus", "sort=name"} {
		if _, _, _, _, _, err := parse(qs); err == nil {
			t.Fatalf("parse(%q): want error, got nil", qs)
		}
	}
}

func TestBoolQuery(t *testing.T) {
	q, _ := url.ParseQuery("enabled=true&other=0")
	v, given, err := boolQuery(q, "enabled")
	if err != nil || !given || v == nil || !*v {
		t.Fatalf("enabled=true → %v/%v/%v", v, given, err)
	}
	v, given, err = boolQuery(q, "other")
	if err != nil || !given || v == nil || *v {
		t.Fatalf("other=0 → %v/%v/%v", v, given, err)
	}
	if _, given, _ = boolQuery(q, "missing"); given {
		t.Fatal("missing key must not be reported as given")
	}
	bad, _ := url.ParseQuery("enabled=yes")
	if _, _, err := boolQuery(bad, "enabled"); err == nil {
		t.Fatal("enabled=yes must error")
	}
}

// withTestWS stamps a workspace (DB only — these handlers touch nothing else)
// onto a request context, the same way the workspace middleware does.
func withTestWS(r *http.Request, database *db.DB) *http.Request {
	w := &workspace.Workspace{DB: database}
	return r.WithContext(context.WithValue(r.Context(), workspaceCtxKey, w))
}

type pageEnv struct {
	Items   json.RawMessage `json:"items"`
	Total   int             `json:"total"`
	Offset  int             `json:"offset"`
	Limit   int             `json:"limit"`
	HasMore bool            `json:"hasMore"`
}

func TestHandleListTasksPaginationAndFilters(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	for _, tk := range []db.Task{
		{Title: "A", BoardState: "todo", Priority: "high", OwnerAgentID: "AG1"},
		{Title: "B", BoardState: "in_progress", Priority: "low", OwnerAgentID: "AG2"},
		{Title: "C", BoardState: "todo", Priority: "low", OwnerAgentID: "AG1"},
	} {
		if _, err := database.CreateTask(ctx, tk); err != nil {
			t.Fatalf("create task: %v", err)
		}
	}
	s := newTestServer()

	// Legacy call: no query → plain array (UI compatibility).
	req := withTestWS(httptest.NewRequest("GET", "/api/tasks", nil), database)
	rec := httptest.NewRecorder()
	s.handleListTasks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("legacy status = %d, want 200", rec.Code)
	}
	var arr []db.Task
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil || len(arr) != 3 {
		t.Fatalf("legacy body must be a plain array of 3: %v %s", err, rec.Body.String())
	}

	// Paged call → envelope.
	req = withTestWS(httptest.NewRequest("GET", "/api/tasks?limit=2", nil), database)
	rec = httptest.NewRecorder()
	s.handleListTasks(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("paged status = %d, want 200", rec.Code)
	}
	var env pageEnv
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("paged body not envelope: %v", err)
	}
	if env.Total != 3 || env.Limit != 2 || !env.HasMore || len(env.Items) == 0 {
		t.Fatalf("paged = %+v; want total 3 limit 2 hasMore, 1+ items", env)
	}

	// Filter + paging combine.
	req = withTestWS(httptest.NewRequest("GET", "/api/tasks?boardState=todo&limit=1", nil), database)
	rec = httptest.NewRecorder()
	s.handleListTasks(rec, req)
	env = pageEnv{}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("filtered body not envelope: %v", err)
	}
	if env.Total != 2 || !env.HasMore {
		t.Fatalf("boardState=todo paged = %+v; want total 2 hasMore", env)
	}

	// Malformed sort → 400.
	req = withTestWS(httptest.NewRequest("GET", "/api/tasks?sort=bogus", nil), database)
	rec = httptest.NewRecorder()
	s.handleListTasks(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("bad sort status = %d, want 400", rec.Code)
	}
}

func TestHandleListAgentsPagination(t *testing.T) {
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer database.Close()
	for i := 0; i < 3; i++ {
		if _, err := database.CreateAgent(ctx, db.Agent{Name: fmt.Sprintf("Agent%d", i), Provider: "anthropic"}); err != nil {
			t.Fatalf("create agent: %v", err)
		}
	}
	s := newTestServer()

	req := withTestWS(httptest.NewRequest("GET", "/api/agents", nil), database)
	rec := httptest.NewRecorder()
	s.handleListAgents(rec, req)
	var arr []db.Agent
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil || len(arr) != 3 {
		t.Fatalf("legacy body must be a plain array of 3: %v %s", err, rec.Body.String())
	}

	req = withTestWS(httptest.NewRequest("GET", "/api/agents?limit=2&provider=ANTHROPIC", nil), database)
	rec = httptest.NewRecorder()
	s.handleListAgents(rec, req)
	var env pageEnv
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("paged body not envelope: %v", err)
	}
	if env.Total != 3 || env.Limit != 2 || !env.HasMore {
		t.Fatalf("agents paged = %+v; want total 3 limit 2 hasMore", env)
	}
}

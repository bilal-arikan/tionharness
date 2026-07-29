package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// flowRunFixture builds a two-level composed run tree in a fresh store:
//
//	root ── child ── grandchild
//	 └───── sibling                (a second root run of another flow)
//
// and returns the store plus the ids in creation order.
func flowRunFixture(t *testing.T) (*db.DB, map[string]db.FlowRun) {
	t.Helper()
	ctx := context.Background()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { database.Close() })

	runs := map[string]db.FlowRun{}
	mk := func(key, flowID, parent string) db.FlowRun {
		r := db.FlowRun{FlowID: flowID}
		if parent != "" {
			p := runs[parent]
			r.ParentRunID = p.ID
			r.ParentNodeID = "n-" + key
			r.RootRunID = p.RootOf()
		}
		created, err := database.CreateFlowRun(ctx, r)
		if err != nil {
			t.Fatalf("create %s: %v", key, err)
		}
		runs[key] = created
		return created
	}
	mk("root", "F1", "")
	mk("child", "F1", "root")
	mk("grandchild", "F1", "child")
	mk("sibling", "F2", "")
	return database, runs
}

// serveFlowRuns runs one request through the handler with a workspace bound to
// the context, the way withWorkspace does in production.
func serveFlowRuns(h http.HandlerFunc, database *db.DB, target string, pathValues map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	for k, v := range pathValues {
		req.SetPathValue(k, v)
	}
	ctx := context.WithValue(req.Context(), workspaceCtxKey, &workspace.Workspace{DB: database})
	rec := httptest.NewRecorder()
	h(rec, req.WithContext(ctx))
	return rec
}

func decodeRuns(t *testing.T, rec *httptest.ResponseRecorder) []db.FlowRun {
	t.Helper()
	var out []db.FlowRun
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode body %q: %v", rec.Body.String(), err)
	}
	return out
}

func ids(runs []db.FlowRun) []string {
	out := make([]string, len(runs))
	for i, r := range runs {
		out[i] = r.ID
	}
	return out
}

func containsRunID(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestListFlowRunsRootOnly locks the opt-in nature of the filter: without it the
// list still carries every run (so existing callers are untouched), with it only
// the runs nothing else launched.
func TestListFlowRunsRootOnly(t *testing.T) {
	database, runs := flowRunFixture(t)
	s := newTestServer()

	all := ids(decodeRuns(t, serveFlowRuns(s.handleListFlowRuns, database, "/api/flow-runs", nil)))
	if len(all) != 4 {
		t.Fatalf("unfiltered list = %v, want all 4 runs", all)
	}

	rec := serveFlowRuns(s.handleListFlowRuns, database, "/api/flow-runs?rootOnly=true", nil)
	rootOnly := ids(decodeRuns(t, rec))
	if len(rootOnly) != 2 {
		t.Fatalf("rootOnly list = %v, want the 2 root runs", rootOnly)
	}
	for _, key := range []string{"child", "grandchild"} {
		if containsRunID(rootOnly, runs[key].ID) {
			t.Fatalf("rootOnly list must not contain %s (%s): %v", key, runs[key].ID, rootOnly)
		}
	}

	// The flowId filter still applies on top of rootOnly.
	scoped := ids(decodeRuns(t, serveFlowRuns(s.handleListFlowRuns, database, "/api/flow-runs?rootOnly=true&flowId=F2", nil)))
	if len(scoped) != 1 || scoped[0] != runs["sibling"].ID {
		t.Fatalf("rootOnly+flowId=F2 = %v, want [%s]", scoped, runs["sibling"].ID)
	}

	// Only the exact value opts in — a bare/other value must not silently filter.
	loose := ids(decodeRuns(t, serveFlowRuns(s.handleListFlowRuns, database, "/api/flow-runs?rootOnly=1", nil)))
	if len(loose) != 4 {
		t.Fatalf("rootOnly=1 must not filter, got %v", loose)
	}
}

// TestFlowRunTreeNormalisesToRoot is the point of the endpoint: the UI asks with
// whatever run the user has selected — usually a child — and must still get the
// whole tree, parent before children.
func TestFlowRunTreeNormalisesToRoot(t *testing.T) {
	database, runs := flowRunFixture(t)
	s := newTestServer()

	want := []string{runs["root"].ID, runs["child"].ID, runs["grandchild"].ID}
	for _, from := range []string{"root", "child", "grandchild"} {
		rec := serveFlowRuns(s.handleFlowRunTree, database, "/api/flow-runs/x/tree", map[string]string{"id": runs[from].ID})
		if rec.Code != http.StatusOK {
			t.Fatalf("tree from %s: status %d", from, rec.Code)
		}
		got := ids(decodeRuns(t, rec))
		if len(got) != len(want) {
			t.Fatalf("tree from %s = %v, want %v", from, got, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("tree from %s = %v, want %v (breadth-first, parent first)", from, got, want)
			}
		}
	}

	// A run in another tree must not leak in, and a lone root is a tree of one.
	solo := ids(decodeRuns(t, serveFlowRuns(s.handleFlowRunTree, database, "/api/flow-runs/x/tree", map[string]string{"id": runs["sibling"].ID})))
	if len(solo) != 1 || solo[0] != runs["sibling"].ID {
		t.Fatalf("sibling tree = %v, want [%s]", solo, runs["sibling"].ID)
	}
}

// TestFlowRunTreeUnknownID keeps an unknown id a 404 rather than an empty list,
// so a stale deep link is distinguishable from a genuinely childless run.
func TestFlowRunTreeUnknownID(t *testing.T) {
	database, _ := flowRunFixture(t)
	s := newTestServer()

	rec := serveFlowRuns(s.handleFlowRunTree, database, "/api/flow-runs/x/tree", map[string]string{"id": "RUN-nope"})
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id status = %d, want 404", rec.Code)
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestWorkspaceIDFromRequest pins the scope resolution that a silently-ignored query
// param broke: a caller scoping by URL (?workspace=WS17) used to fall through to the
// header path, resolve to nothing, and get the DEFAULT workspace's data back under its
// own id. Every documented alias must be recognised, and a query-supplied id must be
// reported as explicit so withWorkspace rejects an unknown one instead of substituting.
func TestWorkspaceIDFromRequest(t *testing.T) {
	for _, key := range workspaceQueryKeys {
		r := httptest.NewRequest(http.MethodGet, "/api/mcp-servers?"+key+"=WS17", nil)
		id, fromQuery := workspaceIDFromRequest(r)
		if id != "WS17" || !fromQuery {
			t.Errorf("?%s= → id=%q fromQuery=%v; want WS17/true", key, id, fromQuery)
		}
	}

	// Header only: recognised, but NOT explicit — a stale localStorage id must stay
	// eligible for the graceful default fallback.
	r := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	r.Header.Set("X-Workspace-Id", "WS5")
	if id, fromQuery := workspaceIDFromRequest(r); id != "WS5" || fromQuery {
		t.Errorf("header → id=%q fromQuery=%v; want WS5/false", id, fromQuery)
	}

	// An explicit query param outranks the header: per-request scope beats the
	// client's ambient selection.
	r = httptest.NewRequest(http.MethodGet, "/api/agents?workspace=WS17", nil)
	r.Header.Set("X-Workspace-Id", "WS5")
	if id, fromQuery := workspaceIDFromRequest(r); id != "WS17" || !fromQuery {
		t.Errorf("query+header → id=%q fromQuery=%v; want WS17/true", id, fromQuery)
	}

	// Neither: caller gets the default, and it is not an explicit request.
	r = httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	if id, fromQuery := workspaceIDFromRequest(r); id != "" || fromQuery {
		t.Errorf("no scope → id=%q fromQuery=%v; want \"\"/false", id, fromQuery)
	}

	// Whitespace-only is not a scope (it would otherwise 400 as "unknown workspace").
	r = httptest.NewRequest(http.MethodGet, "/api/agents?ws=%20", nil)
	if id, fromQuery := workspaceIDFromRequest(r); id != "" || fromQuery {
		t.Errorf("blank ws → id=%q fromQuery=%v; want \"\"/false", id, fromQuery)
	}
}

package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearchIndexRefreshRejectsAnEmptyBody(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/search-indexes/refresh", strings.NewReader(`{}`))
	quietServer().handleSearchIndexRefresh(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

func TestSearchIndexRefreshRejectsMalformedJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/search-indexes/refresh", strings.NewReader(`{not json`))
	quietServer().handleSearchIndexRefresh(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

// A root alone is not enough: the tool names which index layout to act on, and
// acting on the wrong one would refresh a store the request never mentioned.
func TestSearchIndexRefreshRequiresBothToolAndRoot(t *testing.T) {
	for _, body := range []string{`{"root":"C:/repo"}`, `{"tool":"zg"}`, `{"tool":" ","root":" "}`} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/search-indexes/refresh", strings.NewReader(body))
		quietServer().handleSearchIndexRefresh(w, r)

		if w.Code != http.StatusBadRequest {
			t.Errorf("body %s: status = %d, want 400 (%s)", body, w.Code, w.Body.String())
		}
	}
}

// Without a workspace runtime there is nothing to run the indexer, and the
// handler must say so rather than panic on a nil runtime.
func TestSearchIndexRefreshWithoutAWorkspaceIsAConflict(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/search-indexes/refresh",
		strings.NewReader(`{"tool":"zg","root":"C:/repo"}`))
	quietServer().handleSearchIndexRefresh(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", w.Code, w.Body.String())
	}
}

// Like the drop, a refresh runs through a workspace runtime and therefore may
// not be served without one.
func TestSearchIndexRefreshIsNotWorkspaceOptional(t *testing.T) {
	if workspaceOptionalPath("/api/search-indexes/refresh") {
		t.Error("refreshing an index must go through a workspace runtime")
	}
}

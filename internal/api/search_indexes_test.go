package api

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/indexstate"
)

// quietServer is a Server with a real (discarding) logger, since the index
// handlers log every drop attempt and must not depend on a nil logger.
func quietServer() *Server {
	return &Server{logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func TestSearchIndexesReportsPhaseAndUsability(t *testing.T) {
	ledger := agent.IndexLedger()
	root := t.TempDir()
	ledger.Observe("zg", root, indexstate.PhaseReady, "local/potion-code-16m-v2", "1.0.0")
	t.Cleanup(func() { ledger.Forget("zg", root) })

	w := httptest.NewRecorder()
	quietServer().handleSearchIndexes(w, httptest.NewRequest(http.MethodGet, "/api/search-indexes", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (%s)", w.Code, w.Body.String())
	}
	var got []searchIndexStatus
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (%s)", err, w.Body.String())
	}
	var found *searchIndexStatus
	for i := range got {
		if strings.EqualFold(got[i].Root, root) {
			found = &got[i]
		}
	}
	if found == nil {
		t.Fatalf("root %q missing from %s", root, w.Body.String())
	}
	if found.Phase != string(indexstate.PhaseReady) || !found.Usable {
		t.Errorf("phase=%q usable=%v, want ready/true", found.Phase, found.Usable)
	}
	if found.Embedding != "local/potion-code-16m-v2" {
		t.Errorf("embedding=%q not reported", found.Embedding)
	}
}

func TestSearchIndexesReportsAFailureRatherThanHidingIt(t *testing.T) {
	ledger := agent.IndexLedger()
	root := t.TempDir()
	ledger.Begin("zg", root, indexstate.ActionCreate)
	ledger.Fail("zg", root, "zg exited 1")
	t.Cleanup(func() { ledger.Forget("zg", root) })

	w := httptest.NewRecorder()
	quietServer().handleSearchIndexes(w, httptest.NewRequest(http.MethodGet, "/api/search-indexes", nil))

	var got []searchIndexStatus
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	for _, e := range got {
		if !strings.EqualFold(e.Root, root) {
			continue
		}
		if e.Phase != string(indexstate.PhaseFailed) || e.Usable {
			t.Fatalf("a failed index surfaced as phase=%q usable=%v", e.Phase, e.Usable)
		}
		if e.Error != "zg exited 1" {
			t.Errorf("error=%q, want the recorded reason", e.Error)
		}
		return
	}
	t.Fatalf("root %q missing from %s", root, w.Body.String())
}

func TestSearchIndexDropRejectsAnEmptyBody(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/search-indexes/drop", strings.NewReader(`{}`))
	quietServer().handleSearchIndexDrop(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

func TestSearchIndexDropRejectsMalformedJSON(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/search-indexes/drop", strings.NewReader(`{not json`))
	quietServer().handleSearchIndexDrop(w, r)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (%s)", w.Code, w.Body.String())
	}
}

// The listing endpoint is workspace-optional (the ledger is process-wide), while
// the drop goes through a workspace runtime and therefore is not.
func TestSearchIndexListIsWorkspaceOptionalButDropIsNot(t *testing.T) {
	if !workspaceOptionalPath("/api/search-indexes") {
		t.Error("listing indexes should not require a workspace")
	}
	if workspaceOptionalPath("/api/search-indexes/drop") {
		t.Error("dropping an index must go through a workspace runtime")
	}
}

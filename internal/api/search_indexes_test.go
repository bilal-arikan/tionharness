package api

import (
	"encoding/json"
	"fmt"
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
	claim, _ := ledger.Begin("zg", root, indexstate.ActionCreate)
	if _, err := ledger.Fail("zg", root, claim.Run, "zg exited 1"); err != nil {
		t.Fatalf("Fail with the current claim: %v", err)
	}
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

// Without a workspace runtime there is nothing to delete through, and the
// handler must answer rather than dereference a nil runtime. Mirrors the same
// guard on the refresh endpoint.
func TestSearchIndexDropWithoutAWorkspaceIsAConflict(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/api/search-indexes/drop",
		strings.NewReader(`{"tool":"zg","root":"C:/repo","confirmRoot":"C:/repo"}`))
	quietServer().handleSearchIndexDrop(w, r)

	if w.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409 (%s)", w.Code, w.Body.String())
	}
	if msg := decodeAPIError(t, w.Body.Bytes()); msg == "" {
		t.Error("a refused drop answered 409 with no explanation in the body")
	}
}

// A confirmation gate that fails must come back as 409 WITH the reason: the
// Settings panel prints the body verbatim, so "confirm does not match the root"
// is the text the user acts on. A bare status would leave the panel with a
// failure it cannot explain.
func TestDropConfirmationFailuresMapTo409WithTheReason(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"not confirmed", indexstate.ErrDropNotConfirmed},
		{"root mismatch", indexstate.ErrDropRootMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			// The same mapping handleSearchIndexDrop applies to a runtime error.
			writeDropError(w, tc.err)

			if w.Code != http.StatusConflict {
				t.Fatalf("status = %d, want 409", w.Code)
			}
			if got := decodeAPIError(t, w.Body.Bytes()); got != tc.err.Error() {
				t.Errorf("body error = %q, want the sentinel text %q", got, tc.err.Error())
			}
		})
	}
}

// An unknown tool is a 404, not a 409: the request is well-formed and confirmed,
// there is simply no index layout to delete. Keeping it distinct stops the panel
// from telling the user to "confirm again" for something confirmation cannot fix.
func TestDropUnknownToolIsNotFound(t *testing.T) {
	w := httptest.NewRecorder()
	writeDropError(w, fmt.Errorf("%w: nope", agent.ErrUnknownIndexTool))

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (%s)", w.Code, w.Body.String())
	}
}

func decodeAPIError(t *testing.T, body []byte) string {
	t.Helper()
	var payload struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode error body: %v (%s)", err, body)
	}
	return payload.Error
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

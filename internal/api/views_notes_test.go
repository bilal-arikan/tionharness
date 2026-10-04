package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/notes"
)

// TestViewAPIProjectsNotesFromTheRuntimeStore pins that the HTTP projector carries
// the workspace's notes store the same way the agent's get_view does. The API
// projector used to be built without it: the Explorer map drew an empty Notlar
// bucket and GET /api/views/note/{id} answered "notes store unavailable" while
// the agent listed the very same notes (2026-10-04).
func TestViewAPIProjectsNotesFromTheRuntimeStore(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	store := wsp.Runtime.Notes()
	if store == nil {
		t.Fatal("workspace runtime has no notes store")
	}
	note, err := store.Put(notes.Note{
		Kind:       notes.KindDecision,
		Title:      "Map shows memory notes",
		Body:       "The Explorer map lists the workspace notes under Notlar.",
		Scope:      notes.ScopeWorkspace,
		Confidence: notes.ConfidenceInferred,
		Source:     notes.SourceUser,
	})
	if err != nil {
		t.Fatalf("put note: %v", err)
	}

	h := s.Routes()
	get := func(path string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.Header.Set("X-Workspace-Id", wsp.ID)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}

	rec := get("/api/views/note/" + note.ID + "?level=card")
	if rec.Code != http.StatusOK {
		t.Fatalf("note view status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Map shows memory notes") {
		t.Errorf("note view does not carry the note title:\n%s", rec.Body.String())
	}

	rec = get("/api/views/graph")
	if rec.Code != http.StatusOK {
		t.Fatalf("graph status %d: %s", rec.Code, rec.Body.String())
	}
	var graph struct {
		Nodes []struct {
			Ref struct {
				Kind string `json:"kind"`
				ID   string `json:"id"`
			} `json:"ref"`
		} `json:"nodes"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	found := false
	for _, n := range graph.Nodes {
		if n.Ref.Kind == "note" && n.Ref.ID == note.ID {
			found = true
		}
	}
	if !found {
		t.Errorf("graph has no node for note %s (%d nodes)", note.ID, len(graph.Nodes))
	}
}

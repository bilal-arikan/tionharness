package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/notes"
)

func noteRequest(t *testing.T, s *Server, wsID, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-Id", wsID)
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func TestNotesAPICreateListCorrectArchiveDelete(t *testing.T) {
	s, wsp := newWorkspaceServer(t)

	// Create (user source, defaults filled).
	rec := noteRequest(t, s, wsp.ID, http.MethodPost, "/api/notes", `{"kind":"decision","title":"Use JSONL","body":"No database.","tags":["storage"]}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	created := decodeInto[notes.Note](t, rec)
	if created.ID != "NOTE1" || created.Scope != notes.ScopeWorkspace || created.Confidence != notes.ConfidenceInferred || created.Source != notes.SourceUser {
		t.Fatalf("created: %+v", created)
	}
	// Validation → 400 naming the field.
	rec = noteRequest(t, s, wsp.ID, http.MethodPost, "/api/notes", `{"kind":"idea","title":"x","body":"y"}`)
	if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "kind") {
		t.Fatalf("bad kind: %d %s", rec.Code, rec.Body.String())
	}
	// A private note is listed for the human.
	rec = noteRequest(t, s, wsp.ID, http.MethodPost, "/api/notes", `{"kind":"reference","title":"Mine","body":"secret","private":true}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create private: %d", rec.Code)
	}
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/notes", "")
	if got := decodeInto[[]notes.Note](t, rec); len(got) != 2 {
		t.Fatalf("list: %d notes", len(got))
	}
	// Search.
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/notes/search?q=database", "")
	if hits := decodeInto[[]notes.Hit](t, rec); len(hits) != 1 || hits[0].Note.ID != "NOTE1" {
		t.Fatalf("search: %s", rec.Body.String())
	}
	// Correct → new note, old retired, default list hides the old one.
	rec = noteRequest(t, s, wsp.ID, http.MethodPost, "/api/notes/NOTE1/correct", `{"body":"JSONL plus an index.","confidence":"verified","verification":"measured"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("correct: %d %s", rec.Code, rec.Body.String())
	}
	fixed := decodeInto[notes.Note](t, rec)
	if fixed.Supersedes != "NOTE1" || fixed.Source != notes.SourceCorrection {
		t.Fatalf("correction: %+v", fixed)
	}
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/notes", "")
	for _, n := range decodeInto[[]notes.Note](t, rec) {
		if n.ID == "NOTE1" {
			t.Fatal("a superseded note must not be in the default listing")
		}
	}
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/notes?retired=true", "")
	if got := decodeInto[[]notes.Note](t, rec); len(got) != 3 {
		t.Fatalf("retired listing: %d", len(got))
	}
	// Expand shows the chain.
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/notes/NOTE1/expand", "")
	ex := decodeInto[notes.Expansion](t, rec)
	if len(ex.Successors) != 1 || ex.Successors[0].ID != fixed.ID {
		t.Fatalf("expand: %+v", ex)
	}
	// A chain member cannot be deleted (409); it can be archived.
	rec = noteRequest(t, s, wsp.ID, http.MethodDelete, "/api/notes/NOTE1", "")
	if rec.Code != http.StatusConflict {
		t.Fatalf("delete chain member: %d %s", rec.Code, rec.Body.String())
	}
	rec = noteRequest(t, s, wsp.ID, http.MethodPost, "/api/notes/"+fixed.ID+"/archive", `{"archived":true}`)
	if rec.Code != http.StatusOK || !decodeInto[notes.Note](t, rec).Archived {
		t.Fatalf("archive: %d %s", rec.Code, rec.Body.String())
	}
	// Update in place keeps provenance.
	rec = noteRequest(t, s, wsp.ID, http.MethodPut, "/api/notes/NOTE2", `{"title":"Mine (renamed)"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if up := decodeInto[notes.Note](t, rec); up.Title != "Mine (renamed)" || !up.Private || up.Source != notes.SourceUser {
		t.Fatalf("update: %+v", up)
	}
	// The unlinked private note can be deleted.
	rec = noteRequest(t, s, wsp.ID, http.MethodDelete, "/api/notes/NOTE2", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	// Stats + awareness settings + empty digest list.
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/notes/stats", "")
	if st := decodeInto[notes.Stats](t, rec); st.Total != 2 || st.Retired != 1 || st.Archived != 1 {
		t.Fatalf("stats: %+v", st)
	}
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/awareness/settings", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"briefBudgetBytes":6144`) {
		t.Fatalf("awareness settings: %d %s", rec.Code, rec.Body.String())
	}
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/awareness/digests", "")
	if rec.Code != http.StatusOK || strings.TrimSpace(rec.Body.String()) != "[]" {
		t.Fatalf("digests must serialise as []: %d %s", rec.Code, rec.Body.String())
	}
	rec = noteRequest(t, s, wsp.ID, http.MethodGet, "/api/sessions/SESX/digest", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("missing digest: %d", rec.Code)
	}
}

func TestWorkspaceSettingsCarryAwareness(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	rec := noteRequest(t, s, wsp.ID, http.MethodGet, "/api/workspace-settings", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("workspace settings: %d %s", rec.Code, rec.Body.String())
	}
	var dto struct {
		Awareness map[string]any `json:"awareness"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	if dto.Awareness["briefBudgetBytes"] != float64(6144) || dto.Awareness["enabled"] != true {
		t.Fatalf("settings DTO awareness: %+v", dto.Awareness)
	}
	// A PUT replaces the block and the runtime picks it up live.
	rec = noteRequest(t, s, wsp.ID, http.MethodPut, "/api/workspace-settings", `{"awareness":{"enabled":true,"briefBudgetBytes":3000}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	if got := wsp.Runtime.Awareness().Settings(); got.BriefBudgetBytes != 3000 || got.TurnBudgetBytes != 16384 {
		t.Fatalf("live settings: %+v", got)
	}
}

package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func sessionDecisionAPIFixture(t *testing.T) (*db.DB, db.Session, db.Session, http.Handler) {
	t.Helper()
	database, err := db.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	a, err := database.CreateAgent(ctx, db.Agent{Name: "Decision agent", Provider: "anthropic", ProviderInstanceID: "provider-1", Model: "agent-model", ThinkingLevel: "off"})
	if err != nil {
		t.Fatal(err)
	}
	session, err := database.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat", Model: "session-model"})
	if err != nil {
		t.Fatal(err)
	}
	other, err := database.CreateSession(ctx, db.Session{AgentID: a.ID, Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	server := &Server{}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/sessions/{id}/decisions", server.handleSessionDecisions)
	mux.HandleFunc("PUT /api/sessions/{id}/decisions/pin", server.handleSessionDecisionPin)
	mux.HandleFunc("POST /api/sessions/{id}/decisions/feedback", server.handleSessionDecisionFeedback)
	return database, session, other, mux
}

func sessionDecisionAPIRequest(t *testing.T, handler http.Handler, database *db.DB, method, sessionID, suffix, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	req := withTestWS(httptest.NewRequest(method, "/api/sessions/"+sessionID+"/decisions"+suffix, strings.NewReader(body)), database)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != status {
		t.Fatalf("%s %s: status %d, want %d: %s", method, req.URL.Path, rec.Code, status, rec.Body.String())
	}
	return rec
}

func TestSessionDecisionsAPIReadPrivacyAndDefaultRoute(t *testing.T) {
	database, session, other, handler := sessionDecisionAPIFixture(t)
	if err := database.UpdateSessionDecisions(context.Background(), session.ID, func(state *db.SessionDecisions) error {
		state.Memories = []db.DecisionMemory{{Key: "context-1", Label: "Architecture constraint", SourceID: "MSG1", Text: "PRIVATE_CONTEXT_TEXT"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	rec := sessionDecisionAPIRequest(t, handler, database, http.MethodGet, session.ID, "", "", http.StatusOK)
	state := decodeInto[db.SessionDecisions](t, rec)
	if strings.Contains(rec.Body.String(), "PRIVATE_CONTEXT_TEXT") || strings.Contains(rec.Body.String(), `"text"`) {
		t.Fatal("the inspector response exposed saved context text")
	}
	if len(state.Memories) != 1 || state.Memories[0].Label != "Architecture constraint" || state.Memories[0].SourceID != "MSG1" {
		t.Fatalf("memory provenance missing: %+v", state.Memories)
	}
	if state.Route == nil || state.Route.Provider != "provider-1" || state.Route.Model != "session-model" || state.Route.Pinned {
		t.Fatalf("default route = %+v", state.Route)
	}
	stored, err := database.ReadSessionDecisions(context.Background(), session.ID)
	if err != nil || stored.Memories[0].Text != "PRIVATE_CONTEXT_TEXT" || stored.Route != nil {
		t.Fatalf("GET changed canonical state: %+v, %v", stored, err)
	}
	otherState := decodeInto[db.SessionDecisions](t, sessionDecisionAPIRequest(t, handler, database, http.MethodGet, other.ID, "", "", http.StatusOK))
	if otherState.Route == nil || otherState.Route.Model != "agent-model" || len(otherState.Memories) != 0 || otherState.Entries == nil || otherState.Feedback == nil {
		t.Fatalf("fresh session state = %+v", otherState)
	}
	sessionDecisionAPIRequest(t, handler, database, http.MethodGet, "missing", "", "", http.StatusNotFound)
}

func TestSessionDecisionsAPIPinsAndSessionIsolation(t *testing.T) {
	database, session, other, handler := sessionDecisionAPIFixture(t)
	if err := database.UpdateSessionDecisions(context.Background(), session.ID, func(state *db.SessionDecisions) error {
		state.Memories = []db.DecisionMemory{{Key: "context-1", Label: "Constraint", Text: "private"}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, pinned := range []string{"true", "false"} {
		rec := sessionDecisionAPIRequest(t, handler, database, http.MethodPut, session.ID, "/pin", `{"key":"model","pinned":`+pinned+`}`, http.StatusOK)
		state := decodeInto[db.SessionDecisions](t, rec)
		if state.Route == nil || state.Route.Pinned != (pinned == "true") || state.Route.Model != "session-model" {
			t.Fatalf("model pin response = %+v", state.Route)
		}
		rec = sessionDecisionAPIRequest(t, handler, database, http.MethodPut, session.ID, "/pin", `{"key":"context-1","pinned":`+pinned+`}`, http.StatusOK)
		state = decodeInto[db.SessionDecisions](t, rec)
		if len(state.Memories) != 1 || state.Memories[0].Pinned != (pinned == "true") || state.Memories[0].Text != "" {
			t.Fatalf("context pin response = %+v", state.Memories)
		}
		stored, _ := database.ReadSessionDecisions(context.Background(), session.ID)
		if stored.Memories[0].Pinned != (pinned == "true") || stored.Memories[0].Text != "private" {
			t.Fatalf("pin did not persist or damaged saved context: %+v", stored.Memories)
		}
	}
	sessionDecisionAPIRequest(t, handler, database, http.MethodPut, other.ID, "/pin", `{"key":"context-1","pinned":true}`, http.StatusNotFound)
	sessionDecisionAPIRequest(t, handler, database, http.MethodPut, session.ID, "/pin", `{"key":"unknown","pinned":true}`, http.StatusNotFound)
	sessionDecisionAPIRequest(t, handler, database, http.MethodPut, "missing", "/pin", `{"key":"model","pinned":true}`, http.StatusNotFound)
	sessionDecisionAPIRequest(t, handler, database, http.MethodPut, session.ID, "/pin", `{"key":" ","pinned":true}`, http.StatusBadRequest)
}

func TestSessionDecisionsAPICannotUnpinRequiredContext(t *testing.T) {
	database, session, _, handler := sessionDecisionAPIFixture(t)
	if err := database.UpdateSessionDecisions(context.Background(), session.ID, func(state *db.SessionDecisions) error {
		state.Memories = []db.DecisionMemory{{Key: "required", Label: "Required instruction", Text: "retain", Pinned: true, Mandatory: true}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	sessionDecisionAPIRequest(t, handler, database, http.MethodPut, session.ID, "/pin", `{"key":"required","pinned":false}`, http.StatusBadRequest)
	stored, err := database.ReadSessionDecisions(context.Background(), session.ID)
	if err != nil || !stored.Memories[0].Pinned || stored.Memories[0].Text != "retain" {
		t.Fatalf("rejected unpin changed state: %+v, %v", stored, err)
	}
}

func TestSessionDecisionsAPIFeedbackUpdatesExistingRating(t *testing.T) {
	database, session, other, handler := sessionDecisionAPIFixture(t)
	if err := database.AppendSessionDecision(context.Background(), session.ID, db.SessionDecision{ID: "decision-1", Authority: "session-setup", Mode: "shadow", Status: "observed"}); err != nil {
		t.Fatal(err)
	}
	rec := sessionDecisionAPIRequest(t, handler, database, http.MethodPost, session.ID, "/feedback", `{"decisionId":"decision-1","rating":"helpful","note":" initial "}`, http.StatusOK)
	first := decodeInto[db.SessionDecisions](t, rec)
	if len(first.Feedback) != 1 || first.Feedback[0].Rating != "helpful" || first.Feedback[0].Note != "initial" || first.Feedback[0].At <= 0 {
		t.Fatalf("first feedback = %+v", first.Feedback)
	}
	rec = sessionDecisionAPIRequest(t, handler, database, http.MethodPost, session.ID, "/feedback", `{"decisionId":"decision-1","rating":"correction","note":" revised "}`, http.StatusOK)
	updated := decodeInto[db.SessionDecisions](t, rec)
	if len(updated.Feedback) != 1 || updated.Feedback[0].Rating != "correction" || updated.Feedback[0].Note != "revised" || updated.Feedback[0].At < first.Feedback[0].At {
		t.Fatalf("updated feedback = %+v", updated.Feedback)
	}
	stored, _ := database.ReadSessionDecisions(context.Background(), session.ID)
	if len(stored.Feedback) != 1 || stored.Feedback[0] != updated.Feedback[0] {
		t.Fatalf("feedback response is not persisted state: %+v", stored.Feedback)
	}
	for _, body := range []string{
		`{"decisionId":"decision-1","rating":"unknown"}`,
		`{"decisionId":"","rating":"helpful"}`,
		`{"decisionId":"decision-1","rating":"helpful","note":"` + strings.Repeat("x", 1001) + `"}`,
		`{`,
	} {
		sessionDecisionAPIRequest(t, handler, database, http.MethodPost, session.ID, "/feedback", body, http.StatusBadRequest)
	}
	sessionDecisionAPIRequest(t, handler, database, http.MethodPost, session.ID, "/feedback", `{"decisionId":"missing","rating":"helpful"}`, http.StatusNotFound)
	sessionDecisionAPIRequest(t, handler, database, http.MethodPost, other.ID, "/feedback", `{"decisionId":"decision-1","rating":"helpful"}`, http.StatusNotFound)
	sessionDecisionAPIRequest(t, handler, database, http.MethodPost, "missing", "/feedback", `{"decisionId":"decision-1","rating":"helpful"}`, http.StatusNotFound)
	stored, _ = database.ReadSessionDecisions(context.Background(), session.ID)
	if len(stored.Feedback) != 1 || stored.Feedback[0] != updated.Feedback[0] {
		t.Fatal("rejected feedback changed the existing rating")
	}
}

func TestSessionDecisionsAPIRequestBodyCap(t *testing.T) {
	database, session, _, handler := sessionDecisionAPIFixture(t)
	padding := strings.Repeat(" ", 4096)
	sessionDecisionAPIRequest(t, handler, database, http.MethodPut, session.ID, "/pin", padding+`{"key":"model","pinned":true}`, http.StatusBadRequest)
	sessionDecisionAPIRequest(t, handler, database, http.MethodPost, session.ID, "/feedback", padding+`{"decisionId":"decision-1","rating":"helpful"}`, http.StatusBadRequest)
	stored, err := database.ReadSessionDecisions(context.Background(), session.ID)
	if err != nil || stored.Route != nil || len(stored.Feedback) != 0 {
		t.Fatalf("oversized body changed state: %+v, %v", stored, err)
	}
}

package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func asyncAskFixture(t *testing.T) (*Server, *workspace.Workspace, *chatRun, *tools.AsyncInput) {
	t.Helper()
	s, wsp := newWorkspaceServer(t)
	session, err := wsp.DB.CreateSession(context.Background(), db.Session{Kind: "chat", AgentID: "A1"})
	if err != nil {
		t.Fatal(err)
	}
	// Freeze dispatch so assertions inspect the real persisted inbox without
	// starting a provider or leaving a waiting worker goroutine behind.
	s.inbox.sessions[scopeKey(wsp.ID, session.ID)] = &sessionInbox{wsID: wsp.ID, seen: map[string]bool{}, closing: true}
	run := s.runs.register("async-test", session.ID, wsp.ID, func() {})
	s.runs.activate(run)
	t.Cleanup(func() { s.runs.unregister(run.id) })
	turn := &chatTurn{s: s, wsp: wsp, database: wsp.DB, session: session, run: run, ctx: context.Background()}
	input := turn.asyncInputForAgent("A1")
	run.setAsyncInput(input)
	return s, wsp, run, input
}

func answerAsyncTest(s *Server, wsp *workspace.Workspace, sessionID, id, answer string) bool {
	r := httptest.NewRequest("POST", "/", nil)
	r = r.WithContext(context.WithValue(r.Context(), workspaceCtxKey, wsp))
	return s.answerDurableAsk(r, sessionID, id, answer, "test")
}

func TestAsyncAskCLIRoundTrip(t *testing.T) {
	s, wsp, run, _ := asyncAskFixture(t)
	b := &interactionBackend{runs: s.runs, apiSrv: s, tun: s.tun}
	res, err := b.Call(context.Background(), run.token, "ask_user_async", json.RawMessage(`{"question":"Color?"}`))
	if err != nil || res.IsError {
		t.Fatalf("post=%+v %v", res, err)
	}
	var posted struct {
		ID string `json:"request_id"`
	}
	if err := json.Unmarshal([]byte(res.Text), &posted); err != nil || posted.ID == "" {
		t.Fatalf("post=%s", res.Text)
	}
	// The CLI can execute another tool before any answer exists.
	res, err = b.Call(context.Background(), run.token, "active_tools", json.RawMessage(`{}`))
	if err != nil || res.IsError || len(res.UserInput) != 0 {
		t.Fatalf("independent call=%+v %v", res, err)
	}
	if answerAsyncTest(s, wsp, "wrong-session", posted.ID, "wrong") {
		t.Fatal("cross-session answer accepted")
	}
	if !answerAsyncTest(s, wsp, run.sessionID, posted.ID, "Blue") {
		t.Fatal("answer rejected")
	}
	if answerAsyncTest(s, wsp, run.sessionID, posted.ID, "Red") {
		t.Fatal("duplicate answer accepted")
	}
	permission, err := b.Call(context.Background(), run.token, "permission_prompt", json.RawMessage(`{"tool_name":"Read","input":{}}`))
	if err != nil || len(permission.UserInput) != 0 || !json.Valid([]byte(permission.Text)) {
		t.Fatalf("permission contract corrupted: %+v %v", permission, err)
	}
	res, err = b.Call(context.Background(), run.token, "active_tools", json.RawMessage(`{}`))
	if err != nil || len(res.UserInput) != 1 || !strings.Contains(res.UserInput[0], "Blue") || !strings.Contains(res.UserInput[0], posted.ID) {
		t.Fatalf("delivery=%+v %v", res, err)
	}
	s.finishAsyncAnswers(run, wsp, run.sessionID)
	if got := queuedMessages(s, wsp.ID, run.sessionID); len(got) != 0 {
		t.Fatalf("reply duplicated: %v", got)
	}
}

func TestAsyncAnswerFinalizationRace(t *testing.T) {
	for _, timing := range []string{"before", "after", "concurrent"} {
		t.Run(timing, func(t *testing.T) {
			s, wsp, run, input := asyncAskFixture(t)
			id, err := input.Ask(context.Background(), []tools.AskQuestion{{Question: "Audience?"}})
			if err != nil {
				t.Fatal(err)
			}
			answer := func() {
				if !answerAsyncTest(s, wsp, run.sessionID, id, "Engineers") {
					t.Error("answer rejected")
				}
			}
			finish := func() { s.finishAsyncAnswers(run, wsp, run.sessionID) }
			switch timing {
			case "before":
				answer()
				finish()
			case "after":
				finish()
				answer()
			default:
				var wg sync.WaitGroup
				wg.Go(answer)
				wg.Go(finish)
				wg.Wait()
			}
			got := queuedMessages(s, wsp.ID, run.sessionID)
			if len(got) != 1 || !strings.Contains(got[0], "Engineers") {
				t.Fatalf("queued=%v", got)
			}
		})
	}
}

func TestAsyncAskStopCancelsUnansweredQuestions(t *testing.T) {
	s, wsp, run, input := asyncAskFixture(t)
	id, err := input.Ask(context.Background(), []tools.AskQuestion{{Question: "Audience?"}})
	if err != nil {
		t.Fatal(err)
	}
	if !s.stopSessionTurn(wsp, run.sessionID) {
		t.Fatal("stop not handled")
	}
	if answerAsyncTest(s, wsp, run.sessionID, id, "late") {
		t.Fatal("stopped question accepted an answer")
	}
	ask, _ := wsp.DB.GetSessionAsk(context.Background(), id)
	if ask.Status != db.SessionAskCancelled {
		t.Fatalf("status=%s", ask.Status)
	}
}

func TestAsyncAskStopDoesNotRestartFromUnconsumedAnswer(t *testing.T) {
	s, wsp, run, input := asyncAskFixture(t)
	id, err := input.Ask(context.Background(), []tools.AskQuestion{{Question: "Audience?"}})
	if err != nil {
		t.Fatal(err)
	}
	if !answerAsyncTest(s, wsp, run.sessionID, id, "Engineers") {
		t.Fatal("answer rejected")
	}
	// Exercise the worst ordering: cancellation immediately finalizes the turn.
	run.cancel = func() { s.finishAsyncAnswers(run, wsp, run.sessionID) }
	s.stopSessionTurn(wsp, run.sessionID)
	s.finishAsyncAnswers(run, wsp, run.sessionID)
	if got := queuedMessages(s, wsp.ID, run.sessionID); len(got) != 0 {
		t.Fatalf("stop restarted session: %v", got)
	}
}

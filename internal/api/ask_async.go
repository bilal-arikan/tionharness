package api

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/sessionhub"
	"github.com/bilal-arikan/tionharness/internal/tools"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func (r *chatRun) setAsyncInput(input *tools.AsyncInput) {
	r.mu.Lock()
	r.asyncInput = input
	r.mu.Unlock()
}

func (r *chatRun) asyncInputFor() *tools.AsyncInput {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.asyncFinalized || r.asyncStopped {
		return nil
	}
	return r.asyncInput
}

func (t *chatTurn) asyncInputForAgent(agentID string) *tools.AsyncInput {
	// CLI callbacks can outlive an agent pass; do not read mutable turn fields
	// after the next pass refreshes its context or session snapshot.
	turnCtx, sessionID := t.ctx, t.session.ID
	return &tools.AsyncInput{
		Ask: func(ctx context.Context, questions []tools.AskQuestion) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			if err := turnCtx.Err(); err != nil {
				return "", err
			}
			var ask db.SessionAsk
			var err error
			if !t.withGeneration(func() {
				ask, err = t.s.createAsyncAsk(ctx, t.wsp, sessionID, agentID, questions)
			}) {
				return "", fmt.Errorf("turn no longer owns this session")
			}
			// Stop may race persistence after the initial context check.
			if err == nil && (ctx.Err() != nil || turnCtx.Err() != nil) {
				if closed, closeErr := t.database.CloseSessionAsk(context.Background(), ask.ID, db.SessionAskCancelled); closeErr == nil && closed {
					t.s.publishHub(t.wsp.ID, sessionID, sessionhub.KindInteractionResolve, map[string]any{"id": ask.ID, "cancelled": true, "reason": "stopped"}, false)
				}
				return "", context.Canceled
			}
			return ask.ID, err
		},
		Drain: func() []string {
			var replies []string
			t.withGeneration(func() {
				for _, ask := range t.s.takeAsyncAnswers(t.wsp, sessionID, agentID) {
					replies = append(replies, asyncAnswerText(ask))
				}
			})
			return replies
		},
	}
}

func (s *Server) createAsyncAsk(ctx context.Context, wsp *workspace.Workspace, sessionID, agentID string, questions []tools.AskQuestion) (db.SessionAsk, error) {
	payload, err := json.Marshal(map[string]any{"questions": questions, "async": true})
	if err != nil {
		return db.SessionAsk{}, err
	}
	ask, err := wsp.DB.CreateSessionAsk(ctx, db.SessionAsk{
		SessionID: sessionID, AgentID: agentID, Kind: "ask", Async: true, Payload: string(payload),
	})
	if err == nil {
		s.openDurableAskCard(wsp.ID, sessionID, ask, false)
	}
	return ask, err
}

func asyncAnswerText(ask db.SessionAsk) string {
	questions, _ := tools.ParseAskInputMulti(json.RawMessage(ask.Payload))
	answer := tools.FormatMultiAnswer(questions, ask.Answer)
	// Single-question clients may send plain text. Keep the question alongside
	// it so replies remain unambiguous when several requests were pending.
	if len(questions) == 1 && answer == ask.Answer {
		answer = questions[0].Question + "\nAnswer: " + answer
	}
	return fmt.Sprintf("[User reply to ask_user_async request_id=%s]\n%s", ask.ID, answer)
}

func (s *Server) takeAsyncAnswers(wsp *workspace.Workspace, sessionID, agentID string) []db.SessionAsk {
	if wsp == nil || wsp.DB == nil {
		return nil
	}
	asks, err := wsp.DB.TakeAsyncSessionAnswers(context.Background(), sessionID, agentID)
	if err != nil && s.logger != nil {
		s.logger.Warn("async answer delivery failed", "session", sessionID, "error", err)
	}
	return asks
}

func (s *Server) queueAsyncAnswers(wsp *workspace.Workspace, sessionID string) {
	for _, ask := range s.takeAsyncAnswers(wsp, sessionID, "") {
		req := chatReq{SessionID: sessionID, Message: asyncAnswerText(ask)}
		if ask.AgentID != "" {
			req.AgentIDs = []string{ask.AgentID}
		}
		s.enqueueMessage(wsp.ID, req, "async-answer:"+ask.ID)
	}
}

// Mark finalization BEFORE collecting replies. An answer that races this drain
// either appears in it or sees the finalized flag and enqueues itself.
func (s *Server) finishAsyncAnswers(run *chatRun, wsp *workspace.Workspace, sessionID string) {
	run.mu.Lock()
	run.asyncFinalized = true
	stopped := run.asyncStopped
	run.mu.Unlock()
	if stopped {
		s.cancelAsyncAsks(wsp, sessionID)
		return
	}
	s.queueAsyncAnswers(wsp, sessionID)
}

func (s *Server) deliverAsyncAnswer(wsp *workspace.Workspace, sessionID string) {
	run, _ := s.steerTargetRun(wsp.ID, sessionID)
	if run != nil {
		run.mu.Lock()
		stopped := run.asyncStopped
		run.mu.Unlock()
		if stopped {
			s.cancelAsyncAsks(wsp, sessionID)
			return
		}
	}
	if run != nil && run.asyncInputFor() != nil {
		return
	}
	s.queueAsyncAnswers(wsp, sessionID)
}

func (s *Server) cancelAsyncAsks(wsp *workspace.Workspace, sessionID string) bool {
	if wsp == nil || wsp.DB == nil {
		return false
	}
	asks, err := wsp.DB.CancelAsyncSessionAsks(context.Background(), sessionID)
	if err != nil && s.logger != nil {
		s.logger.Warn("cancel async questions failed", "session", sessionID, "error", err)
	}
	for _, ask := range asks {
		s.publishHub(wsp.ID, sessionID, sessionhub.KindInteractionResolve, map[string]any{
			"id": ask.ID, "cancelled": true, "reason": "stopped",
		}, false)
	}
	return len(asks) > 0
}

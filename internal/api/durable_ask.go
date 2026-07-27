package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/bilal-arikan/tionswarm/internal/agent"
	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/sessionhub"
	"github.com/bilal-arikan/tionswarm/internal/workspace"
)

// Durable Ask (MVP) — API wiring. A native interactive turn that called ask_user
// at a clean point is parked by the runtime (db.SessionAsk); here we open the card
// on the session hub, route the answer through the single-winner CAS, and re-drive
// the turn in the background so the continuation streams to every window over the
// hub (the event-sourced chat model, _Docs/58). See _Docs/65.

// openDurableAskCard broadcasts a durably-suspended ask_user prompt on the session
// hub so every window renders it — the durable analog of openInteraction. The card
// id is the SessionAsk id (SAK…), which the answer endpoint routes to the durable
// resume; durable:true tells the frontend to treat it as a durable card.
func (s *Server) openDurableAskCard(sessionID string, ask db.SessionAsk, ephemeral bool) {
	var payload map[string]any
	if ask.Payload != "" {
		_ = json.Unmarshal([]byte(ask.Payload), &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	payload["id"] = ask.ID
	payload["kind"] = ask.Kind // "ask" | "permission"
	payload["durable"] = true
	// The initial suspend + re-suspend publish durably (seq'd on the ring) so the
	// submitting window renders it and a reconnect gap-fills it. Boot restore
	// re-publishes ephemerally per-subscribe (the disk row is the source of truth),
	// so the ring is never polluted with duplicate cards across reconnects.
	s.publishHub(sessionID, sessionhub.KindInteractionOpen, payload, ephemeral)
}

// answerDurableAsk routes an interaction answer to a durably-suspended ask when it
// is not an in-memory interaction. It CAS-claims the ask (single-winner across
// windows/peers), closes the card everywhere, and re-drives the turn detached.
// Returns false when there is no waiting durable ask with that id (the caller then
// reports the ordinary "already resolved" 409).
func (s *Server) answerDurableAsk(r *http.Request, sessionID, askID, answer, by string) bool {
	wsp := ws(r)
	if wsp == nil || wsp.DB == nil {
		return false
	}
	ask, err := wsp.DB.ClaimSessionAsk(r.Context(), askID, answer)
	if err != nil {
		return false // not a waiting durable ask, or lost the answer race
	}
	// Close the card on every window (the resolve-once broadcast).
	s.publishHub(sessionID, sessionhub.KindInteractionResolve, map[string]any{
		"id": askID, "answer": answer, "resolvedBy": by,
	}, false)
	// Re-drive detached: the answering HTTP request must not block on the turn. The
	// continuation reaches every window (including this one) via the session hub.
	go s.driveDurableAskResume(wsp, sessionID, ask, answer)
	return true
}

// driveDurableAskResume folds the answer back into the suspended turn and runs it
// to completion (or re-suspend), broadcasting live steps + the final reply on the
// session hub. Runs in its own goroutine, detached from the HTTP request.
func (s *Server) driveDurableAskResume(wsp *workspace.Workspace, sessionID string, ask db.SessionAsk, answer string) {
	ctx := context.Background()
	onStep := func(st agent.TurnStep) {
		// Mirror the resumed turn's live activity onto the hub so every window renders
		// it — the same routing runChatTurn uses for a normal turn.
		wsp.Runtime.EmitSessionStep(sessionID, st)
		switch st.Kind {
		case agent.StepDelta:
			s.publishHub(sessionID, sessionhub.KindDelta, st, true)
		case agent.StepToolDelta:
			s.publishHub(sessionID, sessionhub.KindToolDelta, st, true)
		case agent.StepAsk, agent.StepTombstone, agent.StepPermission, agent.StepPlan:
			// Interactive prompts ride the interaction CAS, not plain hub steps.
		default:
			s.publishHub(sessionID, sessionhub.KindStep, st, false)
		}
	}
	msg, reSuspend, err := wsp.Runtime.ResumeAskAndRecord(ctx, ask, answer, onStep)
	if err != nil {
		s.recordQueueTurnFailure(sessionID, "ask_resume_error", "Durable ask resume failed: "+err.Error())
		s.hub.Commit(sessionID)
		return
	}
	if reSuspend != nil {
		// The resumed turn asked again at a clean point → a fresh durable card.
		s.openDurableAskCard(sessionID, *reSuspend, false)
		s.hub.Commit(sessionID)
		return
	}
	s.publishHub(sessionID, sessionhub.KindReply, *msg, false)
	s.hub.Commit(sessionID)
	// GC the resolved ask row (its snapshot is spent).
	_ = wsp.DB.DeleteSessionAsk(ctx, ask.ID)
}

// restoreWaitingAsks re-publishes every still-waiting durable ask card for a
// session onto the hub. Called when a window subscribes to the session stream, so
// a card survives a backend restart (which clears the in-memory hub ring): the
// durable ask row on disk is the source of truth, re-rendered on reconnect.
func (s *Server) restoreWaitingAsks(wsp *workspace.Workspace, sessionID string) {
	if wsp == nil || wsp.DB == nil {
		return
	}
	asks, err := wsp.DB.ListWaitingSessionAsks(context.Background())
	if err != nil {
		return
	}
	for _, a := range asks {
		if a.SessionID == sessionID {
			s.openDurableAskCard(sessionID, a, true)
		}
	}
}

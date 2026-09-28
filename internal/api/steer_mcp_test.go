package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/tools"
)

func TestSteerCLIMCPDeliversFIFOWithoutQueueing(t *testing.T) {
	for _, provider := range []string{"claude-cli", "codex-cli"} {
		t.Run(provider, func(t *testing.T) {
			s, wsp, run, _ := asyncAskFixture(t)
			run.setProvider(provider)
			run.setSteerable(true)
			b := &interactionBackend{runs: s.runs, apiSrv: s, tun: s.tun}
			baseline, err := b.Call(context.Background(), run.token, "active_tools", json.RawMessage(`{}`))
			if err != nil || baseline.IsError {
				t.Fatalf("baseline=%+v err=%v", baseline, err)
			}
			messages := []string{"Focus on the login bug", "Preserve the existing API"}
			for _, message := range messages {
				rec := postSteer(t, s, wsp, run.sessionID, message)
				if rec.Code != http.StatusOK || steerResult(t, rec) != "steered" {
					t.Fatalf("steer=%d %s", rec.Code, rec.Body.String())
				}
			}
			if got := queuedMessages(s, wsp.ID, run.sessionID); len(got) != 0 {
				t.Fatalf("live guidance queued as a new turn: %v", got)
			}
			res, err := b.Call(context.Background(), run.token, "active_tools", json.RawMessage(`{}`))
			if err != nil || res.IsError || res.Text != baseline.Text || len(res.UserInput) != 2 {
				t.Fatalf("MCP delivery=%+v err=%v", res, err)
			}
			for i, message := range messages {
				if res.UserInput[i] != steerInjectPreamble+message {
					t.Fatalf("message %d=%q", i, res.UserInput[i])
				}
			}
			res, err = b.Call(context.Background(), run.token, "active_tools", json.RawMessage(`{}`))
			if err != nil || len(res.UserInput) != 0 {
				t.Fatalf("duplicate delivery=%+v err=%v", res, err)
			}
			s.recoverUndeliveredSteer(run, wsp.ID, chatReq{SessionID: run.sessionID})
			if got := queuedMessages(s, wsp.ID, run.sessionID); len(got) != 0 {
				t.Fatalf("delivered guidance recovered again: %v", got)
			}
		})
	}
}

func TestSteerMCPAndAsyncAnswerShareBoundary(t *testing.T) {
	s, wsp, run, input := asyncAskFixture(t)
	run.setProvider("codex-cli")
	run.setSteerable(true)
	id, err := input.Ask(context.Background(), []tools.AskQuestion{{Question: "Audience?"}})
	if err != nil {
		t.Fatal(err)
	}
	if !answerAsyncTest(s, wsp, run.sessionID, id, "Engineers") {
		t.Fatal("answer rejected")
	}
	deliverSteer(run, "Keep the report brief")
	b := &interactionBackend{runs: s.runs, apiSrv: s, tun: s.tun}
	res, err := b.Call(context.Background(), run.token, "active_tools", json.RawMessage(`{}`))
	if err != nil || len(res.UserInput) != 2 || !strings.Contains(res.UserInput[0], "Keep the report brief") || !strings.Contains(res.UserInput[1], "Engineers") {
		t.Fatalf("delivery=%+v err=%v", res, err)
	}
	s.finishAsyncAnswers(run, wsp, run.sessionID)
	s.recoverUndeliveredSteer(run, wsp.ID, chatReq{SessionID: run.sessionID})
	if got := queuedMessages(s, wsp.ID, run.sessionID); len(got) != 0 {
		t.Fatalf("live input duplicated into queue: %v", got)
	}
}

func TestSteerCLIFullBufferPreservesEarlierMessages(t *testing.T) {
	s, wsp, run, _ := asyncAskFixture(t)
	run.setProvider("codex-cli")
	run.setSteerable(true)
	for range cap(run.steer) {
		if deliverSteer(run, "earlier guidance") != steerStashed {
			t.Fatal("buffer filled early")
		}
	}
	rec := postSteer(t, s, wsp, run.sessionID, "must stay in composer")
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("full buffer=%d %s", rec.Code, rec.Body.String())
	}
	for _, message := range run.takeCLISteer() {
		if !strings.HasSuffix(message, "earlier guidance") {
			t.Fatalf("earlier message overwritten: %q", message)
		}
	}
}

func TestSteerMCPDoesNotConsumeCancelledOrNativeInput(t *testing.T) {
	for _, provider := range []string{"anthropic", "codex-cli"} {
		t.Run(provider, func(t *testing.T) {
			s, _, run, _ := asyncAskFixture(t)
			run.setProvider(provider)
			run.setSteerable(true)
			deliverSteer(run, "keep this guidance")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if provider == "codex-cli" {
				cancel()
			}
			b := &interactionBackend{runs: s.runs, apiSrv: s, tun: s.tun}
			res, _ := b.Call(ctx, run.token, "active_tools", json.RawMessage(`{}`))
			if len(res.UserInput) != 0 || len(run.steer) != 1 {
				t.Fatalf("guidance consumed by wrong boundary: %+v, pending=%d", res, len(run.steer))
			}
		})
	}
}

func TestSteerFinalizationRaceKeepsEveryAcceptedMessage(t *testing.T) {
	for _, timing := range []string{"before", "after", "concurrent"} {
		t.Run(timing, func(t *testing.T) {
			s, wsp, run, _ := asyncAskFixture(t)
			run.setProvider("codex-cli")
			run.setSteerable(true)
			var outcome steerOutcome
			send := func() { outcome = deliverSteer(run, "late guidance") }
			finish := func() { s.recoverUndeliveredSteer(run, wsp.ID, chatReq{SessionID: run.sessionID}) }
			switch timing {
			case "before":
				send()
				finish()
			case "after":
				finish()
				send()
			default:
				var wg sync.WaitGroup
				wg.Go(send)
				wg.Go(finish)
				wg.Wait()
			}
			queued := queuedMessages(s, wsp.ID, run.sessionID)
			switch outcome {
			case steerStashed:
				if len(queued) != 1 || queued[0] != "late guidance" {
					t.Fatalf("accepted message lost: %v", queued)
				}
			case steerFinished:
				if len(queued) != 0 {
					t.Fatalf("rejected message queued: %v", queued)
				}
			default:
				t.Fatalf("unexpected result %v", outcome)
			}
			if len(run.steer) != 0 || run.steerableFor() {
				t.Fatal("finalized run still accepts guidance")
			}
		})
	}
}

func TestSteerStopDoesNotRestartTurn(t *testing.T) {
	s, wsp, run, _ := asyncAskFixture(t)
	run.setProvider("codex-cli")
	run.setSteerable(true)
	deliverSteer(run, "pending guidance")
	s.stopSessionTurn(wsp, run.sessionID)
	s.recoverUndeliveredSteer(run, wsp.ID, chatReq{SessionID: run.sessionID})
	if got := queuedMessages(s, wsp.ID, run.sessionID); len(got) != 0 {
		t.Fatalf("stop restarted session: %v", got)
	}
	rec := postSteer(t, s, wsp, run.sessionID, "late guidance")
	if rec.Code != http.StatusConflict {
		t.Fatalf("stopped run accepted guidance: %d %s", rec.Code, rec.Body.String())
	}
}

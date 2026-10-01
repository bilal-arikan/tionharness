package decider

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDecisionDebugFallbackRetriesOutcomeAndLocation(t *testing.T) {
	primary, fallback, challenger := newDecisionServer(t), newDecisionServer(t), newDecisionServer(t)
	primary.status.Store(http.StatusServiceUnavailable)
	h, _ := newTestHub(t, primary)
	ownModel(t, h, "backup", fallback.URL+"/v1")
	ownModel(t, h, "rival", challenger.URL+"/v1")
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOn; ac.Fallback = "backup"; ac.Challenger = "rival" })
	var wg sync.WaitGroup
	resp, err := h.Decide(context.Background(), testGate, yesNo(), WithRef("RUN9"), WithLocation("SES7", "MSG3"),
		WithBackground(func(f func()) { wg.Go(f) }))
	if err != nil || !resp.Fallback || resp.DebugID == "" {
		t.Fatalf("response = %+v, %v", resp, err)
	}
	wg.Wait()
	rec := NewRecord(testGate, ModeOn, resp, nil)
	rec.Outcome, rec.Applied = "yes", true
	h.Log(rec)
	report := h.Debug(DebugFilter{Ref: "SES7", Limit: 5000})
	if report.Summary.Decisions != 1 || report.Summary.Retries != 1 || report.Summary.Fallbacks != 1 || report.Summary.Challengers != 1 || report.Summary.Applied != 1 {
		t.Fatalf("summary = %+v", report.Summary)
	}
	roles := map[string]int{}
	for _, e := range report.Events {
		if e.TraceID != resp.DebugID || e.SessionID != "SES7" || e.TurnID != "MSG3" || e.Ref != "RUN9" || e.ConfigHash == "" {
			t.Fatalf("lost correlation: %+v", e)
		}
		if e.Stage == "attempt" {
			roles[e.Role]++
		}
	}
	if roles["primary"] != 1 || roles["fallback"] != 1 || roles["challenger"] != 1 {
		t.Fatalf("attempt roles = %v", roles)
	}
	filtered := h.Debug(DebugFilter{Instance: "backup"})
	if filtered.Summary.Decisions != 1 || filtered.Summary.Attempts != 1 {
		t.Fatalf("model filter = %+v", filtered.Summary)
	}
}

func TestDecisionDebugOffCancellationAndBusyChallenger(t *testing.T) {
	srv := newDecisionServer(t)
	h, _ := newTestHub(t, srv)
	cfg := h.Config()
	cfg.Enabled = false
	_, _ = h.Update(cfg)
	_, err := h.Decide(context.Background(), testGate, yesNo())
	if !errors.Is(err, ErrDisabled) || debugID(nil, err) == "" || srv.calls.Load() != 0 {
		t.Fatalf("off = %v calls=%d", err, srv.calls.Load())
	}
	report := h.Debug(DebugFilter{})
	if report.Summary.Skipped != 1 || report.Summary.Decisions != 0 {
		t.Fatalf("off report = %+v", report.Summary)
	}
	cfg.Enabled = true
	_, _ = h.Update(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = h.Decide(ctx, testGate, yesNo())
	if !errors.Is(err, context.Canceled) || errorClass(err) != "cancelled" {
		t.Fatalf("cancellation = %v", err)
	}
	ownModel(t, h, "rival", srv.URL+"/v1")
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Challenger = "rival" })
	for range cap(h.challengeSlots) {
		h.challengeSlots <- struct{}{}
	}
	_, err = h.Decide(context.Background(), testGate, yesNo(), WithBackground(func(f func()) { f() }))
	if err != nil {
		t.Fatal(err)
	}
	for range cap(h.challengeSlots) {
		<-h.challengeSlots
	}
	if got := h.Debug(DebugFilter{}).Summary.Skipped; got != 2 {
		t.Fatalf("skipped = %d", got)
	}
}

func TestDecisionDebugMetadataPrivacyAndDetachedAnswers(t *testing.T) {
	srv := newDecisionServer(t)
	h, _ := newTestHub(t, srv)
	req := yesNo()
	req.State = "private conversation body password=never-store-this"
	req.Questions["q"] = Noul("private question body", "private criterion", "other private criterion")
	resp, err := h.Decide(context.Background(), testGate, req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Answers["q"] = Answer{Type: QuestionNoul, Probability: 0}
	report := h.Debug(DebugFilter{})
	data, _ := json.Marshal(report)
	for _, text := range []string{"private conversation body", "never-store-this", "private question body", "private criterion", "key-PRV1"} {
		if strings.Contains(string(data), text) {
			t.Fatalf("debug leaked %q", text)
		}
	}
	for _, e := range report.Events {
		if e.Stage == "attempt" && e.Answers["q"].Probability != 0.9 {
			t.Fatal("caller mutated debug answers")
		}
	}
	for i := range report.Events {
		if report.Events[i].Answers != nil {
			report.Events[i].Answers["q"] = Answer{}
		}
	}
	for _, e := range h.Debug(DebugFilter{}).Events {
		if e.Stage == "attempt" && e.Answers["q"].Probability != 0.9 {
			t.Fatal("reader mutated journal")
		}
	}
}

func TestDecisionDebugRotationReloadAndFiltering(t *testing.T) {
	dir := t.TempDir()
	h := NewHub(HubOptions{DataDir: dir})
	h.debug.maxBytes = 300
	h.debug.capacity = 8
	for i := range 20 {
		h.debug.append(DebugEvent{At: int64(i + 1), TraceID: "trace", Authority: testGate, Stage: "transport", HTTPAttempt: 1})
	}
	for _, name := range []string{"debug.jsonl", "debug.1.jsonl"} {
		info, err := os.Stat(filepath.Join(dir, "decider", name))
		if err != nil || info.Size() > 500 {
			t.Fatalf("rotation %s: %v %v", name, info, err)
		}
	}
	reloaded := NewHub(HubOptions{DataDir: dir})
	report := reloaded.Debug(DebugFilter{Limit: 1, Since: time.UnixMilli(19)})
	if len(report.Events) != 1 || report.Events[0].At != 20 || report.MatchedEvents != 2 || !report.Truncated {
		t.Fatalf("report = %+v", report)
	}
	if got := reloaded.Debug(DebugFilter{Ref: "absent"}); got.MatchedEvents != 0 {
		t.Fatal("reference filter ignored")
	}
}

package decider

import (
	"context"
	"sync"
	"testing"
)

func TestDecisionDebugSeparatesRepeatedSessionIDsByWorkspace(t *testing.T) {
	server := newDecisionServer(t)
	h, _ := newTestHub(t, server)
	opts := h.opts
	opts.DataDir = t.TempDir()
	h = NewHub(opts)
	cfg := h.Config()
	cfg.Enabled = true
	if _, err := h.Update(cfg); err != nil {
		t.Fatal(err)
	}
	ownModel(t, h, "workspace-rival", server.URL+"/v1")
	setAuthority(t, h, testGate, func(ac *AuthorityConfig) { ac.Mode = ModeOn; ac.Challenger = "workspace-rival" })
	var pending sync.WaitGroup
	for _, workspace := range []string{"WS1", "WS2"} {
		resp, err := h.Decide(context.Background(), testGate, yesNo(), WithLocation("SES1", "MSG1"), WithRef("SES1"), WithWorkspace(workspace), WithBackground(func(fn func()) { pending.Go(fn) }))
		if err != nil {
			t.Fatal(err)
		}
		rec := NewRecord(testGate, ModeOn, resp, nil)
		rec.Outcome = "yes"
		rec.Applied = true
		h.Log(rec)
	}
	pending.Wait()
	for _, workspace := range []string{"WS1", "WS2"} {
		report := h.Debug(DebugFilter{WorkspaceID: workspace, Ref: "SES1", Limit: 5000})
		if report.Summary.Decisions != 1 || report.Summary.Applied != 1 || report.Summary.Challengers != 1 {
			t.Fatalf("%s summary=%+v", workspace, report.Summary)
		}
		for _, event := range report.Events {
			if event.WorkspaceID != workspace || event.SessionID != "SES1" {
				t.Fatalf("cross-workspace event: %+v", event)
			}
		}
	}
	if got := h.Debug(DebugFilter{Ref: "SES1"}).Summary.Decisions; got != 2 {
		t.Fatalf("global decisions=%d", got)
	}
	reopened := NewHub(h.opts)
	if got := reopened.Debug(DebugFilter{WorkspaceID: "WS2", Ref: "SES1"}).Summary.Decisions; got != 1 {
		t.Fatalf("persisted workspace scope lost: %d", got)
	}
}

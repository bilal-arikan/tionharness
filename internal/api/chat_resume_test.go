package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

type scopedResumeTestProvider struct {
	ready bool
	can   bool
}

func (p *scopedResumeTestProvider) Name() string { return "codex-cli" }
func (p *scopedResumeTestProvider) Complete(context.Context, providers.Request) (*providers.Response, error) {
	return nil, nil
}
func (p *scopedResumeTestProvider) ResumeScopeReady(string) bool { return p.ready }
func (p *scopedResumeTestProvider) CanResumeScoped(string, string) bool {
	return p.can
}

// TestResumeGateEnabled locks the ClaudeResume ⟂ ClaudePersistentSession contract:
// persistent-session ALWAYS supersedes --resume, and the delta path is claude-cli +
// single-agent only. Regression guard for the "both settings on → neither trims"
// gotcha (see _Docs/17).
func TestResumeGateEnabled(t *testing.T) {
	cases := []struct {
		name       string
		resume     bool
		persistent bool
		agentCount int
		provider   string
		multiPart  bool
		want       bool
	}{
		{"resume only, single claude-cli", true, false, 1, "claude-cli", false, true},
		{"persistent supersedes resume", true, true, 1, "claude-cli", false, false},
		{"persistent only", false, true, 1, "claude-cli", false, false},
		{"resume off", false, false, 1, "claude-cli", false, false},
		{"multi-agent turn blocks resume", true, false, 2, "claude-cli", false, false},
		{"multi-participant session blocks resume", true, false, 1, "claude-cli", true, false},
		{"non-cli provider blocks resume", true, false, 1, "anthropic", false, false},
		// codex-cli's thread_id is stable (does not rotate), so the delta-tracking
		// gate this function guards is claude-cli-only by design — see the
		// resumeGateEnabled doc comment.
		{"codex-cli blocks resume", true, false, 1, "codex-cli", false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := resumeGateEnabled(c.resume, c.persistent, c.agentCount, c.provider, c.multiPart); got != c.want {
				t.Errorf("resumeGateEnabled(%v,%v,%d,%q,%v) = %v, want %v",
					c.resume, c.persistent, c.agentCount, c.provider, c.multiPart, got, c.want)
			}
		})
	}
}

func TestClaudeResumeDecision(t *testing.T) {
	cases := []struct {
		name       string
		enabled    bool
		cliID      string
		sentCount  int
		rawLen     int
		compacted  bool
		wantActive bool
		wantResume string
		wantDelta  int
		wantSent   int
	}{
		// Gate off → inert (full transcript, no id captured/persisted).
		{"disabled", false, "sess", 2, 5, false, false, "", 0, 0},
		// Cold start: no prior id → send full transcript, persist count for next turn.
		{"cold first turn", true, "", 0, 1, false, true, "", 0, 1},
		// Warm resume: prior id + valid boundary + unseen delta.
		{"warm with delta", true, "sess", 3, 5, false, true, "sess", 3, 5},
		// Boundary equals raw length → nothing new → cold fallback (re-capture id).
		{"no new messages", true, "sess", 5, 5, false, true, "", 0, 5},
		// Boundary past the end (history shrank after edits) → cold fallback.
		{"boundary past end", true, "sess", 9, 5, false, true, "", 0, 5},
		// Prior id but zero boundary (shouldn't happen) → cold.
		{"zero boundary", true, "sess", 0, 4, false, true, "", 0, 4},
		// A fold this turn forces a COLD start even with a valid warm boundary, so the
		// compacted tail re-baselines a fresh CLI session; sentCount stays rawLen.
		{"compacted forces cold despite warm boundary", true, "sess", 3, 5, true, true, "", 0, 5},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			plan, resumeID, delta := claudeResumeDecision(c.enabled, c.cliID, c.sentCount, c.rawLen, c.compacted)
			if plan.active != c.wantActive {
				t.Errorf("active = %v, want %v", plan.active, c.wantActive)
			}
			if resumeID != c.wantResume {
				t.Errorf("resumeID = %q, want %q", resumeID, c.wantResume)
			}
			if delta != c.wantDelta {
				t.Errorf("deltaStart = %d, want %d", delta, c.wantDelta)
			}
			if c.wantActive && plan.sentCount != c.wantSent {
				t.Errorf("sentCount = %d, want %d", plan.sentCount, c.wantSent)
			}
			// coldStart is exactly "resume applied to this turn but no warm thread
			// carried into it" — the signal the transcript's new-CLI-session divider
			// is drawn from (TSK514). Derived from the row rather than hand-listed so
			// a new case cannot forget it.
			wantCold := c.wantActive && c.wantResume == ""
			if plan.coldStart != wantCold {
				t.Errorf("coldStart = %v, want %v", plan.coldStart, wantCold)
			}
		})
	}
}

func TestPlanCodexResumeWarmDelta(t *testing.T) {
	s := &Server{}
	p := &scopedResumeTestProvider{ready: true, can: true}
	session := db.Session{
		ID:              "SES1",
		AgentID:         "AGT1",
		Participants:    []string{"AGT1"},
		CLISessionID:    "thread-1",
		CLISentMsgCount: 2,
	}
	agentRow := db.Agent{ID: "AGT1", Provider: "codex-cli", Model: "gpt-test"}
	raw := []db.Message{{Role: providers.RoleUser}, {Role: providers.RoleAssistant}, {Role: providers.RoleUser}}
	req := providers.Request{System: "stable persona", Messages: []providers.Message{{Role: providers.RoleUser, Text: "full prepared tail"}}}

	plan := s.planCodexResume(context.Background(), p, 1, session, agentRow, raw, false, &req)
	if !plan.active || plan.sentCount != len(raw) {
		t.Fatalf("plan = %+v, want active boundary %d", plan, len(raw))
	}
	if req.ResumeSessionID != "thread-1" {
		t.Fatalf("ResumeSessionID = %q", req.ResumeSessionID)
	}
	if req.CLIResumeScope == "" {
		t.Fatal("CLIResumeScope is empty")
	}
	if len(req.Messages) != 1 || req.Messages[0].Role != providers.RoleUser {
		t.Fatalf("delta messages = %+v", req.Messages)
	}
}

func TestPlanCodexResumeFoldStartsFreshFromPreparedTail(t *testing.T) {
	s := &Server{}
	p := &scopedResumeTestProvider{ready: true, can: true}
	session := db.Session{ID: "SES1", AgentID: "AGT1", CLISessionID: "thread-1", CLISentMsgCount: 2}
	agentRow := db.Agent{ID: "AGT1", Provider: "codex-cli", Model: "gpt-test"}
	raw := []db.Message{{Role: providers.RoleUser}, {Role: providers.RoleAssistant}, {Role: providers.RoleUser}}
	prepared := []providers.Message{{Role: providers.RoleUser, Text: "summary-backed recent tail"}}
	req := providers.Request{System: "stable persona", Summary: "summary", Messages: prepared}

	plan := s.planCodexResume(context.Background(), p, 1, session, agentRow, raw, true, &req)
	if !plan.active {
		t.Fatal("folded codex turn should capture a fresh thread")
	}
	if req.ResumeSessionID != "" {
		t.Fatalf("fold resumed stale thread %q", req.ResumeSessionID)
	}
	if len(req.Messages) != 1 || req.Messages[0].Text != prepared[0].Text {
		t.Fatalf("prepared summary tail was replaced: %+v", req.Messages)
	}
	if req.CLIResumeScope == "" {
		t.Fatal("fresh folded turn needs a durable scope for its new thread")
	}
}

func TestPlanCodexResumeSafetyGates(t *testing.T) {
	cases := []struct {
		name         string
		ready        bool
		agentCount   int
		sessionAgent string
		participants []string
	}{
		{"home unavailable", false, 1, "AGT1", nil},
		{"multi agent turn", true, 2, "AGT1", nil},
		{"different persona", true, 1, "AGT2", nil},
		{"multi participant session", true, 1, "AGT1", []string{"AGT1", "AGT2"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s := &Server{}
			p := &scopedResumeTestProvider{ready: tc.ready, can: true}
			session := db.Session{ID: "SES1", AgentID: tc.sessionAgent, Participants: tc.participants, CLISessionID: "thread-1", CLISentMsgCount: 1}
			agentRow := db.Agent{ID: "AGT1", Provider: "codex-cli", Model: "gpt-test"}
			raw := []db.Message{{Role: providers.RoleAssistant}, {Role: providers.RoleUser}}
			req := providers.Request{System: "persona", Messages: []providers.Message{{Role: providers.RoleUser}}}

			plan := s.planCodexResume(context.Background(), p, tc.agentCount, session, agentRow, raw, false, &req)
			if plan.active || req.ResumeSessionID != "" || req.CLIResumeScope != "" {
				t.Fatalf("unsafe resume engaged: plan=%+v req=%+v", plan, req)
			}
		})
	}
}

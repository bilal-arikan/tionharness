package api

import (
	"context"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/settings"
)

// Fixture constants for the reload test below. Every expectation is derived from
// these (and from nativeCompactBoundary), never read back out of the code under
// test.
const (
	reloadPreCompactCLIID  = "cli-before-compaction"
	reloadPostCompactCLIID = "cli-after-compaction"
	reloadPreCompactSent   = 2 // messages the CLI had seen before it compacted
	reloadHistoryLen       = 6 // transcript length at the moment the gate fires
)

// TestNativeCompactionResumeFieldsRequireASessionReload pins WHY the three turn
// paths re-read the session after prep.NativeCompacted (chat_stream.go,
// wake_turn.go, chat_btw.go): runNativeCompact rotates the session's CLI resume
// id and boundary in the store via SetSessionCLIResume, and the session value the
// turn carries in memory predates that write. Planning the CLI resume from the
// stale copy resumes a thread the CLI has replaced and re-sends the delta from the
// pre-compaction sent-count.
//
// COVERAGE CAVEAT — read before trusting this test: it does NOT execute the
// `session, err = database.GetSession(...)` statement at any of the three call
// sites. Deleting those three blocks leaves this test green. Driving them requires
// prep.NativeCompacted, which requires runNativeCompact to succeed, which requires
// a provider that is both a providers.CLINativeManualCompactor and reports
// NativeCompactionEvents() — i.e. a real claude/codex CLI binary. providers.Registry
// builds providers from its built-in kinds and offers no stub-injection seam, so no
// api-level test can reach that branch without the CLI installed. What this test
// does pin is the consumer contract the reload exists to satisfy: given the store
// state runNativeCompact leaves behind, the stale session and the reloaded session
// produce measurably different CLI resume plans.
func TestNativeCompactionResumeFieldsRequireASessionReload(t *testing.T) {
	s, wsp := newWorkspaceServer(t)
	ctx := context.Background()

	// The delta path is claude-cli + --resume; the persistent-session mode
	// supersedes it (see resumeGateEnabled), so turn it off explicitly.
	resume, persistent := true, false
	if _, err := s.settings.Apply(settings.Patch{ClaudeResume: &resume, ClaudePersistentSession: &persistent}); err != nil {
		t.Fatalf("settings: %v", err)
	}

	agentRow, err := wsp.DB.CreateAgent(ctx, db.Agent{Name: "Ada", Provider: "claude-cli"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := wsp.DB.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	// The warm thread as it stood when this turn started.
	if err := wsp.DB.SetSessionCLIResume(ctx, sess.ID, reloadPreCompactCLIID, reloadPreCompactSent); err != nil {
		t.Fatalf("seed cli resume: %v", err)
	}
	stale, err := wsp.DB.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}

	rawHistory := make([]db.Message, 0, reloadHistoryLen)
	for i := 0; i < reloadHistoryLen; i++ {
		role := providers.RoleUser
		if i%2 == 1 {
			role = providers.RoleAssistant
		}
		rawHistory = append(rawHistory, db.Message{Role: role, Text: "m"})
	}

	// Reproduce exactly what runNativeCompact persists at the end of an automatic
	// native compaction: the rotated CLI session id and the boundary the shared
	// helper computes for this mode.
	boundary := nativeCompactBoundary(nativeCompactAuto, len(rawHistory))
	if boundary != reloadHistoryLen {
		t.Fatalf("auto-mode boundary = %d, want %d (the raw transcript length)", boundary, reloadHistoryLen)
	}
	if err := wsp.DB.SetSessionCLIResume(ctx, sess.ID, reloadPostCompactCLIID, boundary); err != nil {
		t.Fatalf("persist post-compaction cli resume: %v", err)
	}
	fresh, err := wsp.DB.GetSession(ctx, sess.ID)
	if err != nil {
		t.Fatalf("reload session: %v", err)
	}
	if fresh.CLISessionID != reloadPostCompactCLIID || fresh.CLISentMsgCount != boundary {
		t.Fatalf("store did not take the post-compaction resume fields: id=%q sent=%d",
			fresh.CLISessionID, fresh.CLISentMsgCount)
	}

	provider := providers.NewClaudeCLI("claude", "", "", "", "")
	// compacted=false on purpose: a native compaction sets prep.NativeCompacted,
	// NOT prep.Compacted, so the fold-forces-cold branch of claudeResumeDecision is
	// not what protects this path — only the reloaded session is.
	plan := func(session db.Session, history []db.Message) (cliResumePlan, providers.Request) {
		req := providers.Request{Messages: []providers.Message{{Role: providers.RoleUser, Text: "prepared transcript"}}}
		p := s.planCLIResume(ctx, provider, 1, session, agentRow, history, false, &req)
		return p, req
	}

	// (1) The turn that just compacted. The CLI's window now starts at the whole
	// transcript, so there is nothing unseen: the plan must be a cold start.
	stalePlan, staleReq := plan(stale, rawHistory)
	if staleReq.ResumeSessionID != reloadPreCompactCLIID {
		t.Fatalf("stale session resume id = %q, want %q — the test's premise (a warm plan off the pre-compaction copy) does not hold",
			staleReq.ResumeSessionID, reloadPreCompactCLIID)
	}
	if got, want := len(staleReq.Messages), reloadHistoryLen-reloadPreCompactSent; got != want {
		t.Fatalf("stale session delta = %d messages, want %d", got, want)
	}
	if stalePlan.coldStart {
		t.Fatalf("stale session unexpectedly planned a cold start")
	}

	freshPlan, freshReq := plan(fresh, rawHistory)
	if freshReq.ResumeSessionID != "" {
		t.Fatalf("reloaded session resumed %q; after native compaction the whole transcript is inside the rebuilt window, so nothing is unseen",
			freshReq.ResumeSessionID)
	}
	if !freshPlan.coldStart {
		t.Fatalf("reloaded session must flag a cold start on the compaction turn")
	}
	if len(freshReq.Messages) != 1 {
		t.Fatalf("cold start rewrote the prepared messages (%d); it must leave llmReq untouched", len(freshReq.Messages))
	}

	// (2) The NEXT turn, one message later. Now the plans differ in both axes at
	// once: the stale copy resumes the thread the CLI replaced and re-sends four
	// already-summarized messages, while the reloaded copy resumes the rotated
	// thread and sends only the single unseen message.
	nextHistory := append(append([]db.Message{}, rawHistory...), db.Message{Role: providers.RoleUser, Text: "next"})

	_, staleNext := plan(stale, nextHistory)
	if staleNext.ResumeSessionID != reloadPreCompactCLIID {
		t.Fatalf("next turn from the stale session resumed %q, want the dead %q", staleNext.ResumeSessionID, reloadPreCompactCLIID)
	}
	if got, want := len(staleNext.Messages), len(nextHistory)-reloadPreCompactSent; got != want {
		t.Fatalf("next turn from the stale session re-sent %d messages, want %d", got, want)
	}

	_, freshNext := plan(fresh, nextHistory)
	if freshNext.ResumeSessionID != reloadPostCompactCLIID {
		t.Fatalf("next turn from the reloaded session resumed %q, want %q", freshNext.ResumeSessionID, reloadPostCompactCLIID)
	}
	if got, want := len(freshNext.Messages), len(nextHistory)-boundary; got != want {
		t.Fatalf("next turn from the reloaded session sent %d messages, want %d (only the unseen tail)", got, want)
	}
}

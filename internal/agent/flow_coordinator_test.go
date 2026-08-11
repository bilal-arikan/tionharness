package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/orchestration"
)

// TestRunCoordinatorNode_ReturnsFinalReply drives the coordinator node end to
// end with a stubbed coordinator turn: the node must open a coordinator-role
// session of the flow-coordinator kind, seed it with the prompt, and return the
// coordinator's final assistant reply as the node output.
func TestRunCoordinatorNode_ReturnsFinalReply(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{Name: "koordinator"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	// Stand in for a real coordinator turn: record a reply, spawn no workers.
	var ranOn string
	rt.coordRunFn = func(sessionID string) {
		ranOn = sessionID
		if _, err := rt.db.AddMessage(ctx, db.Message{
			SessionID: sessionID, AgentID: agent.ID, Role: "assistant", Text: "3 worker bitti, rapor hazır",
		}); err != nil {
			t.Errorf("record stub reply: %v", err)
		}
	}

	out, err := rt.RunCoordinatorNode(ctx, orchestration.CoordinatorSpec{AgentID: agent.ID, Prompt: "repoyu tara", MaxTurns: 5, TimeoutSec: 30})
	if err != nil {
		t.Fatalf("RunCoordinatorNode: %v", err)
	}
	if out != "3 worker bitti, rapor hazır" {
		t.Errorf("output = %q, want the coordinator's final reply", out)
	}
	if ranOn == "" {
		t.Fatal("no coordinator turn was run")
	}

	sess, err := rt.db.GetSession(ctx, ranOn)
	if err != nil {
		t.Fatalf("get coordinator session: %v", err)
	}
	// Capability, not lineage: the flow owns this session, so it is a coordinator
	// with no parent to report to (Role stays empty).
	if !sess.IsCoordinator() {
		t.Error("flow coordinator session must have coordinator mode")
	}
	if sess.Role != "" {
		t.Errorf("session role = %q, want empty (nothing above it to report to)", sess.Role)
	}
	if sess.Kind != SessionKindFlowCoordinator {
		t.Errorf("session kind = %q, want %q", sess.Kind, SessionKindFlowCoordinator)
	}
	if sess.CoordinatorMaxTurns != 5 {
		t.Errorf("maxTurns = %d, want the node's 5", sess.CoordinatorMaxTurns)
	}
	msgs, _ := rt.db.ListMessages(ctx, ranOn)
	if len(msgs) < 1 || msgs[0].Role != "user" || msgs[0].Text != "repoyu tara" {
		t.Errorf("first message should be the node prompt, got %+v", msgs)
	}
}

// TestRunCoordinatorNode_EmptyReplyFails verifies a coordinator that produces no
// assistant reply fails the node loudly instead of emitting an empty output that
// downstream {{last}} would silently carry forward.
func TestRunCoordinatorNode_EmptyReplyFails(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "sessiz"})
	rt.coordRunFn = func(string) {} // runs, records nothing

	if _, err := rt.RunCoordinatorNode(ctx, orchestration.CoordinatorSpec{AgentID: agent.ID, Prompt: "görev", TimeoutSec: 30}); err == nil {
		t.Fatal("want an error when the coordinator produced no reply")
	}
}

// TestRunCoordinatorNode_MissingAgentFails verifies an unknown agent fails before
// any session is created.
func TestRunCoordinatorNode_MissingAgentFails(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	rt.coordRunFn = func(string) {}

	if _, err := rt.RunCoordinatorNode(ctx, orchestration.CoordinatorSpec{AgentID: "AGTnope", Prompt: "görev", TimeoutSec: 30}); err == nil {
		t.Fatal("want an error for a missing coordinator agent")
	}
	sessions, _ := rt.db.ListSessions(ctx, "")
	if len(sessions) != 0 {
		t.Errorf("no session should be created for a bad agent, got %d", len(sessions))
	}
}

// writeWorkspaceRecipe drops a coordinator-workflow skill into the runtime's
// workspace skills tier. That tier is a SIBLING of the working dir, not a child
// (see workspaceSkillsDir: <workspace>/skills next to <workspace>/store).
func writeWorkspaceRecipe(t *testing.T, workDir, slug, content string) {
	t.Helper()
	dir := filepath.Join(filepath.Dir(workDir), "skills", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestRunCoordinatorNode_WorkflowPersistsAndSetsTurns verifies a selected
// coordination recipe is stamped on the session (so the turn's static prefix
// injects its body) and that its max_turns applies when the node sets none.
func TestRunCoordinatorNode_WorkflowPersistsAndSetsTurns(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	writeWorkspaceRecipe(t, workDir, "wf-fanout",
		"---\nname: Fanout\nkind: coordinator-workflow\npattern: fanout\nmax_turns: 9\n---\nbody")
	rt, _ := newTestRuntime(t, workDir)
	ctx := context.Background()
	agent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "koordinator"})
	var ranOn string
	rt.coordRunFn = func(sessionID string) {
		ranOn = sessionID
		rt.db.AddMessage(ctx, db.Message{SessionID: sessionID, AgentID: agent.ID, Role: "assistant", Text: "rapor"})
	}

	out, err := rt.RunCoordinatorNode(ctx, orchestration.CoordinatorSpec{
		AgentID: agent.ID, Prompt: "görev", Workflow: "wf-fanout",
	})
	if err != nil {
		t.Fatalf("RunCoordinatorNode: %v", err)
	}
	if out != "rapor" {
		t.Errorf("output = %q, want rapor", out)
	}
	sess, err := rt.db.GetSession(ctx, ranOn)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.CoordinatorWorkflow != "wf-fanout" {
		t.Errorf("workflow = %q, want wf-fanout", sess.CoordinatorWorkflow)
	}
	if sess.CoordinatorMaxTurns != 9 {
		t.Errorf("maxTurns = %d, want the recipe's 9", sess.CoordinatorMaxTurns)
	}
}

// TestRunCoordinatorNode_NodeMaxTurnsBeatsRecipe verifies an explicit node cap
// overrides the recipe's own max_turns.
func TestRunCoordinatorNode_NodeMaxTurnsBeatsRecipe(t *testing.T) {
	workDir := filepath.Join(t.TempDir(), "workspace")
	writeWorkspaceRecipe(t, workDir, "wf-fanout",
		"---\nname: Fanout\nkind: coordinator-workflow\npattern: fanout\nmax_turns: 9\n---\nbody")
	rt, _ := newTestRuntime(t, workDir)
	ctx := context.Background()
	agent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "koordinator"})
	var ranOn string
	rt.coordRunFn = func(sessionID string) {
		ranOn = sessionID
		rt.db.AddMessage(ctx, db.Message{SessionID: sessionID, AgentID: agent.ID, Role: "assistant", Text: "rapor"})
	}

	if _, err := rt.RunCoordinatorNode(ctx, orchestration.CoordinatorSpec{
		AgentID: agent.ID, Prompt: "görev", Workflow: "wf-fanout", MaxTurns: 3,
	}); err != nil {
		t.Fatalf("RunCoordinatorNode: %v", err)
	}
	sess, _ := rt.db.GetSession(ctx, ranOn)
	if sess.CoordinatorMaxTurns != 3 {
		t.Errorf("maxTurns = %d, want the node's 3", sess.CoordinatorMaxTurns)
	}
}

// TestRunCoordinatorNode_BadWorkflowFails verifies an unknown recipe slug fails
// the node (no silent fallback to free coordination) before a session exists.
func TestRunCoordinatorNode_BadWorkflowFails(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()
	agent, _ := rt.db.CreateAgent(ctx, db.Agent{Name: "koordinator"})
	rt.coordRunFn = func(string) {}

	if _, err := rt.RunCoordinatorNode(ctx, orchestration.CoordinatorSpec{
		AgentID: agent.ID, Prompt: "görev", Workflow: "yok-boyle-bir-sey",
	}); err == nil {
		t.Fatal("want an error for an unknown coordination recipe")
	}
	sessions, _ := rt.db.ListSessions(ctx, "")
	if len(sessions) != 0 {
		t.Errorf("no session should be created for a bad workflow, got %d", len(sessions))
	}
}

// TestWaitCoordinatorIdle_TimesOut verifies the settle wait gives up (rather than
// blocking the flow forever) when the coordinator never goes quiet.
func TestWaitCoordinatorIdle_TimesOut(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	slot := rt.coordSlotFor("SESbusy")
	slot.workers.Add(1) // a worker that never finishes

	start := time.Now()
	err := rt.waitCoordinatorIdle(context.Background(), "SESbusy", 1)
	if err == nil {
		t.Fatal("want a timeout error while a worker is still active")
	}
	if !strings.Contains(err.Error(), "settle") {
		t.Errorf("error = %v, want it to mention settling", err)
	}
	if elapsed := time.Since(start); elapsed < time.Second {
		t.Errorf("returned after %s, want it to honour the 1s deadline", elapsed)
	}
}

// TestCoordSlotIdle covers the quiescence predicate the settle wait polls. It spans
// BOTH halves now: the coordinator's own policy state (drain loop / pending
// notification / live workers) AND the session's admission queue, so a node that is
// busy with a turn from any other path (a user message, a peer delivery) never
// reads as settled.
func TestCoordSlotIdle(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	slot := rt.coordSlotFor("COORD")
	if !rt.coordSlotIdle("COORD", slot) {
		t.Error("a fresh slot should read idle")
	}
	slot.driving = true
	if rt.coordSlotIdle("COORD", slot) {
		t.Error("an active drain loop must not read idle")
	}
	slot.driving = false
	slot.pending = true
	if rt.coordSlotIdle("COORD", slot) {
		t.Error("a pending notification must not read idle")
	}
	slot.pending = false
	slot.workers.Add(1)
	if rt.coordSlotIdle("COORD", slot) {
		t.Error("an active worker must not read idle")
	}
	slot.workers.Add(-1)
	release := rt.BeginSessionUserTurn("COORD")
	if rt.coordSlotIdle("COORD", slot) {
		t.Error("a turn holding the admission slot must not read idle")
	}
	release()
	if !rt.coordSlotIdle("COORD", slot) {
		t.Error("slot should read idle once everything is quiet")
	}
}

// TestRecoverSkipsFlowCoordinator verifies boot recovery leaves a flow's
// coordinator session alone: the owning flow run re-executes the node with a
// FRESH coordinator, so re-enqueueing a turn here would duplicate the work. Its
// orphaned worker is still reclaimed, just without notifying the dead coordinator.
func TestRecoverSkipsFlowCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	enqueued := 0
	rt.coordRunFn = func(string) { enqueued++ }
	ctx := context.Background()

	coord, _ := rt.db.CreateSession(ctx, db.Session{
		AgentID: "AGT1", Kind: SessionKindFlowCoordinator, Role: "coordinator", SourceID: "s:fc",
	})
	// Orphaned mid-turn: trailing user message, no reply.
	rt.db.AddMessage(ctx, db.Message{SessionID: coord.ID, Role: "user", Text: "node prompt"})
	worker, _ := rt.db.CreateSession(ctx, db.Session{
		AgentID: "AGT1", Kind: "worker", Role: "worker", SourceID: "s:fw", CoordinatorSessionID: coord.ID,
	})
	rt.db.AddMessage(ctx, db.Message{SessionID: worker.ID, Role: "user", Text: "task"})

	rt.RecoverOrphanedTurns(ctx)

	// The worker is reclaimed with an interrupted reply…
	wm, _ := rt.db.ListMessages(ctx, worker.ID)
	if len(wm) != 2 || wm[1].Role != "assistant" || !wm[1].Interrupted {
		t.Fatalf("worker should end with an interrupted reply, got %+v", wm)
	}
	// …but the abandoned flow coordinator gets neither a notification nor a turn.
	cm, _ := rt.db.ListMessages(ctx, coord.ID)
	if len(cm) != 1 {
		t.Fatalf("flow coordinator must not receive a notification, got %+v", cm)
	}
	if enqueued != 0 {
		t.Errorf("flow coordinator must not be re-enqueued, ran %d turns", enqueued)
	}
}

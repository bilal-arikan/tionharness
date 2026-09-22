package agent

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/tools"
)

// TestSpawnToolSessionIsAChat (TSK1005): a session an agent opens with the
// spawn_session tool is an independent conversation, so it is created as a plain
// chat — kind/category "chat", no subagent or worker classification — while its
// lineage still names the session that spawned it.
func TestSpawnToolSessionIsAChat(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	caller, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Lead", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	target, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Helper", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create target: %v", err)
	}
	parent, err := rt.db.CreateSession(ctx, db.Session{AgentID: caller.ID, Kind: "chat", Title: "lead"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	res, err := rt.SpawnSession(ctx, target.ID, "Do the thing", SpawnToolOptions(caller.ID, parent.ID, "", ""))
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	sess, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	if sess.Kind != SessionKindChat || sess.Category != db.CategoryChat || sess.ExecutionType != db.ExecutionInteractive {
		t.Errorf("classification = kind %q / category %q / executionType %q, want chat/chat/interactive",
			sess.Kind, sess.Category, sess.ExecutionType)
	}
	if sess.IsWorker() || sess.CoordinatorSessionID != "" {
		t.Errorf("spawn_session child must not be a worker: role=%q coordinator=%q", sess.Role, sess.CoordinatorSessionID)
	}
	o := sess.Lineage()
	if o.Kind != db.OriginSpawn || o.TriggerSessionID != parent.ID {
		t.Errorf("lineage = %+v, want spawn triggered by %s", o, parent.ID)
	}
	if sess.RootSession() != sess.ID {
		t.Errorf("spawn_session child must be its own root, got %q", sess.RootSession())
	}
	drainSpawns(t, rt)
}

// TestSpawnToolOptionsWithoutCallerSession: the CLI bridge may not know the
// caller's session; the spawn is then still a spawn-origin chat, just without a
// trigger (never a self-reference or an invented id).
func TestSpawnToolOptionsWithoutCallerSession(t *testing.T) {
	opts := SpawnToolOptions("AGT1", "  ", "model-x", "/work")
	if opts.Kind != SessionKindChat || opts.CreatedBy != "AGT1" || opts.ModelOverride != "model-x" || opts.WorkingDir != "/work" {
		t.Fatalf("options = %+v", opts)
	}
	if opts.Origin == nil || opts.Origin.Kind != db.OriginSpawn || opts.Origin.TriggerSessionID != "" {
		t.Fatalf("origin = %+v, want spawn with no trigger", opts.Origin)
	}
	if opts.CoordinatorSessionID != "" || opts.ChildSession != nil {
		t.Fatalf("spawn_session options must not describe a worker or a child run: %+v", opts)
	}
}

// TestRecoverOrphanedSpawnToolChat: moving spawn_session children to kind "chat"
// must not drop them from boot recovery. A spawn-origin chat killed mid-turn is
// still reclaimed, while a user-started chat with a trailing prompt is not.
func TestRecoverOrphanedSpawnToolChat(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	spawned, err := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: SessionKindChat, SourceID: "spawn:x",
		Origin: &db.SessionOrigin{Kind: db.OriginSpawn, TriggerSessionID: "SES1"}})
	if err != nil {
		t.Fatalf("create spawned chat: %v", err)
	}
	userChat, err := rt.db.CreateSession(ctx, db.Session{AgentID: "AGT1", Kind: SessionKindChat, SourceID: "s:user"})
	if err != nil {
		t.Fatalf("create user chat: %v", err)
	}
	for _, id := range []string{spawned.ID, userChat.ID} {
		if _, err := rt.db.AddMessage(ctx, db.Message{SessionID: id, Role: "user", Text: "do the thing"}); err != nil {
			t.Fatalf("add prompt: %v", err)
		}
	}

	rt.RecoverOrphanedTurns(ctx)

	sm, _ := rt.db.ListMessages(ctx, spawned.ID)
	if len(sm) != 2 || sm[1].Role != "assistant" || !sm[1].Interrupted {
		t.Fatalf("spawn-origin chat should end with an interrupted reply, got %+v", sm)
	}
	um, _ := rt.db.ListMessages(ctx, userChat.ID)
	if len(um) != 1 {
		t.Fatalf("user chat must be left alone, got %d msgs", len(um))
	}
}

// TestDelegatedRunsKeepTheirLabels is the other half of TSK1005: real delegations
// are unaffected — a run_subagent child stays in the subagent category and a
// spawn_worker child stays a worker.
func TestDelegatedRunsKeepTheirLabels(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	ctx := context.Background()

	lead, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Lead", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create lead: %v", err)
	}
	parent, err := rt.db.CreateSession(ctx, db.Session{AgentID: lead.ID, Kind: "chat", Title: "lead"})
	if err != nil {
		t.Fatalf("create parent: %v", err)
	}

	meta := subagentSessionMeta(parent.ID, parent.Title, lead, false, tools.RunAgentSpec{Target: lead.Name, Task: "look"})
	child, err := rt.db.CreateChildSession(ctx, meta)
	if err != nil {
		t.Fatalf("create subagent child: %v", err)
	}
	if child.Category != db.CategorySubagent || child.ExecutionType != db.ExecutionSubagent {
		t.Errorf("run_subagent child = category %q / executionType %q, want subagent/subagent", child.Category, child.ExecutionType)
	}
	if o := child.Lineage(); o.Kind != db.OriginSubagent {
		t.Errorf("run_subagent child lineage = %q, want subagent", o.Kind)
	}

	res, err := rt.SpawnSession(ctx, lead.ID, "work", SpawnOptions{
		CreatedBy:            lead.ID,
		CoordinatorSessionID: parent.ID,
		Role:                 db.SessionRoleWorker,
	})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	worker, err := rt.db.GetSession(ctx, res.SessionID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if worker.Kind != "worker" || worker.Category != db.CategoryWorker || !worker.IsWorker() {
		t.Errorf("spawn_worker child = kind %q / category %q / worker %v, want worker/worker/true",
			worker.Kind, worker.Category, worker.IsWorker())
	}
	if o := worker.Lineage(); o.Kind != db.OriginCoordinator || o.TriggerSessionID != parent.ID {
		t.Errorf("worker lineage = %+v, want coordinator triggered by %s", o, parent.ID)
	}
	drainSpawns(t, rt)
}

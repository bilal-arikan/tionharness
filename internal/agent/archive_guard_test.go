package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/archive"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/skills"
)

// archivedAgent stores an agent and archives it.
func archivedAgent(t *testing.T, database *db.DB, name string) db.Agent {
	t.Helper()
	ctx := context.Background()
	a, err := database.CreateAgent(ctx, db.Agent{Name: name})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	a, err = database.SetAgentArchived(ctx, a.ID, true)
	if err != nil {
		t.Fatalf("archive agent: %v", err)
	}
	return a
}

// An archived agent is refused by id and by name with the explicit archive
// error — never resolved as if it were runnable.
func TestResolveAgentRefusesArchived(t *testing.T) {
	r := lifecycleRuntime(t)
	a := archivedAgent(t, r.db, "Shelved")

	for _, ref := range []string{a.ID, "Shelved", "@shelved"} {
		if _, err := r.resolveAgent(context.Background(), ref); !errors.Is(err, archive.ErrArchived) {
			t.Fatalf("resolveAgent(%q) err = %v, want ErrArchived", ref, err)
		}
	}
}

// A live agent wins over an archived namesake, so archiving an old copy does
// not block the name.
func TestResolveAgentPrefersLiveNamesake(t *testing.T) {
	r := lifecycleRuntime(t)
	archivedAgent(t, r.db, "Twin")
	live, err := r.db.CreateAgent(context.Background(), db.Agent{Name: "Twin"})
	if err != nil {
		t.Fatal(err)
	}
	got, err := r.resolveAgent(context.Background(), "Twin")
	if err != nil || got.ID != live.ID {
		t.Fatalf("resolveAgent = %s, %v; want live %s", got.ID, err, live.ID)
	}
}

// run_subagent must surface the archive error instead of falling through to a
// same-named built-in profile or an "unknown target" message.
func TestSubagentTargetArchivedIsExplicit(t *testing.T) {
	r := lifecycleRuntime(t)
	archivedAgent(t, r.db, "Helper")
	if _, _, err := r.resolveSubagentTarget(context.Background(), db.Agent{}, "Helper"); !errors.Is(err, archive.ErrArchived) {
		t.Fatalf("err = %v, want ErrArchived", err)
	}
}

// The tool-loop funnel is the backstop behind every entry point: a turn for an
// archived agent fails before any provider is touched.
func TestToolLoopRefusesArchivedAgent(t *testing.T) {
	r := lifecycleRuntime(t)
	a := archivedAgent(t, r.db, "Idle")
	_, err := r.CompleteWithTools(context.Background(), a, nil, providers.Request{}, false)
	if !errors.Is(err, archive.ErrArchived) {
		t.Fatalf("err = %v, want ErrArchived", err)
	}
}

// An automation whose target agent is archived is declined with its own
// ledger reason instead of spawning a turn that would fail.
func TestAutomationSkipsArchivedTargetAgent(t *testing.T) {
	e := backstopEngine(t)
	ctx := context.Background()
	a := archivedAgent(t, e.db, "Target")
	auto := seedAutomation(t, e, db.Automation{TargetAgentID: a.ID, MaxIterations: 3})
	if got := e.guardReason(ctx, auto); got != db.AutomationSkipAgentArchived {
		t.Fatalf("guardReason = %q, want %q", got, db.AutomationSkipAgentArchived)
	}

	// The no-LLM board actions run no agent and stay unaffected.
	board := seedAutomation(t, e, db.Automation{TriggerKind: db.TriggerBoard, BoardAction: db.BoardActionArchive, TargetAgentID: a.ID, MaxIterations: 3})
	if got := e.guardReason(ctx, board); got != "" {
		t.Fatalf("board archive action guardReason = %q, want pass", got)
	}
}

// An archived automation keeps the explicit "archived" skip reason.
func TestAutomationArchivedRuleSkips(t *testing.T) {
	e := backstopEngine(t)
	auto := seedAutomation(t, e, db.Automation{MaxIterations: 3})
	if err := e.db.SetAutomationArchived(context.Background(), auto.ID, true); err != nil {
		t.Fatal(err)
	}
	got, _ := e.db.GetAutomation(context.Background(), auto.ID)
	if reason := e.guardReason(context.Background(), got); reason != db.AutomationSkipArchived {
		t.Fatalf("guardReason = %q, want %q", reason, db.AutomationSkipArchived)
	}
}

// An agent that still has an archived skill assigned learns why it cannot load
// it, and the skill's tool grants are withheld.
func TestAgentSkillLibRefusesArchivedSkill(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: Old\ndescription: d\naccess: shared\nalways_allow: Bash\narchived: true\n---\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "old", "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	store := skills.New(dir, "")
	lib := agentSkillLib{store: store, allow: store.AllowedFor([]string{"old"})}
	if _, err := lib.Body("old"); !errors.Is(err, archive.ErrArchived) {
		t.Fatalf("Body err = %v, want ErrArchived", err)
	}
	if got := lib.AllowedTools("old"); got != nil {
		t.Fatalf("AllowedTools = %v, want none for an archived skill", got)
	}
}

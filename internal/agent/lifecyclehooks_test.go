package agent

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
)

func lifecycleRuntime(t *testing.T) *Runtime {
	t.Helper()
	database, err := db.Open(filepath.Join(t.TempDir(), "store"))
	if err != nil {
		t.Fatal(err)
	}
	return &Runtime{
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
		workDir: t.TempDir(),
		db:      database,
	}
}

// TestRunLifecycleHooks_InjectsContext verifies a SessionStart hook's
// additionalContext is captured and returned for folding into the turn.
func TestRunLifecycleHooks_InjectsContext(t *testing.T) {
	r := lifecycleRuntime(t)
	ctx := context.Background()
	if _, err := r.db.CreateHook(ctx, db.Hook{
		Event:      db.HookSessionStart,
		Type:       "command",
		Command:    emitCmd(`{"additionalContext":"BE CAVEMAN"}`),
		TimeoutSec: 10,
		Enabled:    true,
	}); err != nil {
		t.Fatal(err)
	}
	out := r.RunLifecycleHooks(ctx, "SES1", db.HookSessionStart, LifecycleExtras{Source: "startup"})
	if !strings.Contains(out.Context, "BE CAVEMAN") {
		t.Fatalf("want injected context, got %q", out.Context)
	}
	if out.Block {
		t.Fatal("SessionStart context hook should not block")
	}
	if len(out.Steps) == 0 {
		t.Fatal("expected an audit step for the context injection")
	}
}

// TestRunLifecycleHooks_Blocks verifies a UserPromptSubmit hook that exits 2
// blocks the turn with its stderr reason.
func TestRunLifecycleHooks_Blocks(t *testing.T) {
	r := lifecycleRuntime(t)
	ctx := context.Background()
	if _, err := r.db.CreateHook(ctx, db.Hook{
		Event:      db.HookUserPromptSubmit,
		Type:       "command",
		Command:    blockCmd(),
		TimeoutSec: 10,
		Enabled:    true,
	}); err != nil {
		t.Fatal(err)
	}
	out := r.RunLifecycleHooks(ctx, "SES1", db.HookUserPromptSubmit, LifecycleExtras{Prompt: "hi"})
	if !out.Block {
		t.Fatalf("expected block, got %+v", out)
	}
}

// TestRunLifecycleHooks_PlainStdoutContext verifies Claude Code parity: a
// SessionStart hook's plain (non-JSON) stdout is injected as context (many real
// hooks, e.g. caveman's activate script, print plain text). A Stop hook's plain
// stdout is NOT injected (only the two context events treat stdout this way).
func TestRunLifecycleHooks_PlainStdoutContext(t *testing.T) {
	r := lifecycleRuntime(t)
	ctx := context.Background()
	if _, err := r.db.CreateHook(ctx, db.Hook{
		Event: db.HookSessionStart, Type: "command",
		Command: emitCmd("BE CAVEMAN (plain text)"), TimeoutSec: 10, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.db.CreateHook(ctx, db.Hook{
		Event: db.HookStop, Type: "command",
		Command: emitCmd("this stop text must NOT inject"), TimeoutSec: 10, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if out := r.RunLifecycleHooks(ctx, "SES1", db.HookSessionStart, LifecycleExtras{Source: "startup"}); !strings.Contains(out.Context, "BE CAVEMAN") {
		t.Fatalf("SessionStart plain stdout should be injected, got %q", out.Context)
	}
	if out := r.RunLifecycleHooks(ctx, "SES1", db.HookStop, LifecycleExtras{}); out.Context != "" {
		t.Fatalf("Stop plain stdout must NOT inject, got %q", out.Context)
	}
}

// TestRunLifecycleHooks_Selector verifies a matcher gates SessionStart on its
// source (startup|resume): a hook matching "resume" must not fire on "startup".
func TestRunLifecycleHooks_Selector(t *testing.T) {
	r := lifecycleRuntime(t)
	ctx := context.Background()
	if _, err := r.db.CreateHook(ctx, db.Hook{
		Event:      db.HookSessionStart,
		Matcher:    "resume",
		Type:       "command",
		Command:    emitCmd(`{"additionalContext":"ONLY ON RESUME"}`),
		TimeoutSec: 10,
		Enabled:    true,
	}); err != nil {
		t.Fatal(err)
	}
	// startup → matcher "resume" does not match → no context.
	if out := r.RunLifecycleHooks(ctx, "SES1", db.HookSessionStart, LifecycleExtras{Source: "startup"}); out.Context != "" {
		t.Fatalf("startup should not match a resume-scoped hook, got %q", out.Context)
	}
	// resume → matches.
	if out := r.RunLifecycleHooks(ctx, "SES1", db.HookSessionStart, LifecycleExtras{Source: "resume"}); !strings.Contains(out.Context, "ONLY ON RESUME") {
		t.Fatalf("resume should match, got %q", out.Context)
	}
}

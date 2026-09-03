package workspace_test

import (
	"io"
	"log/slog"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/config"
	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/logbuf"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

func TestCreateSeedsCoreSystemAgents(t *testing.T) {
	root := t.TempDir()
	cipher, err := config.LoadSecret(root)
	if err != nil {
		t.Fatalf("load cipher: %v", err)
	}
	manager, err := workspace.NewManager(
		root,
		providers.NewRegistry(),
		agent.NewTunables(),
		cipher,
		events.NewBus(),
		logbuf.New(16),
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	t.Cleanup(manager.Close)

	wsp, err := manager.Create("system-agent-seed", "", "")
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	agents, err := wsp.DB.ListAgents(t.Context())
	if err != nil {
		t.Fatalf("list agents: %v", err)
	}
	// Derived from the definition list, not hardcoded: this test asserted "5" and
	// broke the moment the six subagent profiles became system agents. The seeding
	// contract is "every default lands, with its own Disabled flag" -- expressing it
	// that way is what makes the assertion survive the next profile.
	defs := agent.SystemAgentDefaults()
	if len(defs) == 0 {
		t.Fatal("no system agent defaults defined")
	}
	if len(agents) != len(defs) {
		t.Fatalf("agent count = %d, want %d (one per system agent default)", len(agents), len(defs))
	}
	for _, def := range defs {
		seeded, ok := wsp.DB.FindAgentBySystemKey(def.SystemKey)
		if !ok {
			t.Errorf("system agent %q not seeded", def.SystemKey)
			continue
		}
		// A built-in is a LOCKED row and is never disabled: it is the fallback
		// every role resolves to when no customisation is enabled.
		if !seeded.Locked || seeded.Disabled {
			t.Errorf("system agent %q locked=%v disabled=%v, want locked and enabled", def.SystemKey, seeded.Locked, seeded.Disabled)
		}
	}
	// The five core (non-subagent) keys are named explicitly so a refactor that
	// quietly drops one still fails here rather than silently shrinking the list.
	for _, key := range []string{"titler", "overview-summarizer", "compaction", "lesson-extractor", "insight"} {
		if _, ok := wsp.DB.FindAgentBySystemKey(key); !ok {
			t.Errorf("core system agent %q not seeded", key)
		}
	}
}

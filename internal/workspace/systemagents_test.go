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
	if len(agents) != 4 {
		t.Fatalf("agent count = %d, want 4", len(agents))
	}
	for _, key := range []string{"titler", "compactor", "lesson-extractor", "insight"} {
		seeded, ok := wsp.DB.FindAgentBySystemKey(key)
		if !ok {
			t.Errorf("system agent %q not seeded", key)
			continue
		}
		if got, want := seeded.Disabled, key == "insight"; got != want {
			t.Errorf("system agent %q disabled = %v, want %v", key, got, want)
		}
	}
}

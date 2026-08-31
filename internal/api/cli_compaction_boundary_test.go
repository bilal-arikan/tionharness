package api

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/agent"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// TestNativeCompactBoundaryPerMode pins the one value that differs between the
// two call paths. The manual offset (+2) is what the /compact command has always
// written; the automatic path appends no command message, so it must land exactly
// on the raw history length.
func TestNativeCompactBoundaryPerMode(t *testing.T) {
	for _, tc := range []struct {
		name       string
		mode       nativeCompactMode
		historyLen int
		want       int
	}{
		{"manual keeps the +2 command/report offset", nativeCompactManual, 7, 9},
		{"manual on an empty transcript", nativeCompactManual, 0, 2},
		{"auto uses the raw history length", nativeCompactAuto, 7, 7},
		{"auto on an empty transcript", nativeCompactAuto, 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := nativeCompactBoundary(tc.mode, tc.historyLen); got != tc.want {
				t.Fatalf("nativeCompactBoundary(%v, %d) = %d, want %d", tc.mode, tc.historyLen, got, tc.want)
			}
		})
	}
}

// TestNativeCompactUnavailableIsSentinel pins that a session which simply cannot
// be compacted by the CLI reports it as errNativeCompactUnavailable rather than as
// an opaque error string. The automatic gate branches on this to fall back to the
// rolling fold, so a plain fmt.Errorf here would silently turn every unavailable
// session into a hard turn failure.
func TestNativeCompactUnavailableIsSentinel(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	database, err := db.Open(filepath.Join(root, "store"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	registry := providers.NewRegistry()
	registry.SetInstances([]providers.Instance{{ID: "claude-cli", KindID: "claude-cli", Enabled: true}})
	rt := agent.NewRuntime(database, registry, agent.NewTunables(), root, root, nil, nil, "WS1", "test", nil, logger)
	// Default keyless claude-cli instance: it implements CLINativeManualCompactor,
	// so the session reaches at least the CLISessionID precondition — which is empty
	// here because no turn has ever run.
	agentRow, err := database.CreateAgent(ctx, db.Agent{Name: "Ada"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sess, err := database.CreateSession(ctx, db.Session{AgentID: agentRow.ID, Title: "T"})
	if err != nil {
		t.Fatalf("create session: %v", err)
	}

	s := newTestServer()
	s.providers = registry
	wsp := &workspace.Workspace{Meta: workspace.Meta{ID: "WS1", Name: "test"}, DB: database, Runtime: rt}

	if _, err := s.providers.Get(agentRow.ProviderRef()); err != nil {
		t.Skipf("default CLI provider unavailable in this environment: %v", err)
	}

	_, err = s.runNativeCompact(ctx, wsp, sess, nil, nativeCompactManual)
	if err == nil {
		t.Fatal("expected native compaction to be refused for a session with no CLI thread")
	}
	if !errors.Is(err, errNativeCompactUnavailable) {
		t.Fatalf("precondition failure must wrap errNativeCompactUnavailable, got %v", err)
	}
}

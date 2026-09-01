package agent

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

func TestPrepareCarriesCodexCLIEffortLevel(t *testing.T) {
	for _, level := range []string{"low", "medium", "high", "xhigh", "max", "ultra"} {
		t.Run(level, func(t *testing.T) {
			rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
			turn := toolLoopTurn{
				r:        rt,
				ctx:      context.Background(),
				agent:    db.Agent{Name: "Codex", Provider: "codex-cli", Model: "gpt-5.6-sol", ThinkingLevel: level},
				provider: providers.NewCodexCLI("codex", "gpt-5", filepath.Join(t.TempDir(), "codex-home")),
			}
			cleanup, err := turn.prepare()
			if err != nil {
				t.Fatalf("prepare: %v", err)
			}
			defer cleanup()
			if turn.req.CLIEffortLevel != level {
				t.Fatalf("CLIEffortLevel = %q, want %q", turn.req.CLIEffortLevel, level)
			}
		})
	}
}

func TestPrepareRejectsUnknownThinkingLevel(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	turn := toolLoopTurn{
		r:        rt,
		ctx:      context.Background(),
		agent:    db.Agent{Name: "Codex", Provider: "codex-cli", Model: "gpt-5.6-sol", ThinkingLevel: "turbo"},
		provider: providers.NewCodexCLI("codex", "gpt-5.6-sol", filepath.Join(t.TempDir(), "codex-home")),
	}
	if _, err := turn.prepare(); err == nil || !strings.Contains(err.Error(), "unknown thinkingLevel") {
		t.Fatalf("prepare error = %v, want unknown thinkingLevel", err)
	}
}

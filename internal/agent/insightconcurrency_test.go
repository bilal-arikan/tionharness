package agent

import (
	"path/filepath"
	"testing"

	"github.com/bilal-arikan/tionswarm/internal/db"
	"github.com/bilal-arikan/tionswarm/internal/insight"
)

// TestApplyInsightScanConcurrency covers the codex-cli serialization policy:
// parallel `codex exec` calls race for the single-use refresh token in the
// shared CODEX_HOME, so a codex analysis agent forces Concurrency=1 — while
// other providers and explicit caller values are left untouched.
func TestApplyInsightScanConcurrency(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))

	cases := []struct {
		name  string
		agent db.Agent
		in    int
		want  int
	}{
		{"codex serializes", db.Agent{ID: "a1", Provider: "codex-cli"}, 0, 1},
		{"claude untouched", db.Agent{ID: "a2", Provider: "claude-cli"}, 0, 0},
		{"explicit wins on codex", db.Agent{ID: "a3", Provider: "codex-cli"}, 3, 3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scope := insight.ScanScope{Concurrency: tc.in}
			rt.applyInsightScanConcurrency(&scope, tc.agent)
			if scope.Concurrency != tc.want {
				t.Fatalf("concurrency = %d, want %d", scope.Concurrency, tc.want)
			}
		})
	}
}

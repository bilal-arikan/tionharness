package agent

import (
	"strings"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/insight"
)

// providerKindCodexCLI is the single spelling of the codex CLI provider kind
// used by the scan-serialization policy below, so the rule lives in one place
// instead of scattered string comparisons.
const providerKindCodexCLI = "codex-cli"

// agentProviderKind resolves an agent's provider KIND id. Agent.Provider is the
// derived kind (see SyncProviderFields) and is the cheap answer; an older row
// that only carries the instance id falls back to the registry lookup. Returns
// "" when neither resolves — callers treat an unknown kind as "no special
// policy" rather than guessing.
func (r *Runtime) agentProviderKind(agent db.Agent) string {
	if kind := strings.TrimSpace(agent.Provider); kind != "" {
		return kind
	}
	if r == nil || r.providers == nil {
		return ""
	}
	return r.providers.KindOf(agent.ProviderInstanceID)
}

// applyInsightScanConcurrency serializes a retrospective scan when its analysis
// agent runs on codex-cli.
//
// Why: every `codex exec` reads CODEX_HOME/auth.json, and the OAuth refresh
// token there is SINGLE-USE and rotated on refresh. Phase 2 of the scanner runs
// defaultScanConcurrency (4) analyzer calls in parallel, and since PinCodexHome
// pins every codex turn to the app-global <dataDir>/codex-home, those calls now
// provably share one auth.json — the losers of a refresh race get "refresh
// token was revoked". The claude-cli side of the same failure mode is handled in
// toolloop.go:265-276; codex has no lock/heal yet, so the scan side avoids the
// race by never issuing two codex analyses at once.
//
// Explicit wins: a caller that set Concurrency itself keeps its value (same
// contract as MaxSessions/MaxAnalyzed).
func (r *Runtime) applyInsightScanConcurrency(scope *insight.ScanScope, agent db.Agent) {
	if scope == nil || scope.Concurrency != 0 {
		return
	}
	if r.agentProviderKind(agent) != providerKindCodexCLI {
		return
	}
	scope.Concurrency = 1
	r.logger.Info("insight scan serialized",
		"reason", "codex-cli single-use refresh token race",
		"provider", providerKindCodexCLI,
		"agent", agent.ID,
		"concurrency", 1)
}

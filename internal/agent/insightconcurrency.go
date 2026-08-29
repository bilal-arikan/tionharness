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
// WHERE THE RACE ACTUALLY IS (checked before trying to narrow this to a mutex):
// nothing in this process refreshes a codex token. providers.prepareShadowHome
// (codexcli_shadowhome.go:53) COPIES the base auth.json into a turn-local home,
// and the refresh happens inside the `codex exec` subprocess, against that copy,
// for as long as the subprocess runs. So the critical section is the whole
// subprocess, not a short in-process "refresh section" — an in-process mutex
// around any code we own would guard nothing, and holding one across the
// subprocess IS Concurrency=1 by another name. The serialization therefore
// stays until the shadow home copies the ROTATED auth.json back to the base
// under a lock (today it is discarded with the temp dir, so every codex turn
// re-presents the same refresh token). Cost is instead cut by sending one
// multi-lens analyzer call per session (insight.AnalysisRequest.Lenses), which
// removes ~4/5 of the serialized calls without touching this policy.
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

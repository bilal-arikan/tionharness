package api

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
	"github.com/bilal-arikan/tionharness/internal/workspace"
)

// Where a CLI-overhead projection comes from.
const (
	// cliOverheadSourceMeasured: the learned running mean of real turns for THIS
	// CLI version and THIS shipped tool catalog (store_token_calibration.go).
	cliOverheadSourceMeasured = "measured"
	// cliOverheadSourceReference: the hand-measured constants in
	// conversation/clioverhead.go, the floor used until the first turn is learned.
	cliOverheadSourceReference = "reference"
)

// learnsCLIOverhead reports whether a provider's harness overhead is learned.
// codex-cli is excluded on purpose: its stream does not expose a per-call prompt
// size, so there is nothing precise to learn from.
func learnsCLIOverhead(provider string) bool { return provider == "claude-cli" }

// countEagerTools counts the core-tier bridged tools, the ones that ship a full
// JSON schema at turn start. Deferred tools are name-only stubs until activated,
// so they are excluded from the projection (which is therefore a floor).
func countEagerTools(defs []providers.ToolDef) int {
	n := 0
	for _, d := range defs {
		if interactionTier(d.Name) == "core" {
			n++
		}
	}
	return n
}

// cliOverheadKey names the calibration entry for this agent's CLI harness: the
// installed CLI version and the fingerprint of the tool catalog the agent ships.
// Either changing (a CLI update, a tool added/removed/re-described) starts a
// fresh measurement under a new key, so a stale harness cost is never reused.
func (s *Server) cliOverheadKey(ctx context.Context, wsp *workspace.Workspace, agent db.Agent) (key, version, fingerprint string, ok bool) {
	if !learnsCLIOverhead(agent.Provider) || s.providers == nil {
		return "", "", "", false
	}
	version = claudeCLIVersion(ctx, s.providers.ClaudeCLIPath())
	fingerprint = conversation.CatalogFingerprint(wsp.Runtime.ShippedToolCatalog(ctx, agent))
	return db.CLIOverheadCalibrationKey(agent.Provider, version, fingerprint), version, fingerprint, true
}

// learnedCLIOverhead returns the measured harness overhead for this agent's
// CLI version + tool catalog, if any turn has been learned under that key.
func (s *Server) learnedCLIOverhead(ctx context.Context, wsp *workspace.Workspace, agent db.Agent) (db.TokenCalibration, bool) {
	key, _, _, ok := s.cliOverheadKey(ctx, wsp, agent)
	if !ok {
		return db.TokenCalibration{}, false
	}
	c, found := wsp.DB.TokenCalibration(key)
	if !found || c.Samples <= 0 {
		return db.TokenCalibration{}, false
	}
	return c, true
}

// projectedCLIOverhead is the harness cost to budget for an agent's next turn:
// the learned value when one exists, otherwise the hand-measured reference
// projection for eagerTools bridged schemas. source names which one was used.
func (s *Server) projectedCLIOverhead(ctx context.Context, wsp *workspace.Workspace, agent db.Agent, eagerTools int) (tokens int, source string, samples int) {
	if c, ok := s.learnedCLIOverhead(ctx, wsp, agent); ok {
		return c.Tokens, cliOverheadSourceMeasured, c.Samples
	}
	return conversation.PredictCLIOverhead(eagerTools), cliOverheadSourceReference, 0
}

// measuredPromptTokens extracts the prompt size of the turn's FIRST model call
// from a response: the per-call figure the CLI streamed when it has one, else the
// cumulative usage when the turn was a single round-trip (then cumulative ==
// single). Multi-call turns without a per-call figure return 0: dividing the
// cumulative usage by the call count is an average over a growing prompt, too
// noisy to learn from.
func measuredPromptTokens(resp *providers.Response) int {
	if resp == nil {
		return 0
	}
	if resp.FirstCallPromptTokens > 0 {
		return resp.FirstCallPromptTokens
	}
	if resp.ProviderCalls > 1 {
		return 0
	}
	return resp.Usage.InputTokens + resp.Usage.CacheReadTokens + resp.Usage.CacheWriteTokens
}

// learnCLIOverhead folds one completed chat turn into the harness-overhead
// calibration: the real prompt the CLI sent minus TionHarness's pre-send estimate
// of the same turn (messages + summary + the non-message overhead the fold gate
// budgets). A negative gap means the heuristic over-counted; it is clamped to 0
// but still counted as a sample so the mean reflects it. Best-effort: a failed
// write only delays learning to the next turn.
func (s *Server) learnCLIOverhead(ctx context.Context, wsp *workspace.Workspace, agent db.Agent, estimated int, resp *providers.Response) (db.TokenCalibration, bool) {
	if estimated <= 0 {
		return db.TokenCalibration{}, false
	}
	measured := measuredPromptTokens(resp)
	if measured <= 0 {
		return db.TokenCalibration{}, false
	}
	key, version, fingerprint, ok := s.cliOverheadKey(ctx, wsp, agent)
	if !ok {
		return db.TokenCalibration{}, false
	}
	over := measured - estimated
	if over < 0 {
		over = 0
	}
	c, err := wsp.DB.ObserveTokenCalibration(ctx, db.TokenCalibration{
		Key:         key,
		Kind:        db.TokenCalibrationCLIOverhead,
		Provider:    agent.Provider,
		CLIVersion:  version,
		Fingerprint: fingerprint,
		Tokens:      over,
	})
	if err != nil {
		s.logger.Warn("token calibration: cli overhead not persisted", "agent", agent.ID, "error", err)
		return c, false
	}
	s.logger.Debug("token calibration: cli overhead learned",
		"agent", agent.ID, "cli_version", version, "catalog", fingerprint,
		"measured", measured, "estimated", estimated, "overhead", over, "mean", c.Tokens, "samples", c.Samples)
	return c, true
}

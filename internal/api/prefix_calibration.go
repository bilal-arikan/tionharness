package api

import (
	"context"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/conversation"
	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

// Exact prompt-prefix counting.
//
// The static prefix of a turn (system prompt: persona, profile, catalogs, plus
// the shipped tool schemas) is identical from turn to turn until the agent's
// configuration changes, and it is usually the largest single share of the
// context. Providers exposing a server-side counter (Anthropic
// /v1/messages/count_tokens) can price it EXACTLY, once; the number is then
// remembered under the prefix fingerprint (store_token_calibration.go) and reused
// on every later turn, so only the per-turn part (messages, dynamic suffix) is
// still estimated heuristically.

const (
	// prefixCountTimeout bounds one count call: it is a small request with no
	// generation, and a slow provider must not stall the turn it runs inside.
	prefixCountTimeout = 8 * time.Second
	// prefixCountRetryAfter is how long a prefix that failed to count is left
	// alone before the next miss retries it, so a broken key / offline provider
	// costs one failed call per prefix per window, not one per turn.
	prefixCountRetryAfter = 10 * time.Minute
)

// prefixCountFailures remembers, per calibration key, when counting last failed.
var prefixCountFailures sync.Map // key -> time.Time

// prefixRoles are the context-meter buckets that make up the counted prefix.
var prefixRoles = map[string]bool{"system": true, "skills": true, "lazy-tools": true, "tools": true}

// tokenCounterFor resolves the agent's provider and reports it as a TokenCounter
// when it can count server-side; (nil, false) for every other provider.
func (s *Server) tokenCounterFor(agent db.Agent) (providers.TokenCounter, bool) {
	if s.providers == nil {
		return nil, false
	}
	p, err := s.providers.Get(agent.ProviderRef())
	if err != nil {
		return nil, false
	}
	tc, ok := p.(providers.TokenCounter)
	return tc, ok
}

// exactPrefixTokens returns the exact token count of (system + defs) for the
// agent's model. The cached count is returned when the prefix has been counted
// before; on a miss the prefix is counted now only when measure is true (the
// turn path), otherwise the caller keeps its heuristic. ok is false whenever no
// exact figure is available.
func exactPrefixTokens(ctx context.Context, database *db.DB, counter providers.TokenCounter, agent db.Agent, system string, defs []providers.ToolDef, measure bool) (int, bool) {
	if counter == nil || database == nil {
		return 0, false
	}
	if system == "" && len(defs) == 0 {
		return 0, false
	}
	fingerprint := conversation.PrefixFingerprint(agent.Model, system, defs)
	key := db.PrefixCalibrationKey(agent.Provider, agent.Model, fingerprint)
	if c, ok := database.TokenCalibration(key); ok && c.Samples > 0 {
		return c.Tokens, true
	}
	if !measure {
		return 0, false
	}
	if at, failed := prefixCountFailures.Load(key); failed && time.Since(at.(time.Time)) < prefixCountRetryAfter {
		return 0, false
	}
	cctx, cancel := context.WithTimeout(ctx, prefixCountTimeout)
	defer cancel()
	n, err := counter.CountTokens(cctx, providers.Request{Model: agent.Model, System: system, Tools: defs})
	if err != nil || n <= 0 {
		prefixCountFailures.Store(key, time.Now())
		return 0, false
	}
	prefixCountFailures.Delete(key)
	_ = database.SetTokenCalibration(ctx, db.TokenCalibration{
		Key:         key,
		Kind:        db.TokenCalibrationPrefix,
		Provider:    agent.Provider,
		Model:       agent.Model,
		Fingerprint: fingerprint,
		Tokens:      n,
	})
	return n, true
}

// applyPrefixCalibration rescales the prefix buckets of a filler list so they sum
// to the exact count, keeping their relative shares (the heuristic still says
// which bucket is the big one; the exact count says how big the whole prefix
// is). Non-prefix buckets are untouched. Rounding remainder lands on the largest
// prefix bucket so the sum equals exact to the token.
func applyPrefixCalibration(fillers []contextFiller, exact int) []contextFiller {
	if exact <= 0 {
		return fillers
	}
	parts := make([]*int, 0, len(fillers))
	for i := range fillers {
		if prefixRoles[fillers[i].Role] {
			parts = append(parts, &fillers[i].Tokens)
			fillers[i].Calibrated = true
		}
	}
	scaleToExact(exact, parts...)
	return fillers
}

// scaleToExact rescales a set of heuristic figures so they sum to exact,
// preserving their relative shares; the rounding remainder goes to the largest
// figure. No-op when the figures sum to zero or exact is not positive.
func scaleToExact(exact int, parts ...*int) {
	sum, largest := 0, -1
	for i, p := range parts {
		sum += *p
		if largest < 0 || *p > *parts[largest] {
			largest = i
		}
	}
	if sum <= 0 || exact <= 0 {
		return
	}
	scaled := 0
	for _, p := range parts {
		*p = *p * exact / sum
		scaled += *p
	}
	*parts[largest] += exact - scaled
}

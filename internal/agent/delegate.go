package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal-arikan/swarmgo/internal/db"
	"github.com/bilal-arikan/swarmgo/internal/providers"
)

// Subagent guard defaults. They bound the agent→subagent call graph so a chain
// can neither recurse without end nor fan out into a call storm:
//
//   - depth   — how deeply subagent runs may nest (a→b→c→d).
//   - budget  — how many subagent runs a single user turn may spend in total.
//
// A third guard, the visited-set, has no scalar: an agent already present in the
// current call chain (including the caller itself) can never be re-summoned,
// which closes every cycle (a→b→a) regardless of depth. These are consumed by
// runAgent (subagent.go) and surfaced as tunables (DelegationMax*).
const (
	DefaultMaxDelegationDepth = 3
	DefaultMaxDelegationCalls = 8
)

// delegState carries the per-turn call-graph position through the context so a
// nested subagent run inherits the chain that led to it. The budget counter is a
// shared *int32 mutated atomically, so parallel fan-out is race-free.
type delegState struct {
	depth   int             // subagent runs already nested above this point (0 = top)
	visited map[string]bool // agent ids in the current chain (cycle/self guard)
	calls   *int32          // shared per-turn run counter (fan-out budget); atomic
}

// delegStateKey keys delegState on a context.
type delegStateKey struct{}

// delegStateFrom returns the chain position carried in ctx (zero value + false
// when this is the top of a turn).
func delegStateFrom(ctx context.Context) (delegState, bool) {
	s, ok := ctx.Value(delegStateKey{}).(delegState)
	return s, ok
}

// resolveAgent finds a workspace agent by id first, then by case-insensitive
// display name. A leading "@" (the chat mention form) is tolerated. Shared by the
// subagent runner and the spawn path.
func (r *Runtime) resolveAgent(ctx context.Context, ref string) (db.Agent, error) {
	ref = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ref), "@"))
	if ref == "" {
		return db.Agent{}, fmt.Errorf("no agent specified")
	}
	if a, err := r.db.GetAgent(ctx, ref); err == nil {
		return a, nil
	}
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return db.Agent{}, err
	}
	for _, a := range agents {
		if strings.EqualFold(strings.TrimSpace(a.Name), ref) {
			return a, nil
		}
	}
	return db.Agent{}, fmt.Errorf("no agent named %q in this workspace", ref)
}

// inheritedMessages flattens the caller's live request into a clean,
// human-readable conversation for an inherited-context subagent: user and
// assistant text turns are kept; tool-call / tool-result plumbing is dropped so
// the child provider never receives a dangling tool_use it cannot answer.
func inheritedMessages(reqPtr *providers.Request) []providers.Message {
	if reqPtr == nil {
		return nil
	}
	out := make([]providers.Message, 0, len(reqPtr.Messages))
	for _, m := range reqPtr.Messages {
		if strings.TrimSpace(m.Text) == "" {
			continue // tool-result-only or empty turns carry no readable content
		}
		out = append(out, providers.Message{Role: m.Role, Text: m.Text})
	}
	return out
}

package agent

import (
	"context"
	"fmt"
	"strings"

	"github.com/bilal/swarmgo/internal/db"
	"github.com/bilal/swarmgo/internal/providers"
	"github.com/bilal/swarmgo/internal/tools"
)

// Delegation guard defaults. They bound agent→agent delegation so a chain can
// neither recurse without end nor fan out into a call storm:
//
//   - depth   — how deeply delegations may nest (a→b→c→d).
//   - budget  — how many delegations a single user turn may spend in total.
//
// A third guard, the visited-set, has no scalar: an agent already present in the
// current call chain (including the caller itself) can never be re-summoned,
// which closes every cycle (a→b→a) regardless of depth.
const (
	DefaultMaxDelegationDepth = 3
	DefaultMaxDelegationCalls = 8
)

// delegState carries the per-turn call-graph position through the context so a
// recursive completion inherits the chain that led to it.
type delegState struct {
	depth   int             // delegations already nested above this point (0 = top)
	visited map[string]bool // agent ids in the current chain (cycle/self guard)
	calls   *int            // shared per-turn delegation counter (fan-out budget)
}

// delegStateKey keys delegState on a context.
type delegStateKey struct{}

// delegStateFrom returns the chain position carried in ctx (zero value + false
// when this is the top of a turn).
func delegStateFrom(ctx context.Context) (delegState, bool) {
	s, ok := ctx.Value(delegStateKey{}).(delegState)
	return s, ok
}

// withDelegation wires the call_agent tool for one completion. It seeds the
// chain position (top-level: depth 0, the current agent marked visited, a fresh
// budget counter) or inherits it from a parent delegation, then attaches a
// runner that summons another agent. reqPtr points at the live request so the
// summoned agent inherits the conversation exactly as the caller currently sees
// it — including the caller's own narration appended earlier in this turn.
func (r *Runtime) withDelegation(ctx context.Context, caller db.Agent, reqPtr *providers.Request, autonomous bool) context.Context {
	cur, ok := delegStateFrom(ctx)
	if !ok {
		n := 0
		cur = delegState{depth: 0, visited: map[string]bool{caller.ID: true}, calls: &n}
		ctx = context.WithValue(ctx, delegStateKey{}, cur)
	}

	maxDepth := r.tun.DelegationMaxDepth()
	maxCalls := r.tun.DelegationMaxCalls()

	runner := func(rctx context.Context, target, task string) (tools.DelegateResult, error) {
		// Guard 1 — depth: refuse once the chain is already as deep as allowed.
		if cur.depth >= maxDepth {
			return tools.DelegateResult{}, fmt.Errorf("delegation depth limit (%d) reached; answer directly instead of delegating further", maxDepth)
		}
		// Guard 2 — budget: cap total delegations across the whole turn's tree.
		if cur.calls != nil && *cur.calls >= maxCalls {
			return tools.DelegateResult{}, fmt.Errorf("delegation budget (%d calls per turn) exhausted; answer directly", maxCalls)
		}

		sub, err := r.resolveAgent(rctx, target)
		if err != nil {
			return tools.DelegateResult{}, err
		}
		// Guard 3 — visited-set: an agent already in the chain (or the caller
		// itself) must not be re-summoned; this closes every cycle.
		if cur.visited[sub.ID] {
			return tools.DelegateResult{}, fmt.Errorf("agent %q is already part of this delegation chain; pick a different agent", sub.Name)
		}

		provider, err := r.providers.Get(sub.Provider)
		if err != nil {
			return tools.DelegateResult{}, fmt.Errorf("agent %q provider unavailable: %w", sub.Name, err)
		}

		// Spend one unit of the shared budget for this delegation.
		if cur.calls != nil {
			*cur.calls++
		}

		// Build the child chain position: one level deeper, the summoned agent
		// added to the visited-set, sharing the same budget counter.
		childVisited := make(map[string]bool, len(cur.visited)+1)
		for id := range cur.visited {
			childVisited[id] = true
		}
		childVisited[sub.ID] = true
		childCtx := context.WithValue(rctx, delegStateKey{}, delegState{
			depth:   cur.depth + 1,
			visited: childVisited,
			calls:   cur.calls,
		})

		// The summoned agent inherits the caller's conversation (so it sees the
		// same history and what the caller just wrote) plus the delegated task.
		msgs := inheritedMessages(reqPtr)
		msgs = append(msgs, providers.Message{
			Role: providers.RoleUser,
			Text: fmt.Sprintf("[Delegated by agent %q]\n\n%s", caller.Name, task),
		})
		childReq := providers.Request{
			Model:    sub.Model,
			System:   r.systemPrompt(sub),
			Messages: msgs,
		}

		r.logger.Info("agent delegation",
			"from", caller.ID, "to", sub.ID, "depth", cur.depth+1, "calls", *cur.calls)

		resp, _, err := r.completeTraced(childCtx, sub, provider, childReq, autonomous, nil)
		if err != nil {
			return tools.DelegateResult{}, fmt.Errorf("agent %q failed: %w", sub.Name, err)
		}
		return tools.DelegateResult{AgentName: sub.Name, Reply: resp.Text}, nil
	}

	return tools.WithDelegation(ctx, runner)
}

// resolveAgent finds a workspace agent by id first, then by case-insensitive
// display name. A leading "@" (the chat mention form) is tolerated.
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
// human-readable conversation for the summoned agent: user and assistant text
// turns are kept; tool-call / tool-result plumbing is dropped so the child
// provider never receives a dangling tool_use it cannot answer.
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

package agent

import (
	"context"
	"fmt"

	"github.com/bilal-arikan/tionharness/internal/db"
)

// systemRoleCaller returns the identity an internal job with no owner of its own
// runs as (user decision 2026-10-07): the role's own system agent — assignment →
// enabled customisation → locked built-in row. Its session, usage and billing
// land on that agent, never on an ordinary agent the user did not set up for the
// job (the insight scan and ownerless titles used to borrow
// the default or newest agent).
//
// Only the TRANSPORT may be borrowed: when the system agent's provider cannot be
// served here (a codex-only install whose built-ins name claude-cli), the
// workspace's default agent lends provider/instance/model, else the first live
// ordinary agent whose provider is available. Identity stays on the system agent.
func (r *Runtime) systemRoleCaller(ctx context.Context, key string) (db.Agent, error) {
	sa, _, err := r.ResolveSystemAgent(key)
	if err != nil || sa.ID == "" {
		// Unseeded store or unresolvable role (every real workspace seeds its
		// system agents on open): there is no row to bill, so the lender carries
		// the call, with the role's prompt/model (or the embedded fallback)
		// applied downstream.
		lend, ok := r.transportLender(ctx)
		if !ok {
			return db.Agent{}, fmt.Errorf("system agent %q is not seeded and no agent can carry the call", key)
		}
		r.logger.Warn("system role has no agent row; running as a lender agent", "systemKey", key, "lender", lend.ID)
		return lend, nil
	}
	if r.systemTransportUsable(sa) {
		return sa, nil
	}
	if lend, ok := r.transportLender(ctx); ok {
		r.logger.Info("system role borrows transport", "systemKey", key, "lender", lend.ID, "provider", lend.Provider)
		sa.Provider = lend.Provider
		sa.ProviderInstanceID = lend.ProviderInstanceID
		sa.Model = lend.Model
	}
	return sa, nil
}

// systemTransportUsable reports whether a system agent's own provider can run
// here. A pinned provider is the user's explicit choice and always counts (an
// unavailable one should fail visibly); a registry-less runtime (tests) too.
func (r *Runtime) systemTransportUsable(sa db.Agent) bool {
	if pinsProvider(sa) || r.providers == nil {
		return true
	}
	inst := sa.ProviderInstanceID
	if inst == "" {
		inst = sa.Provider
	}
	if inst == "" {
		inst = "claude-cli"
	}
	return r.providers.Available(inst) && !r.systemRouteQuarantined(inst)
}

// transportLender picks the ordinary agent whose provider a system role may
// borrow: the workspace default agent, else the first live non-system agent
// whose provider is available.
func (r *Runtime) transportLender(ctx context.Context) (db.Agent, bool) {
	live := func(a db.Agent) bool {
		return !a.System && !a.Disabled && !a.Deleted && a.RunnableErr() == nil
	}
	if id := r.DefaultAgentID(); id != "" {
		if a, err := r.db.GetAgent(ctx, id); err == nil && live(a) {
			return a, true
		}
	}
	agents, err := r.db.ListAgents(ctx)
	if err != nil {
		return db.Agent{}, false
	}
	for _, a := range agents {
		if live(a) && r.systemTransportUsable(db.Agent{Provider: a.Provider, ProviderInstanceID: a.ProviderInstanceID}) {
			return a, true
		}
	}
	return db.Agent{}, false
}

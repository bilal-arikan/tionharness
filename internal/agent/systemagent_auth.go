package agent

import (
	"context"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func (r *Runtime) systemRouteQuarantined(instance string) bool {
	value, ok := r.systemRouteBad.Load(instance)
	if !ok || r.providers == nil {
		return false
	}
	if generation, _ := value.(uint64); generation == r.providers.Generation() {
		return true
	}
	r.systemRouteBad.Delete(instance)
	return false
}

// Only an unpinned auxiliary role can fall back. Ordinary agent turns and
// explicit provider choices retain their errors and never change transport.
func (r *Runtime) systemAuthFallback(ctx context.Context, routed db.Agent, err error) (db.Agent, bool) {
	if routed.SystemKey == "" || classifyProviderError(err) != errAuth || r.providers == nil {
		return db.Agent{}, false
	}
	system, _, resolveErr := r.ResolveSystemAgent(routed.SystemKey)
	if resolveErr != nil || pinsProvider(system) {
		return db.Agent{}, false
	}
	caller, lookupErr := r.db.GetAgent(ctx, routed.ID)
	if lookupErr != nil || caller.System || caller.ProviderRef() == routed.ProviderRef() || !r.providers.Available(caller.ProviderRef()) {
		return db.Agent{}, false
	}
	r.systemRouteBad.Store(routed.ProviderRef(), r.providers.Generation())
	fallback := routed
	fallback.Provider = caller.Provider
	fallback.ProviderInstanceID = caller.ProviderInstanceID
	fallback.Model = adoptSystemAgentModel(r.logger, routed.SystemKey, caller.Provider, caller.Model, system.Model)
	return fallback, true
}

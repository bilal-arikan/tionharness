package agent

import "github.com/bilal-arikan/tionharness/internal/decider"

// SetDecider wires the app-wide decision-model hub. The API server builds one
// hub per process and hands it here, so every workspace runtime (they all share
// this Tunables) consults the same configuration, client and ledger.
func (t *Tunables) SetDecider(h *decider.Hub) {
	t.mu.Lock()
	t.decider = h
	t.mu.Unlock()
}

// Decider returns the decision-model hub, or nil when none is wired (tests,
// early boot). A nil hub reads as "every site off" everywhere it is consulted.
func (t *Tunables) Decider() *decider.Hub {
	if t == nil {
		return nil
	}
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.decider
}

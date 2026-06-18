package tools

import "sync"

// ActiveTools is the per-turn set of lazy tools the model has activated via the
// activate_tools tool. It is consulted each loop iteration to decide which lazy
// tool schemas to ship (Registry.ActiveDefs). It also tracks per-tool age and
// last-use so unused activations can be pruned in long turns (Phase 3).
//
// It is safe for concurrent use: tools mutate it from Call while the loop reads.
type ActiveTools struct {
	mu      sync.Mutex
	set     map[string]bool
	addedAt map[string]int // iteration the tool was activated
	usedAt  map[string]int // last iteration the tool was actually called
	iter    int            // current loop iteration (set by the loop)
}

// NewActiveTools returns an empty active set.
func NewActiveTools() *ActiveTools {
	return &ActiveTools{set: map[string]bool{}, addedAt: map[string]int{}, usedAt: map[string]int{}}
}

// SetIter records the loop's current iteration, used for age-based pruning.
func (a *ActiveTools) SetIter(i int) {
	if a == nil {
		return
	}
	a.mu.Lock()
	a.iter = i
	a.mu.Unlock()
}

// Activate adds tools to the active set, returning the names that were newly
// added (already-active names are reported via the second return value).
func (a *ActiveTools) Activate(names ...string) (added, already []string) {
	if a == nil {
		return nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, n := range names {
		if n == "" {
			continue
		}
		if a.set[n] {
			already = append(already, n)
			continue
		}
		a.set[n] = true
		a.addedAt[n] = a.iter
		a.usedAt[n] = a.iter // grace: counts as fresh so it is not pruned immediately
		added = append(added, n)
	}
	return added, already
}

// Deactivate removes tools from the active set, returning those actually removed.
func (a *ActiveTools) Deactivate(names ...string) (removed []string) {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, n := range names {
		if a.set[n] {
			delete(a.set, n)
			delete(a.addedAt, n)
			delete(a.usedAt, n)
			removed = append(removed, n)
		}
	}
	return removed
}

// MarkUsed records that a tool was called this iteration (resets its idle age).
func (a *ActiveTools) MarkUsed(name string) {
	if a == nil {
		return
	}
	a.mu.Lock()
	if a.set[name] {
		a.usedAt[name] = a.iter
	}
	a.mu.Unlock()
}

// Snapshot returns a copy of the active set for ActiveDefs.
func (a *ActiveTools) Snapshot() map[string]bool {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make(map[string]bool, len(a.set))
	for n := range a.set {
		out[n] = true
	}
	return out
}

// Has reports whether a tool is currently active.
func (a *ActiveTools) Has(name string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.set[name]
}

// Prune drops tools that have been idle (never called) for more than maxIdle
// iterations, keeping the shipped schema set lean across a long turn. Returns
// the pruned names. A non-positive maxIdle disables pruning.
func (a *ActiveTools) Prune(maxIdle int) (pruned []string) {
	if a == nil || maxIdle <= 0 {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for n := range a.set {
		if a.iter-a.usedAt[n] > maxIdle {
			delete(a.set, n)
			delete(a.addedAt, n)
			delete(a.usedAt, n)
			pruned = append(pruned, n)
		}
	}
	return pruned
}

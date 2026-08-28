package tools

import (
	"sort"
	"sync"
)

// ActiveTools is the per-turn set of lazy tools the model has activated via the
// activate_tools tool. It is consulted each loop iteration to decide which lazy
// tool schemas to ship (Registry.ActiveDefs). It also tracks per-tool age and
// last-use so unused activations can be pruned in long turns (Phase 3).
//
// It is safe for concurrent use: tools mutate it from Call while the loop reads.
type ActiveTools struct {
	mu            sync.Mutex
	set           map[string]bool
	addedAt       map[string]int  // iteration the tool was activated
	usedAt        map[string]int  // last iteration the tool was actually called
	autoActivated map[string]bool // tools already recovered by automatic activation
	iter          int             // current loop iteration (set by the loop)

	// groups is the set of BUNDLE keys the model has opened this turn (see
	// bundles.go). Opening a bundle only lists its members' summaries — it never
	// puts a member into `set`, so no schema is shipped for it and Prune has
	// nothing to reclaim. Tracked purely so a repeated open can be reported as
	// "already opened" instead of re-printing the whole listing.
	groups map[string]bool
}

// NewActiveTools returns an empty active set.
func NewActiveTools() *ActiveTools {
	return &ActiveTools{set: map[string]bool{}, addedAt: map[string]int{}, usedAt: map[string]int{}, autoActivated: map[string]bool{}, groups: map[string]bool{}}
}

// OpenBundle records bundle keys as opened, returning the keys that were newly
// opened and the ones already open. It deliberately does NOT touch the active
// tool set: a bundle carries summaries only.
func (a *ActiveTools) OpenBundle(keys ...string) (opened, already []string) {
	if a == nil {
		return nil, nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.groups == nil {
		a.groups = map[string]bool{}
	}
	for _, k := range keys {
		if k == "" {
			continue
		}
		if a.groups[k] {
			already = append(already, k)
			continue
		}
		a.groups[k] = true
		opened = append(opened, k)
	}
	return opened, already
}

// CloseBundle forgets bundle keys, returning those that were actually open.
func (a *ActiveTools) CloseBundle(keys ...string) (closed []string) {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, k := range keys {
		if a.groups[k] {
			delete(a.groups, k)
			closed = append(closed, k)
		}
	}
	return closed
}

// HasBundle reports whether a bundle key is currently open.
func (a *ActiveTools) HasBundle(key string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.groups[key]
}

// OpenBundles returns a sorted snapshot of the open bundle keys.
func (a *ActiveTools) OpenBundles() []string {
	if a == nil {
		return nil
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.groups))
	for k := range a.groups {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// AutoActivate activates name and reports whether this is its first automatic
// activation attempt this turn. A repeated attempt is rejected to prevent a
// model from looping on the same schema-less call.
func (a *ActiveTools) AutoActivate(name string) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.autoActivated[name] {
		return false
	}
	a.autoActivated[name] = true
	a.set[name] = true
	a.addedAt[name] = a.iter
	a.usedAt[name] = a.iter
	return true
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

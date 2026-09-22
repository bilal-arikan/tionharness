package procwatch

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultHistory is how many FINISHED processes a registry retains. Running
// entries are never dropped — they are live state, and a process the ledger
// forgot is exactly the invisible process this package exists to prevent.
const DefaultHistory = 300

// commandLimit bounds the retained command line, in runes. See capRunes.
const commandLimit = 2000

// Registry is the ledger: every live process plus a bounded history of recently
// finished ones. Safe for concurrent use.
type Registry struct {
	mu      sync.Mutex
	live    map[string]*Handle
	done    []Entry // oldest→newest
	maxDone int
	seq     int64
	// notify, when set, receives an entry snapshot on every state change (start,
	// stop request, finish). Used to fan the ledger out over SSE. It runs on the
	// caller's goroutine outside the registry lock, so it must not block.
	notify atomic.Pointer[func(Entry)]
}

// New returns an empty registry retaining history finished entries (<= 0 uses
// DefaultHistory).
func New(history int) *Registry {
	if history <= 0 {
		history = DefaultHistory
	}
	return &Registry{live: map[string]*Handle{}, maxDone: history}
}

// SetNotify installs the state-change callback. Pass nil to remove.
func (r *Registry) SetNotify(fn func(Entry)) {
	if r == nil {
		return
	}
	if fn == nil {
		r.notify.Store(nil)
		return
	}
	r.notify.Store(&fn)
}

// Begin registers a process that is about to be started and returns its handle.
// The returned handle is nil when r is nil, and every Handle method tolerates a
// nil receiver — so an uninstrumented context (a unit test, a preview build)
// costs the call site nothing but the Begin line itself.
//
// ctx is kept to classify the outcome later: a run whose context hit its
// deadline is reported as timed out rather than as a generic failure.
func (r *Registry) Begin(ctx context.Context, m Meta) *Handle {
	if r == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	owner := mergeOwner(OwnerFrom(ctx), m.Owner)
	r.mu.Lock()
	r.seq++
	id := fmt.Sprintf("p%d", r.seq)
	h := &Handle{
		reg: r,
		ctx: ctx,
		e: Entry{
			ID:        id,
			Kind:      m.Kind,
			Label:     m.Label,
			Command:   capRunes(m.Command, commandLimit),
			Dir:       m.Dir,
			Status:    StatusRunning,
			StartedAt: time.Now().UnixMilli(),
			Owner:     owner,
			Stoppable: m.Stop != nil,
		},
		stop: m.Stop,
	}
	r.live[id] = h
	r.mu.Unlock()
	r.emit(h.snapshot())
	return h
}

// finish moves a handle out of the live set and into the bounded history.
func (r *Registry) finish(h *Handle) {
	r.mu.Lock()
	delete(r.live, h.e.ID)
	r.done = append(r.done, h.snapshot())
	if len(r.done) > r.maxDone {
		r.done = append([]Entry(nil), r.done[len(r.done)-r.maxDone:]...)
	}
	r.mu.Unlock()
}

// emit hands a snapshot to the notify callback, if one is installed.
func (r *Registry) emit(e Entry) {
	if fn := r.notify.Load(); fn != nil {
		(*fn)(e)
	}
}

// Filter narrows a List call. A zero value matches everything. The string
// facets are exact matches; Statuses and Kinds are OR-sets.
type Filter struct {
	Statuses []Status
	Kinds    []Kind
	// Owner facets. An empty field does not filter.
	WorkspaceID string
	SessionID   string
	AgentID     string
	// Limit caps the returned rows to the newest N (<= 0 means every match).
	Limit int
}

func (f Filter) matches(e Entry) bool {
	if len(f.Statuses) > 0 && !slices.Contains(f.Statuses, e.Status) {
		return false
	}
	if len(f.Kinds) > 0 && !slices.Contains(f.Kinds, e.Kind) {
		return false
	}
	if f.WorkspaceID != "" && e.Owner.WorkspaceID != f.WorkspaceID {
		return false
	}
	if f.SessionID != "" && e.Owner.SessionID != f.SessionID {
		return false
	}
	if f.AgentID != "" && e.Owner.AgentID != f.AgentID {
		return false
	}
	return true
}

// List returns matching entries newest-first (by start time). Running and
// finished processes come back in one list: the panel shows a single timeline,
// and a caller that wants only one of them says so with Filter.Statuses.
func (r *Registry) List(f Filter) []Entry {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	out := make([]Entry, 0, len(r.live)+len(r.done))
	for _, h := range r.live {
		if e := h.snapshot(); f.matches(e) {
			out = append(out, e)
		}
	}
	for _, e := range slices.Backward(r.done) {
		if f.matches(e) {
			out = append(out, e)
		}
	}
	r.mu.Unlock()
	slices.SortStableFunc(out, func(a, b Entry) int {
		return cmp.Compare(b.StartedAt, a.StartedAt)
	})
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out
}

// Get returns one entry by id, live or finished.
func (r *Registry) Get(id string) (Entry, bool) {
	if r == nil {
		return Entry{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.live[id]; ok {
		return h.snapshot(), true
	}
	for _, e := range slices.Backward(r.done) {
		if e.ID == id {
			return e, true
		}
	}
	return Entry{}, false
}

// Stop terminates a running process. It returns an error when the id is
// unknown, and reports ok=false (with no error) when the entry exists but is
// already finished or was registered without a cancel func — "nothing to stop"
// is a normal outcome of a race with the process exiting, not a failure.
//
// Stop only SIGNALS: the process dies when its cancel func's kill lands, and the
// entry reaches its terminal status when the site that started it reaps it.
func (r *Registry) Stop(id string) (ok bool, err error) {
	if r == nil {
		return false, fmt.Errorf("process registry unavailable")
	}
	r.mu.Lock()
	h := r.live[id]
	r.mu.Unlock()
	if h == nil {
		if _, found := r.Get(id); found {
			return false, nil // already finished
		}
		return false, fmt.Errorf("no tracked process %q", id)
	}
	return h.RequestStop(), nil
}

// capRunes truncates s to at most max runes (UTF-8 safe). A command line is
// attacker-shaped input as far as this package is concerned — an agent can put a
// whole heredoc script on one line — and the ledger is held in memory and
// broadcast over SSE, so it is bounded here rather than at every call site.
func capRunes(s string, max int) string {
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

// mergeOwner overlays the explicitly declared owner fields on the ones carried
// by the context; a declared field always wins.
func mergeOwner(base, over Owner) Owner {
	if over.WorkspaceID != "" {
		base.WorkspaceID = over.WorkspaceID
	}
	if over.SessionID != "" {
		base.SessionID = over.SessionID
	}
	if over.AgentID != "" {
		base.AgentID = over.AgentID
	}
	if over.AgentName != "" {
		base.AgentName = over.AgentName
	}
	if over.ParentSessionID != "" {
		base.ParentSessionID = over.ParentSessionID
	}
	return base
}

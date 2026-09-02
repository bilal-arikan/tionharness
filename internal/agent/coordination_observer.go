package agent

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/events"
)

// Coordination observer (_Docs/77 R7).
//
// coordination.go is the largest file in the runtime; everything that wants to
// KNOW about coordinator/worker lifecycle — the workspace stream, the trajectory
// binder, tests — used to mean more code inside it. The observer seam turns
// those into subscribers: the coordination paths call the four hooks below at
// their existing emit points and know nothing about who listens.
//
// Hooks run synchronously on the coordination goroutine, so an observer must
// return promptly and never block on the runtime; a panicking observer is
// recovered and logged so it can never take a coordinator down.

// SpawnEvent: a coordinator spawned a worker (or a sub-coordinator).
type SpawnEvent struct {
	CoordinatorID  string `json:"coordinatorId"`
	WorkerID       string `json:"workerId"`
	RootID         string `json:"rootId"`
	Depth          int    `json:"depth"`
	AgentRef       string `json:"agentRef"` // the requested target (name/id/profile)
	AgentName      string `json:"agentName,omitempty"`
	SubCoordinator bool   `json:"subCoordinator,omitempty"`
	Workflow       string `json:"workflow,omitempty"` // recipe ref, sub-coordinators only
	Queued         bool   `json:"queued,omitempty"`
	At             int64  `json:"at"`
}

// ReportEvent: a worker's <task-notification> reached its coordinator.
type ReportEvent struct {
	CoordinatorID string `json:"coordinatorId"`
	WorkerID      string `json:"workerId"`
	Status        string `json:"status"` // completed | failed | killed | timeout | incomplete
	LastWorker    bool   `json:"lastWorker"`
	ToolUses      int    `json:"toolUses"`
	NoteBytes     int    `json:"noteBytes"`
	At            int64  `json:"at"`
}

// DrainEvent: the coordinator's auto-turn loop started or finished one turn.
type DrainEvent struct {
	CoordinatorID string `json:"coordinatorId"`
	Phase         string `json:"phase"` // turn_start | turn_end
	Turn          int    `json:"turn"`  // auto-turn ordinal within this drain
	At            int64  `json:"at"`
}

// StallEvent: the phantom-spawn guard hard-halted a coordinator.
type StallEvent struct {
	CoordinatorID string `json:"coordinatorId"`
	AgentID       string `json:"agentId,omitempty"`
	Reason        string `json:"reason"`
	Streak        int    `json:"streak"`
	At            int64  `json:"at"`
}

// CoordinationObserver receives coordinator/worker lifecycle events.
type CoordinationObserver interface {
	OnSpawn(ev SpawnEvent)
	OnReport(ev ReportEvent)
	OnDrain(ev DrainEvent)
	OnStall(ev StallEvent)
}

// coordObserverRegistry holds the subscribers. The built-in workspace-stream
// publisher is not in the list: it is called first, unconditionally, from
// notifyCoordinationObservers, so a subscriber can never accidentally silence
// the stream by panicking first.
type coordObserverRegistry struct {
	mu   sync.RWMutex
	list []CoordinationObserver
}

// AddCoordinationObserver registers a subscriber for the runtime's lifetime.
func (r *Runtime) AddCoordinationObserver(o CoordinationObserver) {
	if r == nil || o == nil {
		return
	}
	r.coordObservers.mu.Lock()
	r.coordObservers.list = append(r.coordObservers.list, o)
	r.coordObservers.mu.Unlock()
}

// notifyCoordinationObservers publishes the built-in workspace-stream event,
// then fans the hook out to every subscriber with panic isolation.
func (r *Runtime) notifyCoordinationObservers(fn func(CoordinationObserver)) {
	if r == nil {
		return
	}
	r.coordObservers.mu.RLock()
	subs := append([]CoordinationObserver(nil), r.coordObservers.list...)
	r.coordObservers.mu.RUnlock()
	for _, o := range subs {
		func(o CoordinationObserver) {
			defer func() {
				if p := recover(); p != nil {
					r.logger.Error("coordination observer panicked", "observer", slog.AnyValue(p))
				}
			}()
			fn(o)
		}(o)
	}
}

// --- emit points (called from coordination*.go) ---

func (r *Runtime) observeSpawn(ev SpawnEvent) {
	if ev.At == 0 {
		ev.At = time.Now().Unix()
	}
	r.emitWorkspaceEvent(events.TypeWSSpawn,
		map[string]string{"sessionId": ev.WorkerID, "coordinatorId": ev.CoordinatorID, "rootId": ev.RootID}, ev)
	r.notifyCoordinationObservers(func(o CoordinationObserver) { o.OnSpawn(ev) })
}

func (r *Runtime) observeReport(ev ReportEvent) {
	if ev.At == 0 {
		ev.At = time.Now().Unix()
	}
	r.emitWorkspaceEvent(events.TypeWSReport,
		map[string]string{"sessionId": ev.WorkerID, "coordinatorId": ev.CoordinatorID, "status": ev.Status}, ev)
	r.notifyCoordinationObservers(func(o CoordinationObserver) { o.OnReport(ev) })
}

func (r *Runtime) observeDrain(ev DrainEvent) {
	if ev.At == 0 {
		ev.At = time.Now().Unix()
	}
	r.emitWorkspaceEvent(events.TypeWSCoordination,
		map[string]string{"sessionId": ev.CoordinatorID, "phase": ev.Phase}, ev)
	r.notifyCoordinationObservers(func(o CoordinationObserver) { o.OnDrain(ev) })
}

func (r *Runtime) observeStall(ev StallEvent) {
	if ev.At == 0 {
		ev.At = time.Now().Unix()
	}
	r.emitWorkspaceEvent(events.TypeWSCoordination,
		map[string]string{"sessionId": ev.CoordinatorID, "phase": "stall_halt"}, ev)
	r.notifyCoordinationObservers(func(o CoordinationObserver) { o.OnStall(ev) })
}

// noteTag extracts the text of one <tag>…</tag> element from a task
// notification ("" when absent). Used to read the status a worker reported
// without threading it through every notify path.
func noteTag(note, tag string) string {
	open, close := "<"+tag+">", "</"+tag+">"
	i := strings.Index(note, open)
	if i < 0 {
		return ""
	}
	rest := note[i+len(open):]
	j := strings.Index(rest, close)
	if j < 0 {
		return ""
	}
	return strings.TrimSpace(rest[:j])
}

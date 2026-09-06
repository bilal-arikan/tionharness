package agent

import (
	"context"
	"strconv"
	"time"

	"github.com/bilal-arikan/tionharness/internal/events"
	"github.com/bilal-arikan/tionharness/internal/liveness"
)

// Liveness composes the workspace's liveness snapshot (_Docs/77 R2) from the
// runtime's own registries plus the api's chat-run probe. This is the single
// server-side source every "what is running?" consumer reads; the sources it
// folds, most active first:
//
//   - turn admission slots (turnqueue): every turn entry path claims one —
//     chat, slash command, coordinator drain, worker, wake, peer, spawn,
//     automation — so this is the central Running source, with kind + since.
//   - tracked autonomous invokes (trackSession): flow runs and background turns
//     that may run session-less of a slot for a moment.
//   - the api's streamed chat runs (extActive): belt-and-braces for the window
//     between a request arriving and its slot claim.
//   - coordinator slots: an owed drain turn (Queued) and live workers under an
//     otherwise idle coordinator (AwaitingWorkers).
//   - durable asks: a turn parked on a question (WaitingAsk).
//   - waiting flow runs: a run suspended at await-input (WaitingInput).
//
// Capacity reports the spawn budget (active/max, queue depth/max), busy turn
// count and the autonomy brake.
func (r *Runtime) Liveness(ctx context.Context) liveness.Snapshot {
	var b liveness.Builder
	if r == nil {
		return b.Snapshot(liveness.Capacity{}, time.Now().Unix())
	}

	busy := r.turns.BusySessionIDs()
	for _, sid := range busy {
		snap := r.turns.Snapshot(sid)
		e := liveness.Entry{SessionID: sid, State: liveness.Running, Reason: "turn"}
		if snap.Running != nil {
			e.Reason = "turn:" + string(snap.Running.Kind)
			e.Since = snap.Running.Since
		}
		e.Waiting = len(snap.Waiting)
		b.Add(e)
	}
	for _, sid := range r.ActiveSessionIDs() {
		b.Add(liveness.Entry{SessionID: sid, State: liveness.Running, Reason: "run"})
	}
	if r.extActive != nil {
		for _, sid := range r.extActive() {
			b.Add(liveness.Entry{SessionID: sid, State: liveness.Running, Reason: "turn:chat"})
		}
	}

	// Coordinator slots: a drain owed by a pending worker note is admitted work
	// that has not started (Queued); live workers under an idle coordinator make
	// it AwaitingWorkers (the previous runningSessionIDs promoted such a
	// coordinator into the live scope; RunningSet keeps that contract).
	r.coordSlots.Range(func(key, value any) bool {
		coordID, _ := key.(string)
		slot, _ := value.(*coordSlot)
		if coordID == "" || slot == nil {
			return true
		}
		slot.mu.Lock()
		pending := slot.pending || slot.workerPending
		slot.mu.Unlock()
		if pending {
			b.Add(liveness.Entry{SessionID: coordID, State: liveness.Queued, Reason: "coord:drain-pending"})
		}
		if n := slot.workers.Load(); n > 0 {
			b.Add(liveness.Entry{SessionID: coordID, State: liveness.AwaitingWorkers, Reason: "workers:" + strconv.FormatInt(n, 10)})
		}
		return true
	})

	if r.db != nil {
		if asks, err := r.db.ListWaitingSessionAsks(ctx); err == nil {
			for _, a := range asks {
				b.Add(liveness.Entry{SessionID: a.SessionID, State: liveness.WaitingAsk, Reason: "ask:" + a.ID, Since: a.CreatedAt})
			}
		}
		if runs, err := r.db.ListWaitingFlowRuns(ctx); err == nil {
			for _, run := range runs {
				if run.SessionID == "" {
					continue
				}
				b.Add(liveness.Entry{SessionID: run.SessionID, State: liveness.WaitingInput, Reason: "flow:" + run.ID, Since: run.UpdatedAt})
			}
		}
	}

	cap := liveness.Capacity{
		SpawnActive:    int(r.spawnActive.Load()),
		QueueDepth:     r.spawnQueueLen(),
		BusyTurns:      len(busy),
		AutonomyPaused: r.Paused(),
	}
	if r.tun != nil {
		cap.SpawnMax = r.tun.SpawnMaxConcurrent()
		cap.QueueMax = r.tun.SpawnQueueMax()
	}
	return b.Snapshot(cap, time.Now().Unix())
}

// LivenessPayload is the Data shape of a ws:liveness event: one session's
// admission state right after it changed.
type LivenessPayload struct {
	SessionID string `json:"sessionId"`
	Busy      bool   `json:"busy"`
	Kind      string `json:"kind,omitempty"`  // turnqueue.Kind of the running turn
	Label     string `json:"label,omitempty"` // its label, when any
	Since     int64  `json:"since,omitempty"`
	Waiting   int    `json:"waiting"`
}

// emitLiveness publishes a session's admission state to the workspace stream.
// Called from publishTurnQueue, i.e. on every slot claim/release/queue change.
func (r *Runtime) emitLiveness(sessionID string) {
	if r == nil || r.bus == nil || sessionID == "" {
		return
	}
	snap := r.turns.Snapshot(sessionID)
	p := LivenessPayload{SessionID: sessionID, Busy: snap.Running != nil, Waiting: len(snap.Waiting)}
	if snap.Running != nil {
		p.Kind, p.Label, p.Since = string(snap.Running.Kind), snap.Running.Label, snap.Running.Since
	}
	r.emitWorkspaceEvent(events.TypeWSLiveness,
		map[string]string{"sessionId": sessionID, "busy": strconv.FormatBool(p.Busy)}, p)
}

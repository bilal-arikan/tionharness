// Package agent implements the autonomous agent runtime: each agent runs as
// its own goroutine with a heartbeat ticker, manual wake channel, and failure
// backoff. This is the foundation of SwarmGo's multi-agent ("swarm") engine.
package agent

import (
	"context"
	"sync"
	"time"
)

const (
	defaultIntervalSec = 30
	maxFailures        = 10 // auto-disable after this many consecutive failures
	maxBackoffSteps    = 6  // cap exponential backoff growth
	tickTimeout        = 120 * time.Second
)

// WorkerStatus is a snapshot of a worker's state for the API.
type WorkerStatus struct {
	AgentID             string `json:"agentId"`
	Running             bool   `json:"running"`
	Disabled            bool   `json:"disabled"`
	IntervalSec         int    `json:"intervalSec"`
	LastWakeAt          int64  `json:"lastWakeAt"`
	LastOutcome         string `json:"lastOutcome"`
	ConsecutiveFailures int    `json:"consecutiveFailures"`
	TickCount           int    `json:"tickCount"`
}

// worker drives one agent's autonomous loop.
type worker struct {
	agentID string
	rt      *Runtime

	wakeCh chan struct{}
	stopCh chan struct{}
	doneCh chan struct{}

	mu    sync.Mutex
	state WorkerStatus
}

func newWorker(agentID string, rt *Runtime, intervalSec int) *worker {
	if intervalSec <= 0 {
		intervalSec = defaultIntervalSec
	}
	return &worker{
		agentID: agentID,
		rt:      rt,
		wakeCh:  make(chan struct{}, 1),
		stopCh:  make(chan struct{}),
		doneCh:  make(chan struct{}),
		state:   WorkerStatus{AgentID: agentID, Running: true, IntervalSec: intervalSec},
	}
}

// run is the main loop: react to stop, manual wake, or heartbeat ticks.
func (w *worker) run() {
	defer close(w.doneCh)

	interval := time.Duration(w.snapshot().IntervalSec) * time.Second
	timer := time.NewTimer(interval)
	defer timer.Stop()

	for {
		select {
		case <-w.stopCh:
			return

		case <-w.wakeCh:
			w.tick("manual")
			resetTimer(timer, w.nextInterval())

		case <-timer.C:
			if w.snapshot().Disabled {
				return
			}
			w.tick("heartbeat")
			resetTimer(timer, w.nextInterval())
		}
	}
}

// tick performs one wake cycle: run the heartbeat action and classify outcome.
func (w *worker) tick(trigger string) {
	ctx, cancel := context.WithTimeout(context.Background(), tickTimeout)
	defer cancel()

	w.mark(func(s *WorkerStatus) {
		s.LastWakeAt = time.Now().Unix()
		s.TickCount++
	})

	err := w.rt.runHeartbeat(ctx, w.agentID, trigger)

	w.mark(func(s *WorkerStatus) {
		if err != nil {
			s.ConsecutiveFailures++
			s.LastOutcome = "failure: " + err.Error()
			if s.ConsecutiveFailures >= maxFailures {
				s.Disabled = true
				s.Running = false
			}
		} else {
			s.ConsecutiveFailures = 0
			s.LastOutcome = "success"
		}
	})

	w.rt.logger.Info("agent tick",
		"agent", w.agentID, "trigger", trigger,
		"failures", w.snapshot().ConsecutiveFailures, "ok", err == nil)
}

// nextInterval applies exponential backoff while failing.
func (w *worker) nextInterval() time.Duration {
	s := w.snapshot()
	base := time.Duration(s.IntervalSec) * time.Second
	if s.ConsecutiveFailures == 0 {
		return base
	}
	steps := s.ConsecutiveFailures
	if steps > maxBackoffSteps {
		steps = maxBackoffSteps
	}
	return base * time.Duration(1<<steps)
}

func (w *worker) wake() {
	select {
	case w.wakeCh <- struct{}{}:
	default: // a wake is already pending
	}
}

func (w *worker) stop() {
	close(w.stopCh)
	<-w.doneCh
}

func (w *worker) snapshot() WorkerStatus {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.state
}

func (w *worker) mark(fn func(*WorkerStatus)) {
	w.mu.Lock()
	defer w.mu.Unlock()
	fn(&w.state)
}

func resetTimer(t *time.Timer, d time.Duration) {
	if !t.Stop() {
		select {
		case <-t.C:
		default:
		}
	}
	t.Reset(d)
}

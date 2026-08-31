package agent

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

var errSpawnQueueShutdown = errors.New("spawn queue discarded during workspace shutdown")

type spawnQueueItem struct {
	agent      db.Agent
	prompt     string
	opts       SpawnOptions
	enqueuedAt time.Time
}

type spawnQueue struct {
	mu      sync.Mutex
	shallow []spawnQueueItem
	deep    []spawnQueueItem
	closed  bool
}

func (r *Runtime) enqueueSpawn(item spawnQueueItem) (int, error) {
	q := &r.spawnQueue
	q.mu.Lock()
	defer q.mu.Unlock()
	queued := len(q.shallow) + len(q.deep)
	if q.closed {
		return 0, errSpawnQueueShutdown
	}
	if queued >= r.tun.SpawnQueueMax() {
		return 0, fmt.Errorf("spawn limit reached (%d running, %d queued); the queue is full; wait for some to finish", r.tun.SpawnMaxConcurrent(), queued)
	}
	if item.opts.CoordinatorDepth >= deepSpawnDepth {
		q.deep = append(q.deep, item)
	} else {
		q.shallow = append(q.shallow, item)
	}
	return queued + 1, nil
}

func (r *Runtime) signalSpawnQueue() {
	select {
	case r.spawnWake <- struct{}{}:
	default:
	}
}

func (r *Runtime) runSpawnQueue() {
	defer close(r.spawnDone)
	for {
		select {
		case <-r.spawnStop:
			r.dropSpawnQueue(errSpawnQueueShutdown)
			return
		case <-r.spawnWake:
			r.dispatchSpawnQueue()
		}
	}
}

func (r *Runtime) dispatchSpawnQueue() {
	for {
		item, ok := r.takeQueuedSpawn()
		if !ok {
			return
		}
		select {
		case <-r.spawnStop:
			r.releaseSpawnSlot()
			r.dropSpawn(item, errSpawnQueueShutdown)
			return
		default:
		}
		if _, err := r.launchSpawn(context.Background(), item.agent, item.prompt, item.opts); err != nil {
			r.dropSpawn(item, err)
		}
	}
}

func (r *Runtime) takeQueuedSpawn() (spawnQueueItem, bool) {
	q := &r.spawnQueue
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return spawnQueueItem{}, false
	}
	if len(q.shallow) > 0 {
		item := q.shallow[0]
		if !r.acquireSpawnSlotAtDepth(item.opts.CoordinatorDepth) {
			return spawnQueueItem{}, false
		}
		q.shallow = q.shallow[1:]
		return item, true
	}
	if len(q.deep) > 0 {
		item := q.deep[0]
		if !r.acquireSpawnSlotAtDepth(item.opts.CoordinatorDepth) {
			return spawnQueueItem{}, false
		}
		q.deep = q.deep[1:]
		return item, true
	}
	return spawnQueueItem{}, false
}

func (r *Runtime) stopSpawnQueue() {
	r.spawnClose.Do(func() {
		q := &r.spawnQueue
		q.mu.Lock()
		q.closed = true
		q.mu.Unlock()
		close(r.spawnStop)
		<-r.spawnDone
	})
}

func (r *Runtime) dropSpawnQueue(cause error) {
	q := &r.spawnQueue
	q.mu.Lock()
	items := append(q.shallow, q.deep...)
	q.shallow = nil
	q.deep = nil
	q.mu.Unlock()
	for _, item := range items {
		r.dropSpawn(item, cause)
	}
}

func (r *Runtime) dropSpawn(item spawnQueueItem, cause error) {
	r.logger.Warn("spawn queue: dropped queued spawn", "agent", item.agent.ID, "wait", time.Since(item.enqueuedAt), "error", cause)
	// A dropped queued spawn ends a worker its coordinator already counted, so it is
	// a fleet-finishing event exactly like a worker turn returning. onDrop reports the
	// zero-crossing and it is handed to notifyCoordinator: the drain loop no longer
	// runs a standalone reconcile turn, so an unobserved transition here would strand
	// the coordinator without its all-idle signal.
	lastWorker := false
	if item.opts.onDrop != nil {
		lastWorker = item.opts.onDrop(cause)
	}
	if coordID := item.opts.CoordinatorSessionID; coordID != "" {
		r.notifyCoordinator(coordID, fmt.Sprintf("<task-notification worker=%q status=\"failed\">Queued spawn for %s was dropped: %v</task-notification>", item.agent.Name, item.agent.Name, cause), lastWorker, nil)
	}
}

func (r *Runtime) spawnQueueLen() int {
	r.spawnQueue.mu.Lock()
	defer r.spawnQueue.mu.Unlock()
	return len(r.spawnQueue.shallow) + len(r.spawnQueue.deep)
}

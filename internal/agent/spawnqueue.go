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
	mu     sync.Mutex
	items  []spawnQueueItem
	closed bool
}

func (r *Runtime) enqueueSpawn(item spawnQueueItem) (int, error) {
	q := &r.spawnQueue
	q.mu.Lock()
	defer q.mu.Unlock()
	queued := len(q.items)
	if q.closed {
		return 0, errSpawnQueueShutdown
	}
	if queued >= r.tun.SpawnQueueMax() {
		return 0, fmt.Errorf("spawn limit reached (%d running, %d queued); the queue is full; wait for some to finish", r.tun.SpawnMaxConcurrent(), queued)
	}
	q.items = append(q.items, item)
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
	if len(q.items) == 0 || !r.acquireSpawnSlot() {
		return spawnQueueItem{}, false
	}
	item := q.items[0]
	q.items = q.items[1:]
	return item, true
}

// stopSpawnQueue stops the dispatcher loop and drops what is still queued. It does
// NOT wait for the turns launchSpawn already started — that is closeBackgroundTurns,
// which CloseMCP runs first (see the order there).
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
	items := q.items
	q.items = nil
	q.mu.Unlock()
	for _, item := range items {
		r.dropSpawn(item, cause)
	}
}

func (r *Runtime) dropSpawn(item spawnQueueItem, cause error) {
	r.logger.Warn("spawn queue: dropped queued spawn", "agent", item.agent.ID, "wait", time.Since(item.enqueuedAt), "error", cause)
}

func (r *Runtime) spawnQueueLen() int {
	r.spawnQueue.mu.Lock()
	defer r.spawnQueue.mu.Unlock()
	return len(r.spawnQueue.items)
}

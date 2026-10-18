package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
)

func queueTestAgent(t *testing.T, rt *Runtime) db.Agent {
	t.Helper()
	agent, err := rt.db.CreateAgent(context.Background(), db.Agent{Name: "Queue Worker", Provider: "anthropic", Model: "test"})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return agent
}

func fillSpawnSlots(t *testing.T, rt *Runtime, n int) {
	t.Helper()
	for i := range n {
		if !rt.acquireSpawnSlot() {
			t.Fatalf("acquire slot %d", i)
		}
	}
}

func TestSpawnSessionQueuesAtCapacityAndStartsOnRelease(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	agent := queueTestAgent(t, rt)
	rt.tun.SetSpawnLimits(1, 16, 0)
	fillSpawnSlots(t, rt, 1)

	res, err := rt.SpawnSession(context.Background(), agent.ID, "queued work", SpawnOptions{})
	if err != nil {
		t.Fatalf("queue spawn: %v", err)
	}
	if !res.Queued || res.QueuePosition != 1 || res.SessionID != "" {
		t.Fatalf("unexpected queued result: %+v", res)
	}

	rt.releaseSpawnSlot()
	deadline := time.Now().Add(2 * time.Second)
	for rt.spawnQueueLen() != 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if rt.spawnQueueLen() != 0 {
		t.Fatal("queued spawn did not start after slot release")
	}
	drainSpawns(t, rt)
	rt.CloseMCP()
}

func TestSpawnSessionRejectsWhenQueueFull(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	agent := queueTestAgent(t, rt)
	rt.tun.SetSpawnLimits(1, 1, 0)
	fillSpawnSlots(t, rt, 1)

	if res, err := rt.SpawnSession(context.Background(), agent.ID, "first", SpawnOptions{}); err != nil || !res.Queued {
		t.Fatalf("first spawn should queue: result=%+v err=%v", res, err)
	}
	if _, err := rt.SpawnSession(context.Background(), agent.ID, "second", SpawnOptions{}); err == nil || !strings.Contains(err.Error(), "queue is full") {
		t.Fatalf("expected queue-full rejection, got %v", err)
	}

	rt.CloseMCP()
	rt.releaseSpawnSlot()
	drainSpawns(t, rt)
}

// TestSpawnQueueShutdownDropsQueuedWork: closing the runtime must not hang on a
// non-empty queue — the queued spawn is dropped — and the closed queue refuses
// new work instead of parking it forever.
func TestSpawnQueueShutdownDropsQueuedWork(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	agent := queueTestAgent(t, rt)
	if _, err := rt.enqueueSpawn(spawnQueueItem{
		agent:      agent,
		prompt:     "will be dropped",
		enqueuedAt: time.Now(),
	}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	done := make(chan struct{})
	go func() {
		rt.CloseMCP()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("shutdown hung while queue contained work")
	}
	if n := rt.spawnQueueLen(); n != 0 {
		t.Fatalf("queued spawn was not dropped, %d still queued", n)
	}
	if _, err := rt.enqueueSpawn(spawnQueueItem{agent: agent, prompt: "late", enqueuedAt: time.Now()}); !errors.Is(err, errSpawnQueueShutdown) {
		t.Fatalf("enqueue after shutdown = %v, want errSpawnQueueShutdown", err)
	}
	drainSpawns(t, rt)
}

// TestSpawnQueueIsFIFO: the spawn queue is a single first-in-first-out line —
// whoever queued first gets the next free slot.
func TestSpawnQueueIsFIFO(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.tun.SetSpawnLimits(8, 16, 0)
	fillSpawnSlots(t, rt, 6)
	rt.spawnQueue.items = []spawnQueueItem{{prompt: "first"}, {prompt: "second"}}

	for _, want := range []string{"first", "second"} {
		got, ok := rt.takeQueuedSpawn()
		if !ok || got.prompt != want {
			t.Fatalf("expected %q next, got %+v, ok=%v", want, got, ok)
		}
	}
	if _, ok := rt.takeQueuedSpawn(); ok {
		t.Fatal("an empty queue must yield nothing")
	}
	rt.spawnActive.Store(0)
	rt.CloseMCP()
}

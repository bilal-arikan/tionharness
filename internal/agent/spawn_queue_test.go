package agent

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/providers"
)

type queuedResultProvider struct{}

func (queuedResultProvider) Name() string { return "queued-result-test" }

func (queuedResultProvider) Complete(context.Context, providers.Request) (*providers.Response, error) {
	return &providers.Response{Text: "queued worker result surfaced"}, nil
}

var registerQueuedResultProvider sync.Once

func configureQueuedResultProvider(rt *Runtime) {
	registerQueuedResultProvider.Do(func() {
		providers.RegisterKind(providers.NewBuiltinKind(
			providers.Manifest{Kind: "queued-result-test", Transport: providers.TransportAPI},
			func(providers.ResolvedConfig) bool { return true },
			func(providers.ResolvedConfig) (providers.Provider, error) { return queuedResultProvider{}, nil },
		))
	})
	rt.providers.SetInstances([]providers.Instance{{ID: "queued-result-test", KindID: "queued-result-test"}})
}

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
	for i := 0; i < n; i++ {
		if !rt.acquireSpawnSlotAtDepth(0) {
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

func TestSpawnSessionQueuedWorkerResultSurfacesToCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	configureQueuedResultProvider(rt)
	ctx := context.Background()
	agent, err := rt.db.CreateAgent(ctx, db.Agent{
		Name: "Queue Worker", Provider: "queued-result-test", ProviderInstanceID: "queued-result-test", Model: "test",
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	coord, err := rt.db.CreateSession(ctx, db.Session{AgentID: agent.ID, Kind: "chat", SourceID: "queue-result", Title: "Coordinator"})
	if err != nil {
		t.Fatalf("create coordinator: %v", err)
	}
	rt.tun.SetSpawnLimits(1, 16, 0)
	fillSpawnSlots(t, rt, 1)

	res, err := rt.SpawnSession(ctx, agent.ID, "queued work", SpawnOptions{CoordinatorSessionID: coord.ID})
	if err != nil {
		t.Fatalf("queue worker: %v", err)
	}
	if !res.Queued || res.SessionID != "" {
		t.Fatalf("unexpected queued result: %+v", res)
	}
	rt.releaseSpawnSlot()
	drainSpawns(t, rt)

	messages, err := rt.db.ListMessages(ctx, coord.ID)
	if err != nil {
		t.Fatalf("list coordinator messages: %v", err)
	}
	for _, message := range messages {
		if strings.Contains(message.Text, "queued worker result surfaced") &&
			strings.Contains(message.Text, "<status>completed</status>") && message.Origin == "worker-note" {
			return
		}
	}
	t.Fatalf("queued worker result was not surfaced to coordinator: %+v", messages)
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
}

func TestSpawnQueueShutdownDropsAndNotifiesCoordinator(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	agent := queueTestAgent(t, rt)
	coord, err := rt.db.CreateSession(context.Background(), db.Session{AgentID: agent.ID, Kind: "chat", SourceID: "queue-test", Title: "Coordinator"})
	if err != nil {
		t.Fatalf("create coordinator: %v", err)
	}
	dropped := make(chan struct{}, 1)
	_, err = rt.enqueueSpawn(spawnQueueItem{
		agent:  agent,
		prompt: "will be dropped",
		opts: SpawnOptions{CoordinatorSessionID: coord.ID, onDrop: func(error) {
			dropped <- struct{}{}
		}},
		enqueuedAt: time.Now(),
	})
	if err != nil {
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
	select {
	case <-dropped:
	default:
		t.Fatal("queued spawn was not dropped")
	}
	messages, err := rt.db.ListMessages(context.Background(), coord.ID)
	if err != nil {
		t.Fatalf("list coordinator messages: %v", err)
	}
	found := false
	for _, message := range messages {
		if strings.Contains(message.Text, "Queued spawn") && strings.Contains(message.Text, "status=\"failed\"") {
			found = true
		}
	}
	if !found {
		t.Fatal("coordinator did not receive dropped-spawn notification")
	}
}

func TestSpawnQueuePrefersShallowWork(t *testing.T) {
	rt, _ := newTestRuntime(t, t.TempDir())
	rt.tun.SetSpawnLimits(8, 16, 0)
	fillSpawnSlots(t, rt, 7)
	deep := spawnQueueItem{prompt: "deep", opts: SpawnOptions{CoordinatorDepth: deepSpawnDepth}}
	shallow := spawnQueueItem{prompt: "shallow"}
	rt.spawnQueue.deep = []spawnQueueItem{deep}
	rt.spawnQueue.shallow = []spawnQueueItem{shallow}

	got, ok := rt.takeQueuedSpawn()
	if !ok || got.prompt != "shallow" {
		t.Fatalf("expected shallow item first, got %+v, ok=%v", got, ok)
	}
	rt.spawnQueue.deep = nil
	rt.spawnActive.Store(0)
	rt.CloseMCP()
}

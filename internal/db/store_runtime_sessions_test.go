package db

import (
	"fmt"
	"sync"
	"testing"
)

func TestRuntimeSessionsReuseAndInvalidateProjection(t *testing.T) {
	d, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	ctx := t.Context()
	task, err := d.CreateTask(ctx, Task{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	setTaskLastRunForTest(d, task.ID, "failure")
	linked, err := d.CreateSession(ctx, Session{Kind: "task", SourceID: task.ID})
	if err != nil {
		t.Fatal(err)
	}
	chat, err := d.CreateSession(ctx, Session{Kind: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	rows := d.RuntimeSessions()
	byID := map[string]SessionRuntime{}
	for _, row := range rows {
		byID[row.SessionID] = row
	}
	if len(rows) != 1 || byID[linked.ID].LastStatus != "failure" {
		t.Fatalf("task status rows = %+v", rows)
	}
	snapshot := d.runtimeSessions.Load()
	rows[0].LastStatus = "caller mutation"
	var wg sync.WaitGroup
	for range 12 {
		wg.Go(func() { _ = d.RuntimeSessions() })
	}
	wg.Wait()
	if d.runtimeSessions.Load() != snapshot {
		t.Fatal("unchanged history was rebuilt")
	}
	if d.RuntimeSessions()[0].LastStatus == "caller mutation" {
		t.Fatal("caller mutated shared cache")
	}
	if err := d.SetSessionCoordinatorLineage(ctx, chat.ID, "SES-parent", "SES-root", 1); err != nil {
		t.Fatal(err)
	}
	rows = d.RuntimeSessions()
	if len(rows) != 2 || d.runtimeSessions.Load() == snapshot {
		t.Fatalf("lineage mutation not reflected: %+v", rows)
	}
	for _, row := range rows {
		if row.SessionID == chat.ID && row.RootCoordinatorSessionID != "SES-root" {
			t.Fatalf("lost lineage: %+v", row)
		}
	}
	setTaskLastRunForTest(d, task.ID, "success")
	for _, row := range d.RuntimeSessions() {
		if row.SessionID == linked.ID && row.LastStatus != "success" {
			t.Fatal("stale task status")
		}
	}
	if err := d.DeleteSession(ctx, linked.ID); err != nil {
		t.Fatal(err)
	}
	for _, row := range d.RuntimeSessions() {
		if row.SessionID == linked.ID {
			t.Fatal("deleted session retained")
		}
	}
}

func TestRuntimeSessionsTaskFallback(t *testing.T) {
	// Seed a legacy snapshot directly so normalization behavior stays covered
	// independently of today's creation rules.
	d := &DB{sessions: map[string]Session{
		"task": {ID: "task", Kind: "task", SourceID: "T1"},
	}, tasks: map[string]Task{"T1": {ID: "T1", LastRunStatus: "success"}}}
	for _, row := range d.RuntimeSessions() {
		if row.LastStatus != "success" {
			t.Fatalf("status = %+v", row)
		}
	}
	if len(d.RuntimeSessions()) != 1 {
		t.Fatal("missing rows")
	}
}

func BenchmarkRuntimeSessionsWarm(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			d := &DB{sessions: map[string]Session{}}
			for i := range count {
				id := fmt.Sprint(i)
				d.sessions[id] = Session{ID: id, Kind: "chat"}
			}
			d.sessions["worker"] = Session{ID: "worker", CoordinatorSessionID: "parent"}
			_ = d.RuntimeSessions()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				_ = d.RuntimeSessions()
			}
		})
	}
}

// setTaskLastRunForTest stamps a task's last run status directly (there is no
// public setter: the board is passive, runs no longer write back to cards).
func setTaskLastRunForTest(d *DB, id, status string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	t := d.tasks[id]
	t.LastRunStatus = status
	d.tasks[id] = t
	d.markMutatedLocked()
}

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
	first, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishFlowRun(ctx, first.ID, FlowFailure, "", "failed"); err != nil {
		t.Fatal(err)
	}
	latest, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "FLW1"})
	if err != nil {
		t.Fatal(err)
	}
	if err := d.FinishFlowRun(ctx, latest.ID, FlowSuccess, "", ""); err != nil {
		t.Fatal(err)
	}
	linked, err := d.CreateSession(ctx, Session{Kind: "flow", SourceID: "FLW1", Origin: &SessionOrigin{Kind: OriginFlow, EntityID: "FLW1", RunID: first.ID}})
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := d.CreateSession(ctx, Session{Kind: "flow", SourceID: "FLW1"})
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
	if len(rows) != 2 || byID[linked.ID].LastStatus != FlowFailure || byID[legacy.ID].LastStatus != FlowSuccess {
		t.Fatalf("linked/legacy statuses = %+v", rows)
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
	if len(rows) != 3 || d.runtimeSessions.Load() == snapshot {
		t.Fatalf("lineage mutation not reflected: %+v", rows)
	}
	for _, row := range rows {
		if row.SessionID == chat.ID && row.RootCoordinatorSessionID != "SES-root" {
			t.Fatalf("lost lineage: %+v", row)
		}
	}
	if err := d.FinishFlowRun(ctx, latest.ID, FlowFailure, "", "changed"); err != nil {
		t.Fatal(err)
	}
	for _, row := range d.RuntimeSessions() {
		if row.SessionID == legacy.ID && row.LastStatus != FlowFailure {
			t.Fatal("stale flow status")
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

func TestRuntimeSessionsTaskAndMissingRunFallback(t *testing.T) {
	// Seed a legacy snapshot directly so normalization and missing-link behavior
	// remain covered independently of today's creation rules.
	d := &DB{sessions: map[string]Session{
		"task": {ID: "task", Kind: "task", SourceID: "T1"},
		"flow": {ID: "flow", Kind: "flow", SourceID: "F1", Origin: &SessionOrigin{Kind: OriginFlow, EntityID: "F1", RunID: "missing"}},
	}, tasks: map[string]Task{"T1": {ID: "T1", LastRunStatus: "success"}}, flowRuns: map[string]FlowRun{
		"RUN2":  {ID: "RUN2", FlowID: "F1", CreatedAt: 1, Status: FlowFailure},
		"RUN10": {ID: "RUN10", FlowID: "F1", CreatedAt: 1, Status: FlowSuccess},
	}}
	for _, row := range d.RuntimeSessions() {
		if row.LastStatus != "success" {
			t.Fatalf("status = %+v", row)
		}
	}
	if len(d.RuntimeSessions()) != 2 {
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

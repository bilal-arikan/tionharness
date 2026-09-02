package agent

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bilal-arikan/tionharness/internal/db"
	"github.com/bilal-arikan/tionharness/internal/events"
)

type recordingObserver struct {
	mu      sync.Mutex
	spawns  []SpawnEvent
	reports []ReportEvent
	drains  []DrainEvent
	stalls  []StallEvent
}

func (o *recordingObserver) OnSpawn(ev SpawnEvent) {
	o.mu.Lock()
	o.spawns = append(o.spawns, ev)
	o.mu.Unlock()
}

func (o *recordingObserver) OnReport(ev ReportEvent) {
	o.mu.Lock()
	o.reports = append(o.reports, ev)
	o.mu.Unlock()
}

func (o *recordingObserver) OnDrain(ev DrainEvent) {
	o.mu.Lock()
	o.drains = append(o.drains, ev)
	o.mu.Unlock()
}

func (o *recordingObserver) OnStall(ev StallEvent) {
	o.mu.Lock()
	o.stalls = append(o.stalls, ev)
	o.mu.Unlock()
}

type panickingObserver struct{}

func (panickingObserver) OnSpawn(SpawnEvent)   { panic("boom") }
func (panickingObserver) OnReport(ReportEvent) { panic("boom") }
func (panickingObserver) OnDrain(DrainEvent)   { panic("boom") }
func (panickingObserver) OnStall(StallEvent)   { panic("boom") }

// TestCoordinationObserverSeesSpawnAndReport: spawning a worker reaches every
// subscriber (a panicking one isolated) and the workspace stream; the worker's
// terminal task-notification reaches them as a report with the reported status.
func TestCoordinationObserverSeesSpawnAndReport(t *testing.T) {
	rt, _ := newTestRuntime(t, filepath.Join(t.TempDir(), "workspace"))
	rt.wsID = "WS-test"
	drain := collectWS(t, rt)
	ctx := context.Background()
	seedSystemAgents(t, rt)
	base, err := rt.db.CreateAgent(ctx, db.Agent{Name: "Coord", Provider: "anthropic", Model: "m"})
	if err != nil {
		t.Fatalf("create base agent: %v", err)
	}
	rec := &recordingObserver{}
	rt.AddCoordinationObserver(panickingObserver{})
	rt.AddCoordinationObserver(rec)

	coord := newTestCoordinator(t, rt, 0)
	res, err := rt.SpawnWorker(ctx, coord, "explore", "map the code", base.ID, WorkerSpec{})
	if err != nil {
		t.Fatalf("spawn worker: %v", err)
	}
	waitWorkersSettled(t, rt, coord)
	// The report is delivered by the worker goroutine right after the fleet
	// counter's zero-crossing, so give it a moment to land after settle.
	deadline := time.Now().Add(5 * time.Second)
	for {
		rec.mu.Lock()
		n := len(rec.reports)
		rec.mu.Unlock()
		if n > 0 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	drainSpawns(t, rt)

	rec.mu.Lock()
	defer rec.mu.Unlock()
	// AgentRef is the RESOLVED target (the explore profile's system agent id by the
	// time the spawn lands), AgentName its display name.
	if len(rec.spawns) != 1 || rec.spawns[0].WorkerID != res.SessionID || rec.spawns[0].CoordinatorID != coord || rec.spawns[0].RootID != coord || rec.spawns[0].AgentRef == "" || rec.spawns[0].AgentName != res.AgentName || rec.spawns[0].Depth != 1 {
		t.Fatalf("spawn events = %+v, want one for %s under %s", rec.spawns, res.SessionID, coord)
	}
	if len(rec.reports) != 1 || rec.reports[0].WorkerID != res.SessionID || rec.reports[0].CoordinatorID != coord {
		t.Fatalf("report events = %+v, want one from %s", rec.reports, res.SessionID)
	}
	if st := rec.reports[0].Status; st == "" {
		t.Fatalf("report status must be parsed from the note, got %+v", rec.reports[0])
	}
	if !rec.reports[0].LastWorker {
		t.Fatalf("the only worker's report must carry lastWorker, got %+v", rec.reports[0])
	}

	var spawnKinds, reportKinds int
	for _, e := range drain() {
		switch e.Type {
		case events.TypeWSSpawn:
			p := decodeData[SpawnEvent](t, e)
			if p.WorkerID == res.SessionID {
				spawnKinds++
			}
		case events.TypeWSReport:
			p := decodeData[ReportEvent](t, e)
			if p.WorkerID == res.SessionID {
				reportKinds++
			}
		}
	}
	if spawnKinds != 1 || reportKinds != 1 {
		t.Fatalf("workspace stream: spawn=%d report=%d, want 1/1", spawnKinds, reportKinds)
	}
}

// TestNoteTag reads the status tag out of a task notification.
func TestNoteTag(t *testing.T) {
	note := formatTaskNotification("SES9", "AGT1", "explore", "m", "completed", "done", 3, 1200)
	if got := noteTag(note, "status"); got != "completed" {
		t.Fatalf("status = %q", got)
	}
	if got := noteTag(note, "missing"); got != "" {
		t.Fatalf("missing tag = %q, want empty", got)
	}
}

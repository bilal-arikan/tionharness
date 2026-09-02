package liveness

import "testing"

// TestBuilderKeepsMostActiveState: a session reported by several sources ends
// up once, under its most active state, with the queue depth/since filled from
// an equally active source.
func TestBuilderKeepsMostActiveState(t *testing.T) {
	var b Builder
	b.Add(Entry{SessionID: "SES1", State: AwaitingWorkers, Reason: "workers:2"})
	b.Add(Entry{SessionID: "SES1", State: Running, Reason: "turn:coordinator", Since: 10})
	b.Add(Entry{SessionID: "SES1", State: Running, Reason: "run", Waiting: 3}) // same rank: keeps first, fills Waiting
	b.Add(Entry{SessionID: "SES1", State: WaitingAsk, Reason: "ask:SAK1"})     // less active: ignored
	b.Add(Entry{SessionID: "SES2", State: WaitingInput, Reason: "flow:RUN1"})
	b.Add(Entry{SessionID: "", State: Running})

	snap := b.Snapshot(Capacity{SpawnActive: 1, SpawnMax: 4}, 99)
	if len(snap.Entries) != 2 || snap.Entries[0].SessionID != "SES1" || snap.Entries[1].SessionID != "SES2" {
		t.Fatalf("entries = %+v, want SES1 then SES2", snap.Entries)
	}
	e := snap.Entries[0]
	if e.State != Running || e.Reason != "turn:coordinator" || e.Since != 10 || e.Waiting != 3 {
		t.Fatalf("SES1 = %+v, want running/turn:coordinator since 10 waiting 3", e)
	}
	if !snap.Is("SES2", WaitingInput) || snap.Is("SES2", Running) || snap.Is("SES9", Running) {
		t.Fatal("Is() must reflect the recorded state only")
	}
	if set := snap.RunningSet(); !set["SES1"] || set["SES2"] {
		t.Fatalf("RunningSet = %v, want only SES1", set)
	}
	if snap.Count(Running) != 1 || snap.Count(WaitingInput) != 1 || snap.Count(Queued) != 0 {
		t.Fatal("Count() mismatch")
	}
	if snap.Capacity.SpawnMax != 4 || snap.At != 99 {
		t.Fatalf("capacity/at not carried: %+v", snap)
	}
}

// TestRunningSetIncludesAwaitingWorkers preserves the legacy live-scope
// contract: an idle coordinator with live workers counts as working.
func TestRunningSetIncludesAwaitingWorkers(t *testing.T) {
	var b Builder
	b.Add(Entry{SessionID: "coord", State: AwaitingWorkers})
	b.Add(Entry{SessionID: "queued", State: Queued})
	set := b.Snapshot(Capacity{}, 0).RunningSet()
	if !set["coord"] || set["queued"] {
		t.Fatalf("RunningSet = %v, want coord only", set)
	}
}

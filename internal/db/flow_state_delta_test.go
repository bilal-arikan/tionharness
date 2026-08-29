package db

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestFlowRunStateDeltaReloadMaterializesLegacyState(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	d, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	run, err := d.CreateFlowRun(ctx, FlowRun{FlowID: "F", State: `{"current":"a","outputs":{},"steps":0,"trace":[]}`})
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, _, err := d.FlowRunStateJournalInfo(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	delta := FlowRunStateDelta{Version: 1, CheckpointID: checkpoint, Sequence: 1, Scalars: map[string]json.RawMessage{"current": json.RawMessage(`"b"`)}, OutputsUpsert: map[string]string{"a": "value"}}
	if err := d.AppendFlowRunStateDelta(ctx, run.ID, delta); err != nil {
		t.Fatal(err)
	}
	got, _ := d.GetFlowRun(ctx, run.ID)
	if got.State == run.State {
		t.Fatal("reader leaked stale checkpoint")
	}
	d2, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	reloaded, _ := d2.GetFlowRun(ctx, run.ID)
	if reloaded.State != got.State {
		t.Fatalf("reload mismatch\n%s\n%s", reloaded.State, got.State)
	}
	legacyData, _ := os.ReadFile(filepath.Join(root, dirFlowRuns, run.ID+".json"))
	var legacy FlowRun
	if err := json.Unmarshal(legacyData, &legacy); err != nil {
		t.Fatal(err)
	}
	if legacy.State != run.State {
		t.Fatal("delta rewrote legacy checkpoint")
	}
	if err := d2.SetFlowRunState(ctx, run.ID, reloaded.State); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, dirFlowRunStateDeltas, run.ID)); !os.IsNotExist(err) {
		t.Fatalf("journal not cleaned: %v", err)
	}
}

func TestFlowRunStateDeltaRejectsSequenceAndCheckpointCorruption(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	d, _ := Open(root)
	run, _ := d.CreateFlowRun(ctx, FlowRun{FlowID: "F", State: `{"outputs":{}}`})
	checkpoint, _, _ := d.FlowRunStateJournalInfo(ctx, run.ID)
	for name, delta := range map[string]FlowRunStateDelta{
		"sequence":   {Version: 1, CheckpointID: checkpoint, Sequence: 2},
		"checkpoint": {Version: 1, CheckpointID: "wrong", Sequence: 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := d.AppendFlowRunStateDelta(ctx, run.ID, delta); err == nil {
				t.Fatal("expected corruption error")
			}
		})
	}
}
